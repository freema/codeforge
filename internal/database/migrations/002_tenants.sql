-- Tenant management for subscription-based access (dual-auth alongside API keys).

CREATE TABLE IF NOT EXISTS tenants (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL,
    slug                    TEXT UNIQUE NOT NULL,
    tier                    TEXT NOT NULL DEFAULT 'free' CHECK(tier IN ('free', 'pro', 'enterprise')),
    api_token_hash          TEXT NOT NULL,
    max_sessions_per_day    INTEGER NOT NULL DEFAULT 10,
    max_concurrent_sessions INTEGER NOT NULL DEFAULT 2,
    max_budget_usd_per_session REAL NOT NULL DEFAULT 1.0,
    allowed_clis            TEXT NOT NULL DEFAULT '["claude-code"]',
    allowed_models          TEXT,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    updated_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_tenants_token_hash ON tenants(api_token_hash);
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);

-- Managed key pool for subscription tenants (operator-owned API keys).
CREATE TABLE IF NOT EXISTS key_pool (
    id              TEXT PRIMARY KEY,
    provider        TEXT NOT NULL,
    encrypted_token TEXT NOT NULL,
    weight          INTEGER NOT NULL DEFAULT 1,
    active          INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_key_pool_provider ON key_pool(provider, active);

-- Usage tracking per tenant.
CREATE TABLE IF NOT EXISTS usage_logs (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id        TEXT NOT NULL,
    cli               TEXT NOT NULL,
    model             TEXT NOT NULL DEFAULT '',
    input_tokens      INTEGER NOT NULL DEFAULT 0,
    output_tokens     INTEGER NOT NULL DEFAULT 0,
    estimated_cost_usd REAL NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_usage_tenant_date ON usage_logs(tenant_id, created_at);
