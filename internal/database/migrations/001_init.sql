-- Unified schema: all tables with final naming conventions.

-- Provider keys (GitHub, GitLab, Sentry, Anthropic, OpenAI tokens)
CREATE TABLE IF NOT EXISTS keys (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL CHECK(provider IN ('github', 'gitlab', 'sentry', 'anthropic', 'openai')),
    encrypted_token TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '',
    base_url        TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(provider, name)
);
CREATE INDEX IF NOT EXISTS idx_keys_name ON keys(name);

-- MCP server configurations
CREATE TABLE IF NOT EXISTS mcp_servers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'global',
    package    TEXT NOT NULL,
    args       TEXT NOT NULL DEFAULT '[]',
    env        TEXT NOT NULL DEFAULT '{}',
    transport  TEXT NOT NULL DEFAULT 'stdio',
    command    TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL DEFAULT '',
    headers    TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(scope, name)
);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_scope ON mcp_servers(scope);

-- Workflow definitions and runs
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

-- Workflow configs (saved parameter presets)
CREATE TABLE IF NOT EXISTS workflow_configs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL UNIQUE,
    workflow        TEXT NOT NULL,
    params          TEXT NOT NULL DEFAULT '{}',
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

-- Sessions
CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,
    status          TEXT NOT NULL DEFAULT 'pending',
    repo_url        TEXT NOT NULL,
    provider_key    TEXT NOT NULL DEFAULT '',
    prompt          TEXT NOT NULL,
    session_type    TEXT NOT NULL DEFAULT 'code',
    callback_url    TEXT NOT NULL DEFAULT '',
    config_json     TEXT NOT NULL DEFAULT '{}',

    -- Result
    result          TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    changes_json    TEXT NOT NULL DEFAULT '{}',
    usage_json      TEXT NOT NULL DEFAULT '{}',
    review_result_json TEXT NOT NULL DEFAULT '{}',

    -- Iteration
    iteration       INTEGER NOT NULL DEFAULT 1,
    current_prompt  TEXT NOT NULL DEFAULT '',

    -- Git / PR
    branch          TEXT NOT NULL DEFAULT '',
    pr_number       INTEGER NOT NULL DEFAULT 0,
    pr_url          TEXT NOT NULL DEFAULT '',

    -- Workflow
    workflow_run_id TEXT NOT NULL DEFAULT '',

    -- Observability
    trace_id        TEXT NOT NULL DEFAULT '',

    -- Timestamps
    created_at      TEXT NOT NULL,
    started_at      TEXT,
    finished_at     TEXT,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_repo_url ON sessions(repo_url);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_workflow_run_id ON sessions(workflow_run_id);

CREATE TABLE IF NOT EXISTS session_iterations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    number      INTEGER NOT NULL,
    prompt      TEXT NOT NULL,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    changes_json TEXT NOT NULL DEFAULT '{}',
    usage_json  TEXT NOT NULL DEFAULT '{}',
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id),
    UNIQUE(session_id, number)
);
CREATE INDEX IF NOT EXISTS idx_session_iterations_session_id ON session_iterations(session_id);

-- Tools (MCP, builtin, custom)
CREATE TABLE IF NOT EXISTS tools (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT 'global',
    type            TEXT NOT NULL CHECK(type IN ('mcp', 'builtin', 'custom')),
    description     TEXT NOT NULL DEFAULT '',
    version         TEXT NOT NULL DEFAULT '',
    mcp_package     TEXT NOT NULL DEFAULT '',
    mcp_command     TEXT NOT NULL DEFAULT '',
    mcp_args        TEXT NOT NULL DEFAULT '[]',
    mcp_transport   TEXT NOT NULL DEFAULT 'stdio',
    mcp_url         TEXT NOT NULL DEFAULT '',
    required_config TEXT NOT NULL DEFAULT '[]',
    optional_config TEXT NOT NULL DEFAULT '[]',
    capabilities    TEXT NOT NULL DEFAULT '[]',
    builtin         INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(scope, name)
);
CREATE INDEX IF NOT EXISTS idx_tools_scope ON tools(scope);
