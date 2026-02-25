CREATE TABLE IF NOT EXISTS keys (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL CHECK(provider IN ('github', 'gitlab')),
    encrypted_token TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(provider, name)
);

CREATE INDEX IF NOT EXISTS idx_keys_name ON keys(name);
