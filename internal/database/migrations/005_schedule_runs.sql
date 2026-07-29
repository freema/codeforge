-- Schedule hardening: per-occurrence run history + failure tracking + timezone.
CREATE TABLE IF NOT EXISTS schedule_runs (
    id          TEXT PRIMARY KEY,
    schedule_id TEXT NOT NULL,
    session_id  TEXT,
    "trigger"   TEXT NOT NULL, -- 'cron' | 'manual'
    status      TEXT NOT NULL, -- 'fired' | 'fire_failed' | 'skipped_overlap' | 'session_completed' | 'session_failed'
    error       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_schedule_runs_schedule ON schedule_runs(schedule_id);
CREATE INDEX IF NOT EXISTS idx_schedule_runs_session ON schedule_runs(session_id);

ALTER TABLE schedules ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedules ADD COLUMN disabled_reason TEXT;
ALTER TABLE schedules ADD COLUMN timezone TEXT;
