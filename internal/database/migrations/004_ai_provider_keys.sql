-- Allow AI provider keys (Anthropic, OpenAI) alongside existing git/sentry providers.
-- SQLite does not support ALTER TABLE ... ALTER CONSTRAINT, so we drop and recreate.
-- The CHECK constraint is only on INSERT — existing rows are unaffected.
-- We use a CREATE TABLE + copy approach for safety.

CREATE TABLE IF NOT EXISTS keys_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL CHECK(provider IN ('github', 'gitlab', 'sentry', 'anthropic', 'openai')),
    encrypted_token TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '',
    base_url        TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    UNIQUE(provider, name)
);

INSERT OR IGNORE INTO keys_new (id, name, provider, encrypted_token, scope, base_url, created_at)
SELECT id, name, provider, encrypted_token, scope, base_url, created_at FROM keys;

DROP TABLE keys;
ALTER TABLE keys_new RENAME TO keys;

CREATE INDEX IF NOT EXISTS idx_keys_name ON keys(name);
