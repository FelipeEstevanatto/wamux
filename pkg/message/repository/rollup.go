package message_repository

import (
	"fmt"

	"gorm.io/gorm"
)

// The dashboard's aggregates used to be whole-table scans of `messages`. This
// maintains a much smaller rollup table instead, kept in sync by a trigger, so
// the dashboard reads fresh numbers without aggregating the whole table.
//
// One row per (instance_id, day, status, source) with a count: that single
// dimension set answers every dashboard question (total, by status, by day, top
// sources, per-instance count, distinct conversations per instance) while
// keeping the table bounded by the number of conversations per day rather than
// the number of messages.
//
// It must be a trigger rather than application code because a message row is not
// append-only: the same message_id is re-written as Sent -> Delivered -> Read,
// each time with a new status and timestamp, so the counters have to move a
// count from the old bucket to the new one. A trigger sees OLD and NEW and runs
// in the same transaction, so it cannot drift.
const (
	counterTableName = "message_counters"
	counterTrigger   = "message_counters_sync"

	counterTableSQL = `
CREATE TABLE IF NOT EXISTS ` + counterTableName + ` (
    instance_id text   NOT NULL DEFAULT '',
    day         text   NOT NULL DEFAULT '',
    status      text   NOT NULL DEFAULT '',
    source      text   NOT NULL DEFAULT '',
    total       bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (instance_id, day, status, source)
)`

	counterFunctionSQL = `
CREATE OR REPLACE FUNCTION ` + counterTrigger + `() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    old_key text;
    new_key text;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        old_key := COALESCE(OLD.instance_id,'') || '|' || substr(COALESCE(OLD."timestamp",''),1,10)
                   || '|' || COALESCE(OLD.status,'') || '|' || COALESCE(OLD.source,'');
    END IF;
    IF TG_OP <> 'DELETE' THEN
        new_key := COALESCE(NEW.instance_id,'') || '|' || substr(COALESCE(NEW."timestamp",''),1,10)
                   || '|' || COALESCE(NEW.status,'') || '|' || COALESCE(NEW.source,'');
    END IF;

    -- UPDATE that does not move the row between buckets changes nothing.
    IF TG_OP = 'UPDATE' AND old_key = new_key THEN
        RETURN NULL;
    END IF;

    IF TG_OP <> 'INSERT' THEN
        UPDATE ` + counterTableName + ` SET total = total - 1
         WHERE instance_id = COALESCE(OLD.instance_id,'')
           AND day = substr(COALESCE(OLD."timestamp",''),1,10)
           AND status = COALESCE(OLD.status,'')
           AND source = COALESCE(OLD.source,'');
        DELETE FROM ` + counterTableName + `
         WHERE total <= 0
           AND instance_id = COALESCE(OLD.instance_id,'')
           AND day = substr(COALESCE(OLD."timestamp",''),1,10)
           AND status = COALESCE(OLD.status,'')
           AND source = COALESCE(OLD.source,'');
    END IF;

    IF TG_OP <> 'DELETE' THEN
        INSERT INTO ` + counterTableName + ` (instance_id, day, status, source, total)
        VALUES (COALESCE(NEW.instance_id,''), substr(COALESCE(NEW."timestamp",''),1,10),
                COALESCE(NEW.status,''), COALESCE(NEW.source,''), 1)
        ON CONFLICT (instance_id, day, status, source)
        DO UPDATE SET total = ` + counterTableName + `.total + 1;
    END IF;

    RETURN NULL;
END $$`

	counterTriggerSQL = `
CREATE TRIGGER ` + counterTrigger + `
    AFTER INSERT OR UPDATE OR DELETE ON messages
    FOR EACH ROW EXECUTE FUNCTION ` + counterTrigger + `()`

	// The backfill is only needed on first install, when the counters do not
	// exist yet for rows that are already in the table.
	counterBackfillSQL = `
INSERT INTO ` + counterTableName + ` (instance_id, day, status, source, total)
SELECT COALESCE(instance_id,''), substr(COALESCE("timestamp",''),1,10),
       COALESCE(status,''), COALESCE(source,''), count(*)
FROM messages
GROUP BY 1, 2, 3, 4`
)

// EnsureMessageCounters installs the rollup table, its trigger and (on first
// run) the backfill. It is idempotent and safe to call on every boot.
//
// Installing the trigger and backfilling happen in one transaction that takes a
// SHARE lock on `messages`: that blocks writes (reads are unaffected) for the
// duration, which is what guarantees no row can slip in between the backfill's
// snapshot and the trigger becoming active — counted once, not twice and not
// zero times. The lock is only taken when something actually needs installing,
// so later boots do no more than two cheap catalog lookups.
func EnsureMessageCounters(db *gorm.DB) error {
	if err := db.Exec(counterTableSQL).Error; err != nil {
		return fmt.Errorf("create %s: %w", counterTableName, err)
	}
	if err := db.Exec(counterFunctionSQL).Error; err != nil {
		return fmt.Errorf("create %s(): %w", counterTrigger, err)
	}

	var needsBackfill bool
	if err := db.Raw(`SELECT NOT EXISTS (SELECT 1 FROM ` + counterTableName + `)`).Scan(&needsBackfill).Error; err != nil {
		return fmt.Errorf("check %s: %w", counterTableName, err)
	}

	var hasTrigger bool
	if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = ? AND tgrelid = 'messages'::regclass)`, counterTrigger).Scan(&hasTrigger).Error; err != nil {
		return fmt.Errorf("check trigger: %w", err)
	}

	if !needsBackfill && hasTrigger {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`LOCK TABLE messages IN SHARE MODE`).Error; err != nil {
			return fmt.Errorf("lock messages: %w", err)
		}
		if !hasTrigger {
			if err := tx.Exec(counterTriggerSQL).Error; err != nil {
				return fmt.Errorf("create trigger: %w", err)
			}
		}
		if needsBackfill {
			if err := tx.Exec(counterBackfillSQL).Error; err != nil {
				return fmt.Errorf("backfill %s: %w", counterTableName, err)
			}
		}
		return nil
	})
}

// statsFromCounters answers GetStats from the rollup table.
//
// sum(bigint) is numeric in Postgres, so every aggregate is cast back to bigint
// to scan cleanly into int64.
func (m *messageRepository) statsFromCounters() (*MessageStats, error) {
	stats := &MessageStats{ByStatus: []StatKV{}, ByDay: []StatKV{}, TopSources: []StatKV{}}

	if err := m.db.Raw(`SELECT COALESCE(sum(total), 0)::bigint FROM ` + counterTableName).
		Row().Scan(&stats.Total); err != nil {
		return nil, err
	}
	if err := m.db.Raw(`SELECT status AS label, sum(total)::bigint AS total FROM ` + counterTableName +
		` GROUP BY status ORDER BY total DESC`).Scan(&stats.ByStatus).Error; err != nil {
		return nil, err
	}
	if err := m.db.Raw(`SELECT day AS label, sum(total)::bigint AS total FROM ` + counterTableName +
		` GROUP BY day ORDER BY label DESC LIMIT 14`).Scan(&stats.ByDay).Error; err != nil {
		return nil, err
	}
	if err := m.db.Raw(`SELECT source AS label, sum(total)::bigint AS total FROM ` + counterTableName +
		` GROUP BY source ORDER BY total DESC LIMIT 25`).Scan(&stats.TopSources).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// countByInstanceFromCounters answers CountByInstance from the rollup table.
func (m *messageRepository) countByInstanceFromCounters(instanceId string) (int64, error) {
	var total int64
	err := m.db.Raw(`SELECT COALESCE(sum(total), 0)::bigint FROM `+counterTableName+
		` WHERE instance_id = ?`, instanceId).Row().Scan(&total)
	return total, err
}

// countChatsByInstanceFromCounters answers CountChatsByInstance from the rollup
// table. The rollup has one row per (day, status, source), so the distinct
// conversation count is a distinct over its `source` column.
func (m *messageRepository) countChatsByInstanceFromCounters(instanceId string) (int64, error) {
	var total int64
	err := m.db.Raw(`SELECT count(DISTINCT source)::bigint FROM `+counterTableName+
		` WHERE instance_id = ?`, instanceId).Row().Scan(&total)
	return total, err
}
