-- Scheduled (cron) sessions: recurring session templates fired by the scheduler.
CREATE TABLE IF NOT EXISTS schedules (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    cron            TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    session_request TEXT NOT NULL, -- JSON CreateSessionRequest template
    last_run_at     TEXT,
    last_session_id TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON schedules(enabled);
