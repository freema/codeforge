CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    status          TEXT NOT NULL DEFAULT 'pending',
    repo_url        TEXT NOT NULL,
    provider_key    TEXT NOT NULL DEFAULT '',
    prompt          TEXT NOT NULL,
    callback_url    TEXT NOT NULL DEFAULT '',
    config_json     TEXT NOT NULL DEFAULT '{}',

    -- Result
    result          TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    changes_json    TEXT NOT NULL DEFAULT '{}',
    usage_json      TEXT NOT NULL DEFAULT '{}',

    -- Iteration
    iteration       INTEGER NOT NULL DEFAULT 1,
    current_prompt  TEXT NOT NULL DEFAULT '',

    -- Git / PR
    branch          TEXT NOT NULL DEFAULT '',
    pr_number       INTEGER NOT NULL DEFAULT 0,
    pr_url          TEXT NOT NULL DEFAULT '',

    -- Observability
    trace_id        TEXT NOT NULL DEFAULT '',

    -- Timestamps
    created_at      TEXT NOT NULL,
    started_at      TEXT,
    finished_at     TEXT,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_repo_url ON tasks(repo_url);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);

CREATE TABLE IF NOT EXISTS task_iterations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL,
    number      INTEGER NOT NULL,
    prompt      TEXT NOT NULL,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    changes_json TEXT NOT NULL DEFAULT '{}',
    usage_json  TEXT NOT NULL DEFAULT '{}',
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    UNIQUE(task_id, number)
);

CREATE INDEX IF NOT EXISTS idx_task_iterations_task_id ON task_iterations(task_id);
