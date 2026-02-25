CREATE TABLE IF NOT EXISTS workflow_definitions (
    name        TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    builtin     BOOLEAN NOT NULL DEFAULT 0,
    steps_json  TEXT NOT NULL DEFAULT '[]',
    params_json TEXT NOT NULL DEFAULT '[]',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

CREATE TABLE IF NOT EXISTS workflow_runs (
    id             TEXT PRIMARY KEY,
    workflow_name  TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    params_json    TEXT NOT NULL DEFAULT '{}',
    error          TEXT,
    created_at     TEXT NOT NULL,
    started_at     TEXT,
    finished_at    TEXT,
    FOREIGN KEY (workflow_name) REFERENCES workflow_definitions(name)
);

CREATE INDEX IF NOT EXISTS idx_workflow_runs_name ON workflow_runs(workflow_name);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);

CREATE TABLE IF NOT EXISTS workflow_run_steps (
    run_id      TEXT NOT NULL,
    step_name   TEXT NOT NULL,
    step_type   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    result_json TEXT NOT NULL DEFAULT '{}',
    task_id     TEXT,
    error       TEXT,
    started_at  TEXT,
    finished_at TEXT,
    PRIMARY KEY (run_id, step_name),
    FOREIGN KEY (run_id) REFERENCES workflow_runs(id)
);
