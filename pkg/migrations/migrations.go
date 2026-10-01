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

	applog "github.com/evolution-foundation/evolution-go/pkg/applog"
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
