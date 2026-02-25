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
    required_config TEXT NOT NULL DEFAULT '[]',
    optional_config TEXT NOT NULL DEFAULT '[]',
    capabilities    TEXT NOT NULL DEFAULT '[]',
    builtin         INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(scope, name)
);
CREATE INDEX IF NOT EXISTS idx_tools_scope ON tools(scope);
