// Package migrations applies reviewable, versioned schema changes.
//
// # WHY THIS EXISTS
//
// The schema used to be created with a single gorm `AutoMigrate` call. That
// works for a greenfield database but is not safe for production:
//
//   - AutoMigrate only ever ADDS. It cannot drop a column, change a type, back
//     fill a value, or rename anything — so once a model changes the two sides
//     silently drift.
//   - There is no record of what was applied and when. Two environments can end
//     up on different shapes with no way to tell.
//   - It runs blindly on every boot, including on large tables, with no way to
//     review the SQL before it reaches production.
//
// Here each change is an explicit, ordered, named step recorded in a
// `schema_migrations` table. Steps are:
//
//   - append-only (checksum-free names, but never reordered or renamed),
//   - applied inside a transaction where the driver allows it,
//   - idempotent on the parts that must be (CREATE INDEX IF NOT EXISTS, …),
//   - reviewable: the SQL is data in the binary, not inferred from structs.
//
// AutoMigrate is still run FIRST for the initial bootstrap of a brand-new
// database, because the models already describe the desired shape and rewriting
// the whole schema as raw SQL would be a large, error-prone duplication. The
// versioned steps then handle everything AutoMigrate cannot: destructive and
// corrective changes going forward.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
)

// migration is one ordered step. Statements run in order; `Postgres` and
// `SQLite` let a step diverge where the dialects differ.
type migration struct {
	Name string
	// AuthDB marks a migration that belongs in the AUTH database (key store +
	// ownership) rather than the users/messages database. ApplyAuth runs only
	// these.
	AuthDB   bool
	Postgres []string
	SQLite   []string
}

// steps is the ordered list of migrations. APPEND ONLY: never reorder or rename
// an existing entry — a new environment and an old one must agree on the order.
var steps = []migration{
	{
		// The messages table is the one that grows without bound and is queried
		// by instance + chat + time. AutoMigrate created the single-column
		// indexes; this adds the composite index the history endpoints actually
		// use, which a per-column index cannot serve as well.
		Name: "0001_messages_instance_chat_timestamp_index",
		Postgres: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_chat_ts ON messages (instance_id, chat_jid, "timestamp" DESC)`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_chat_ts ON messages (instance_id, chat_jid, "timestamp" DESC)`,
		},
	},
	{
		// Retention deletes with `timestamp < cutoff`; a standalone index on
		// timestamp lets that range scan avoid a full table scan on a large
		// table. AutoMigrate created it as a plain index, but only when the
		// column was first added; make it explicit and idempotent.
		Name: "0002_messages_timestamp_index",
		Postgres: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages ("timestamp")`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages ("timestamp")`,
		},
	},
	{
		// Device-scoped uniqueness for (instance, message) pairs. message_id is
		// globally unique today (a WhatsApp id), but the scoped lookups added for
		// tenant isolation filter by instance_id + message_id; this index keeps
		// them index-only instead of scanning.
		Name: "0003_messages_instance_message_index",
		Postgres: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_message ON messages (instance_id, message_id)`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_message ON messages (instance_id, message_id)`,
		},
	},
	{
		// Ownership + lease record for horizontal scaling. One row per instance
		// says which node currently owns (connects) it and until when the lease
		// is valid. This is the table the routing/failover layer reads; the
		// advisory lock in pkg/ownership is the correctness guard, this is the
		// discoverable state (who is where, and for how long).
		Name:   "0004_instance_ownership",
		AuthDB: true,
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS instance_ownership (
				instance_id   TEXT PRIMARY KEY,
				node_id       TEXT NOT NULL,
				locked_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				lease_expires TIMESTAMPTZ NOT NULL,
				owner_epoch   BIGINT NOT NULL DEFAULT 1
			)`,
			`CREATE INDEX IF NOT EXISTS idx_instance_ownership_node ON instance_ownership (node_id)`,
			`CREATE INDEX IF NOT EXISTS idx_instance_ownership_expiry ON instance_ownership (lease_expires)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS instance_ownership (
				instance_id   TEXT PRIMARY KEY,
				node_id       TEXT NOT NULL,
				locked_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				lease_expires TIMESTAMP NOT NULL,
				owner_epoch   INTEGER NOT NULL DEFAULT 1
			)`,
			`CREATE INDEX IF NOT EXISTS idx_instance_ownership_node ON instance_ownership (node_id)`,
			`CREATE INDEX IF NOT EXISTS idx_instance_ownership_expiry ON instance_ownership (lease_expires)`,
		},
	},
	{
		// Instance API tokens encrypted at rest. token_hash lets auth look the
		// instance up without the plaintext; token_enc keeps a reversible copy.
		// The Go-side backfill (EncryptExistingTokens) fills them and blanks the
		// plaintext column for rows that predate encryption.
		Name: "0005_instance_token_hash_enc",
		Postgres: []string{
			`ALTER TABLE instances ADD COLUMN IF NOT EXISTS token_hash TEXT`,
			`ALTER TABLE instances ADD COLUMN IF NOT EXISTS token_enc TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_instances_token_hash ON instances (token_hash)`,
		},
		SQLite: []string{
			`ALTER TABLE instances ADD COLUMN token_hash TEXT`,
			`ALTER TABLE instances ADD COLUMN token_enc TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_instances_token_hash ON instances (token_hash)`,
		},
	},
	{
		// AutoMigrate created uni_instances_token UNIQUE(token). Once tokens are
		// encrypted, every row stores an empty plaintext token and the second
		// such row violates the constraint, so instance creation failed with a
		// duplicate-key error. Uniqueness only makes sense for rows that still
		// carry a legacy plaintext token (encrypted rows authenticate by
		// token_hash), so replace it with a partial unique index that ignores
		// blanks, and normalise the blank rows to NULL.
		Name: "0006_instances_token_partial_unique",
		Postgres: []string{
			`ALTER TABLE instances DROP CONSTRAINT IF EXISTS uni_instances_token`,
			`DROP INDEX IF EXISTS uni_instances_token`,
			`UPDATE instances SET token = NULL WHERE token = ''`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_instances_token_present ON instances (token) WHERE token IS NOT NULL AND token <> ''`,
		},
		SQLite: []string{
			`DROP INDEX IF EXISTS uni_instances_token`,
			`UPDATE instances SET token = NULL WHERE token = ''`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_instances_token_present ON instances (token) WHERE token IS NOT NULL AND token <> ''`,
		},
	},
	{
		// Persistent operator intent for boot-time reconnection. AutoMigrate adds
		// the column (additive), so the SQLite step only backfills: SQLite has no
		// ADD COLUMN IF NOT EXISTS and AutoMigrate always runs first. Rows that
		// were explicitly disconnected through the API must stay down after a
		// restart; everything else should come back automatically.
		Name: "0007_instances_auto_reconnect_backfill",
		Postgres: []string{
			`ALTER TABLE instances ADD COLUMN IF NOT EXISTS auto_reconnect BOOLEAN NOT NULL DEFAULT TRUE`,
			`UPDATE instances SET auto_reconnect = FALSE WHERE disconnect_reason = 'Disconnected by API'`,
		},
		SQLite: []string{
			`UPDATE instances SET auto_reconnect = FALSE WHERE disconnect_reason = 'Disconnected by API'`,
		},
	},
	{
		// message_id was unique across the whole table, but WhatsApp ids are only
		// unique per account, so two instances sharing a conversation overwrote
		// each other's row. Existing rows satisfy the new index by construction
		// (the old constraint was stricter). The new index has the same columns as
		// idx_messages_instance_message, which it replaces. The second constraint
		// name is what databases created by older gorm versions have.
		//
		// SQLite cannot drop an inline UNIQUE, but the messages table only lives
		// in Postgres in practice (see migrate in cmd/wamux/main.go).
		Name: "0008_messages_unique_per_instance",
		Postgres: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_messages_instance_message ON messages (instance_id, message_id)`,
			`ALTER TABLE messages DROP CONSTRAINT IF EXISTS uni_messages_message_id`,
			`ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_id_key`,
			`DROP INDEX IF EXISTS idx_messages_instance_message`,
		},
		SQLite: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_messages_instance_message ON messages (instance_id, message_id)`,
			`DROP INDEX IF EXISTS idx_messages_instance_message`,
		},
	},
	{
		// Both are prefixes of idx_messages_instance_chat_ts or of the unique
		// index, and nothing filters by chat_jid alone. Each is a write on every
		// insert: dropping them made inserts about 30% faster.
		Name: "0009_messages_drop_redundant_indexes",
		Postgres: []string{
			`DROP INDEX IF EXISTS idx_messages_instance_id`,
			`DROP INDEX IF EXISTS idx_messages_chat_jid`,
		},
		SQLite: []string{
			`DROP INDEX IF EXISTS idx_messages_instance_id`,
			`DROP INDEX IF EXISTS idx_messages_chat_jid`,
		},
	},
	{
		// Receipts for messages that were never stored each left a row with no chat,
		// and those rows inflated every dashboard count. The counters are cleared so
		// EnsureMessageCounters rebuilds them from what is left. The partial index
		// keeps the sweep for new ones off the rest of the table.
		Name: "0010_messages_orphan_receipts",
		Postgres: []string{
			`DELETE FROM messages WHERE COALESCE(chat_jid, '') = ''`,
			`DO $$ BEGIN
				IF to_regclass('message_counters') IS NOT NULL THEN
					DELETE FROM message_counters;
				END IF;
			END $$`,
			`CREATE INDEX IF NOT EXISTS idx_messages_orphan_receipts ON messages ("timestamp") WHERE chat_jid = ''`,
		},
		SQLite: []string{
			`DELETE FROM messages WHERE COALESCE(chat_jid, '') = ''`,
			`CREATE INDEX IF NOT EXISTS idx_messages_orphan_receipts ON messages ("timestamp") WHERE chat_jid = ''`,
		},
	},
	{
		// Everything stored so far counts as read, or an upgrade would show every old
		// message as unread.
		Name: "0011_chat_reads",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS chat_reads (
				instance_id text NOT NULL,
				chat_jid text NOT NULL,
				read_at text,
				PRIMARY KEY (instance_id, chat_jid)
			)`,
			`INSERT INTO chat_reads (instance_id, chat_jid, read_at)
				SELECT instance_id, chat_jid, max("timestamp") FROM messages
				WHERE COALESCE(chat_jid, '') <> '' GROUP BY instance_id, chat_jid
				ON CONFLICT DO NOTHING`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS chat_reads (
				instance_id text NOT NULL,
				chat_jid text NOT NULL,
				read_at text,
				PRIMARY KEY (instance_id, chat_jid)
			)`,
			`INSERT INTO chat_reads (instance_id, chat_jid, read_at)
				SELECT instance_id, chat_jid, max("timestamp") FROM messages
				WHERE COALESCE(chat_jid, '') <> '' GROUP BY instance_id, chat_jid
				ON CONFLICT DO NOTHING`,
		},
	},
	{
		// Durable idempotency for POST /send/text and /send/media. The unique
		// (instance, key) index is the concurrency guard; the response body is
		// replayed to a retry instead of sending a second WhatsApp message.
		Name: "0012_send_operations",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS send_operations (
				id uuid PRIMARY KEY,
				instance_id text NOT NULL,
				idempotency_key text NOT NULL,
				payload_hash text NOT NULL,
				state text NOT NULL,
				status_code integer NOT NULL DEFAULT 0,
				response bytea,
				message_id text,
				error text,
				created_at timestamptz NOT NULL DEFAULT NOW(),
				updated_at timestamptz NOT NULL DEFAULT NOW()
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_send_operations_instance_key ON send_operations (instance_id, idempotency_key)`,
			`CREATE INDEX IF NOT EXISTS idx_send_operations_created ON send_operations (created_at)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS send_operations (
				id text PRIMARY KEY,
				instance_id text NOT NULL,
				idempotency_key text NOT NULL,
				payload_hash text NOT NULL,
				state text NOT NULL,
				status_code integer NOT NULL DEFAULT 0,
				response blob,
				message_id text,
				error text,
				created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uni_send_operations_instance_key ON send_operations (instance_id, idempotency_key)`,
			`CREATE INDEX IF NOT EXISTS idx_send_operations_created ON send_operations (created_at)`,
		},
	},
	{
		// The native id chosen before transport. A pending operation can be
		// reconciled against the stored message with this id after a crash.
		// SQLite gets the column from AutoMigrate (it cannot ADD COLUMN IF NOT
		// EXISTS), so only Postgres needs an explicit statement.
		Name: "0013_send_operations_planned_id",
		Postgres: []string{
			`ALTER TABLE send_operations ADD COLUMN IF NOT EXISTS planned_message_id TEXT`,
		},
	},
	{
		// Album identity on each media item, so the manager can reconstruct one
		// album after a reload. SQLite gets the columns from AutoMigrate.
		Name: "0014_messages_album",
		Postgres: []string{
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS album_id TEXT`,
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS album_index INTEGER`,
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_album ON messages (instance_id, album_id)`,
		},
	},
	{
		// Chat state synced from WhatsApp app-state and history. Separate from
		// chat_reads: this is WhatsApp's state, not the manager's read position.
		Name: "0015_chat_states",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS chat_states (
				instance_id text NOT NULL,
				chat_jid text NOT NULL,
				archived boolean NOT NULL DEFAULT false,
				pinned boolean NOT NULL DEFAULT false,
				muted_until bigint NOT NULL DEFAULT 0,
				marked_unread boolean NOT NULL DEFAULT false,
				updated_at timestamptz NOT NULL DEFAULT NOW(),
				PRIMARY KEY (instance_id, chat_jid)
			)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS chat_states (
				instance_id text NOT NULL,
				chat_jid text NOT NULL,
				archived integer NOT NULL DEFAULT 0,
				pinned integer NOT NULL DEFAULT 0,
				muted_until integer NOT NULL DEFAULT 0,
				marked_unread integer NOT NULL DEFAULT 0,
				updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (instance_id, chat_jid)
			)`,
		},
	},
	{
		// Append-only administrative audit. Local only: no external call and no
		// secret values (detail is a small JSON object chosen by the caller).
		Name: "0016_audit_logs",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS audit_logs (
				id text PRIMARY KEY,
				actor text NOT NULL,
				action text NOT NULL,
				target text NOT NULL,
				detail text,
				created_at timestamptz NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs (created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS audit_logs (
				id text PRIMARY KEY,
				actor text NOT NULL,
				action text NOT NULL,
				target text NOT NULL,
				detail text,
				created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs (created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action)`,
		},
	},
	{
		// Global per-instance search pages by timestamp without a chat filter.
		// The existing (instance, chat, timestamp) index cannot serve that
		// prefix, so add the (instance, timestamp) shape the query needs.
		Name: "0017_messages_instance_timestamp_index",
		Postgres: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_ts ON messages (instance_id, "timestamp" DESC)`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_ts ON messages (instance_id, "timestamp" DESC)`,
		},
	},
	{
		Name: "0018_instances_integration",
		Postgres: []string{
			`ALTER TABLE instances ADD COLUMN IF NOT EXISTS integration TEXT NOT NULL DEFAULT 'WHATSAPP-BAILEYS'`,
			`UPDATE instances SET integration = 'WHATSAPP-BAILEYS' WHERE integration IS NULL OR integration = ''`,
			`CREATE INDEX IF NOT EXISTS idx_instances_integration ON instances (integration)`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_instances_integration ON instances (integration)`,
		},
	},
	{
		Name: "0019_cloud_apps",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_apps (
				app_id TEXT PRIMARY KEY,
				app_secret_enc TEXT NOT NULL,
				verify_token_hash TEXT NOT NULL,
				graph_version TEXT NOT NULL DEFAULT 'v27.0',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_apps_verify_token ON cloud_apps (verify_token_hash)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_apps (
				app_id TEXT PRIMARY KEY,
				app_secret_enc TEXT NOT NULL,
				verify_token_hash TEXT NOT NULL,
				graph_version TEXT NOT NULL DEFAULT 'v27.0',
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_apps_verify_token ON cloud_apps (verify_token_hash)`,
		},
	},
	{
		Name: "0020_cloud_api_credentials",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_api_credentials (
				instance_id UUID PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
				app_id TEXT NOT NULL REFERENCES cloud_apps(app_id),
				waba_id TEXT NOT NULL,
				phone_number_id TEXT NOT NULL,
				business_id TEXT,
				display_phone_number TEXT,
				verified_name TEXT,
				access_token_enc TEXT NOT NULL,
				token_expires_at TIMESTAMPTZ,
				last_verified_at TIMESTAMPTZ,
				last_verify_error TEXT,
				registered BOOLEAN NOT NULL DEFAULT FALSE,
				registered_at TIMESTAMPTZ,
				registration_status TEXT,
				last_registration_error TEXT,
				quality_rating TEXT,
				messaging_limit TEXT,
				enforce_window BOOLEAN NOT NULL DEFAULT TRUE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_credentials_phone ON cloud_api_credentials (phone_number_id)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_credentials_waba ON cloud_api_credentials (waba_id)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_api_credentials (
				instance_id TEXT PRIMARY KEY,
				app_id TEXT NOT NULL,
				waba_id TEXT NOT NULL,
				phone_number_id TEXT NOT NULL,
				business_id TEXT,
				display_phone_number TEXT,
				verified_name TEXT,
				access_token_enc TEXT NOT NULL,
				token_expires_at TIMESTAMP,
				last_verified_at TIMESTAMP,
				last_verify_error TEXT,
				registered INTEGER NOT NULL DEFAULT 0,
				registered_at TIMESTAMP,
				registration_status TEXT,
				last_registration_error TEXT,
				quality_rating TEXT,
				messaging_limit TEXT,
				enforce_window INTEGER NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_credentials_phone ON cloud_api_credentials (phone_number_id)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_credentials_waba ON cloud_api_credentials (waba_id)`,
		},
	},
	{
		Name: "0021_cloud_contacts",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_contacts (
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				contact_id TEXT NOT NULL,
				bsuid TEXT,
				parent_bsuid TEXT,
				wa_id TEXT,
				username TEXT,
				display_name TEXT,
				first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (instance_id, contact_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_contacts_bsuid ON cloud_contacts (instance_id, bsuid)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_contacts_wa_id ON cloud_contacts (instance_id, wa_id)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_contacts (
				instance_id TEXT NOT NULL,
				contact_id TEXT NOT NULL,
				bsuid TEXT,
				parent_bsuid TEXT,
				wa_id TEXT,
				username TEXT,
				display_name TEXT,
				first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (instance_id, contact_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_contacts_bsuid ON cloud_contacts (instance_id, bsuid)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_contacts_wa_id ON cloud_contacts (instance_id, wa_id)`,
		},
	},
	{
		Name: "0022_cloud_templates",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_templates (
				id UUID PRIMARY KEY,
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				template_id TEXT,
				name TEXT NOT NULL,
				language TEXT NOT NULL,
				category TEXT,
				status TEXT,
				components JSONB NOT NULL DEFAULT '[]'::jsonb,
				rejected_reason TEXT,
				quality_score TEXT,
				last_synced_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (instance_id, name, language)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_templates_instance ON cloud_templates (instance_id, status)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_templates (
				id TEXT PRIMARY KEY,
				instance_id TEXT NOT NULL,
				template_id TEXT,
				name TEXT NOT NULL,
				language TEXT NOT NULL,
				category TEXT,
				status TEXT,
				components TEXT NOT NULL DEFAULT '[]',
				rejected_reason TEXT,
				quality_score TEXT,
				last_synced_at TIMESTAMP,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (instance_id, name, language)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_templates_instance ON cloud_templates (instance_id, status)`,
		},
	},
	{
		Name: "0023_cloud_chat_windows",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_chat_windows (
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				contact_id TEXT NOT NULL,
				chat_jid TEXT,
				last_inbound_at TIMESTAMPTZ NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				last_message_id TEXT,
				PRIMARY KEY (instance_id, contact_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_windows_expiry ON cloud_chat_windows (expires_at)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_chat_windows (
				instance_id TEXT NOT NULL,
				contact_id TEXT NOT NULL,
				chat_jid TEXT,
				last_inbound_at TIMESTAMP NOT NULL,
				expires_at TIMESTAMP NOT NULL,
				last_message_id TEXT,
				PRIMARY KEY (instance_id, contact_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_windows_expiry ON cloud_chat_windows (expires_at)`,
		},
	},
	{
		Name: "0024_cloud_webhook_events",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_webhook_events (
				event_key TEXT PRIMARY KEY,
				instance_id UUID,
				event_type TEXT NOT NULL,
				received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				processed_at TIMESTAMPTZ,
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_webhook_events_pending ON cloud_webhook_events (processed_at, received_at)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_webhook_events (
				event_key TEXT PRIMARY KEY,
				instance_id TEXT,
				event_type TEXT NOT NULL,
				received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				processed_at TIMESTAMP,
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_webhook_events_pending ON cloud_webhook_events (processed_at, received_at)`,
		},
	},
	{
		Name: "0025_cloud_media_jobs",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_media_jobs (
				id UUID PRIMARY KEY,
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				message_id TEXT NOT NULL,
				media_id TEXT NOT NULL,
				mime_type TEXT,
				filename TEXT,
				declared_size BIGINT,
				status TEXT NOT NULL DEFAULT 'pending',
				attempts INTEGER NOT NULL DEFAULT 0,
				next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_error TEXT,
				completed_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (instance_id, message_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_media_jobs_ready ON cloud_media_jobs (status, next_attempt_at)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_media_jobs (
				id TEXT PRIMARY KEY,
				instance_id TEXT NOT NULL,
				message_id TEXT NOT NULL,
				media_id TEXT NOT NULL,
				mime_type TEXT,
				filename TEXT,
				declared_size INTEGER,
				status TEXT NOT NULL DEFAULT 'pending',
				attempts INTEGER NOT NULL DEFAULT 0,
				next_attempt_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				last_error TEXT,
				completed_at TIMESTAMP,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (instance_id, message_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_media_jobs_ready ON cloud_media_jobs (status, next_attempt_at)`,
		},
	},
	{
		Name: "0026_messages_cloud_identity",
		Postgres: []string{
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS channel TEXT NOT NULL DEFAULT 'baileys'`,
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS contact_id TEXT`,
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS sender_user_id TEXT`,
			`ALTER TABLE messages ADD COLUMN IF NOT EXISTS recipient_user_id TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_channel ON messages (instance_id, channel)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_contact ON messages (instance_id, contact_id, "timestamp" DESC)`,
		},
		SQLite: []string{
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_channel ON messages (instance_id, channel)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_instance_contact ON messages (instance_id, contact_id, "timestamp" DESC)`,
		},
	},
	{
		Name: "0027_cloud_status_events",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_status_events (
				id UUID PRIMARY KEY,
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				message_id TEXT NOT NULL,
				status TEXT NOT NULL,
				recipient_user_id TEXT,
				recipient_id TEXT,
				error_code TEXT,
				error_title TEXT,
				error_message TEXT,
				occurred_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (instance_id, message_id, status, occurred_at)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_status_message ON cloud_status_events (instance_id, message_id, occurred_at DESC)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_status_events (
				id TEXT PRIMARY KEY,
				instance_id TEXT NOT NULL,
				message_id TEXT NOT NULL,
				status TEXT NOT NULL,
				recipient_user_id TEXT,
				recipient_id TEXT,
				error_code TEXT,
				error_title TEXT,
				error_message TEXT,
				occurred_at TIMESTAMP NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (instance_id, message_id, status, occurred_at)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_status_message ON cloud_status_events (instance_id, message_id, occurred_at DESC)`,
		},
	},
	{
		Name: "0028_cloud_account_events",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_account_events (
				id UUID PRIMARY KEY,
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				field TEXT NOT NULL,
				event_type TEXT,
				payload JSONB NOT NULL,
				received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_account_events_instance ON cloud_account_events (instance_id, received_at DESC)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_account_events (
				id TEXT PRIMARY KEY,
				instance_id TEXT NOT NULL,
				field TEXT NOT NULL,
				event_type TEXT,
				payload TEXT NOT NULL,
				received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX IF NOT EXISTS idx_cloud_account_events_instance ON cloud_account_events (instance_id, received_at DESC)`,
		},
	},
	{
		Name: "0029_cloud_user_preferences",
		Postgres: []string{
			`CREATE TABLE IF NOT EXISTS cloud_user_preferences (
				instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
				contact_id TEXT NOT NULL,
				marketing_opt_out BOOLEAN NOT NULL DEFAULT FALSE,
				source_event_at TIMESTAMPTZ,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (instance_id, contact_id)
			)`,
		},
		SQLite: []string{
			`CREATE TABLE IF NOT EXISTS cloud_user_preferences (
				instance_id TEXT NOT NULL,
				contact_id TEXT NOT NULL,
				marketing_opt_out INTEGER NOT NULL DEFAULT 0,
				source_event_at TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (instance_id, contact_id)
			)`,
		},
	},
}

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    TEXT PRIMARY KEY,
	applied_at TIMESTAMP NOT NULL
)`

// Apply runs every migration that has not been applied yet, in order. It is safe
// to call on every boot: applied steps are skipped. Returns the names applied in
// this call.
func Apply(ctx context.Context, db *sql.DB, driver string) ([]string, error) {
	return applySteps(ctx, db, driver, steps, false)
}

// ApplyAuth runs only the migrations that target the AUTH database (the
// whatsmeow key store + ownership/lease table), which is a separate database
// from the users/messages schema. Both databases keep their own
// schema_migrations table, so the version sets are independent.
func ApplyAuth(ctx context.Context, db *sql.DB, driver string) ([]string, error) {
	authSteps := make([]migration, 0, len(steps))
	for _, s := range steps {
		if s.AuthDB {
			authSteps = append(authSteps, s)
		}
	}
	return applySteps(ctx, db, driver, authSteps, false)
}

func applySteps(ctx context.Context, db *sql.DB, driver string, list []migration, _ bool) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("migrations: nil database")
	}

	if _, err := db.ExecContext(ctx, createMigrationsTable); err != nil {
		return nil, fmt.Errorf("migrations: create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, step := range list {
		if applied[step.Name] {
			continue
		}

		stmts := step.Postgres
		if driver == "sqlite" {
			stmts = step.SQLite
		}

		start := time.Now()
		if err := runStep(ctx, db, step.Name, stmts); err != nil {
			return ran, fmt.Errorf("migrations: %s failed: %w", step.Name, err)
		}
		applog.Logger.LogInfo("[MIGRATIONS] applied %s in %s", step.Name, time.Since(start))
		ran = append(ran, step.Name)
	}

	return ran, nil
}

// runStep runs the statements and records the version. Statements are executed
// individually (not wrapped in one transaction) because some DDL — notably
// CREATE INDEX CONCURRENTLY and SQLite's non-transactional statements — cannot
// run inside a transaction. Each statement is written to be idempotent so a
// partial run can be retried safely.
func runStep(ctx context.Context, db *sql.DB, name string, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("statement failed: %w", err)
		}
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)`,
		name, time.Now(),
	)
	if err != nil {
		// SQLite also accepts the same positional syntax.
		return fmt.Errorf("record version: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrations: read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Names returns the ordered migration names, for tests and diagnostics.
func Names() []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Name)
	}
	return out
}
