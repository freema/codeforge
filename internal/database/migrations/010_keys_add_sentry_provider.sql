-- SQLite does not support ALTER CHECK, so we recreate the table.
CREATE TABLE keys_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL CHECK(provider IN ('github', 'gitlab', 'sentry')),
    encrypted_token TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(provider, name)
);

INSERT INTO keys_new (id, name, provider, encrypted_token, scope, created_at)
    SELECT id, name, provider, encrypted_token, scope, created_at FROM keys;

DROP TABLE keys;

ALTER TABLE keys_new RENAME TO keys;

CREATE INDEX IF NOT EXISTS idx_keys_name ON keys(name);
