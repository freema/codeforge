-- Tenant ownership of sessions — lets the subscription model scope session
-- listing/access to the owning tenant. Empty string = not owned by a tenant
-- (operator/BYOK session), preserving existing behaviour.
ALTER TABLE sessions ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id);
