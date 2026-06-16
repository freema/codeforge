package tenant

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"time"
)

// Store provides CRUD operations for tenants, usage logs, and key pool.
type Store struct {
	db *sql.DB
}

// NewStore creates a new tenant store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateTenant inserts a new tenant.
func (s *Store) CreateTenant(ctx context.Context, t *Tenant) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, slug, tier, api_token_hash, max_sessions_per_day, max_concurrent_sessions, max_budget_usd_per_session, allowed_clis, allowed_models)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Slug, t.Tier, t.APITokenHash,
		t.MaxSessionsPerDay, t.MaxConcurrentSessions, t.MaxBudgetUSDPerSession,
		t.AllowedCLIs, t.AllowedModels,
	)
	if err != nil {
		return fmt.Errorf("creating tenant: %w", err)
	}
	return nil
}

// GetTenant returns a tenant by ID.
func (s *Store) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	return s.scanTenant(s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, tier, api_token_hash, max_sessions_per_day, max_concurrent_sessions, max_budget_usd_per_session, allowed_clis, allowed_models, created_at, updated_at
		FROM tenants WHERE id = ?`, id))
}

// GetTenantByTokenHash returns a tenant by its API token hash.
func (s *Store) GetTenantByTokenHash(ctx context.Context, hash string) (*Tenant, error) {
	return s.scanTenant(s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, tier, api_token_hash, max_sessions_per_day, max_concurrent_sessions, max_budget_usd_per_session, allowed_clis, allowed_models, created_at, updated_at
		FROM tenants WHERE api_token_hash = ?`, hash))
}

// ListTenants returns all tenants.
func (s *Store) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, slug, tier, api_token_hash, max_sessions_per_day, max_concurrent_sessions, max_budget_usd_per_session, allowed_clis, allowed_models, created_at, updated_at
		FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*Tenant
	for rows.Next() {
		t, err := s.scanTenantRow(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// UpdateTenant updates a tenant's mutable fields.
func (s *Store) UpdateTenant(ctx context.Context, t *Tenant) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tenants SET name = ?, tier = ?, max_sessions_per_day = ?, max_concurrent_sessions = ?, max_budget_usd_per_session = ?, allowed_clis = ?, allowed_models = ?, updated_at = ?
		WHERE id = ?`,
		t.Name, t.Tier, t.MaxSessionsPerDay, t.MaxConcurrentSessions, t.MaxBudgetUSDPerSession,
		t.AllowedCLIs, t.AllowedModels, time.Now().UTC().Format("2006-01-02T15:04:05.000"), t.ID,
	)
	if err != nil {
		return fmt.Errorf("updating tenant: %w", err)
	}
	return nil
}

// DeleteTenant removes a tenant by ID.
func (s *Store) DeleteTenant(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM tenants WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting tenant: %w", err)
	}
	return nil
}

// LogUsage records a usage entry. Generates an ID if one is not set.
func (s *Store) LogUsage(ctx context.Context, log *UsageLog) error {
	if log.ID == "" {
		log.ID = generateID()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_logs (id, tenant_id, session_id, cli, model, input_tokens, output_tokens, estimated_cost_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.TenantID, log.SessionID, log.CLI, log.Model,
		log.InputTokens, log.OutputTokens, log.EstimatedCostUSD,
	)
	if err != nil {
		return fmt.Errorf("logging usage: %w", err)
	}
	return nil
}

// GetUsageSummary returns aggregated usage for a tenant within a time window.
func (s *Store) GetUsageSummary(ctx context.Context, tenantID string, since time.Time) (*UsageSummary, error) {
	var summary UsageSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(estimated_cost_usd), 0)
		FROM usage_logs WHERE tenant_id = ? AND created_at >= ?`,
		tenantID, since.Format("2006-01-02T15:04:05.000"),
	).Scan(&summary.TotalSessions, &summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD)
	if err != nil {
		return nil, fmt.Errorf("getting usage summary: %w", err)
	}
	return &summary, nil
}

// CountDailySessions returns the number of DISTINCT sessions a tenant ran today.
// Counting distinct session_id (not rows) means a multi-turn session — which logs
// usage once per iteration — counts as a single session against the daily quota.
func (s *Store) CountDailySessions(ctx context.Context, tenantID string) (int, error) {
	var count int
	today := time.Now().UTC().Format("2006-01-02")
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT session_id) FROM usage_logs
		WHERE tenant_id = ? AND created_at >= ?`,
		tenantID, today+"T00:00:00.000",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting daily sessions: %w", err)
	}
	return count, nil
}

// AddKeyPoolEntry adds a managed key to the pool.
func (s *Store) AddKeyPoolEntry(ctx context.Context, entry *KeyPoolEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO key_pool (id, provider, encrypted_token, weight, active)
		VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.Provider, entry.EncryptedToken, entry.Weight, entry.Active,
	)
	if err != nil {
		return fmt.Errorf("adding key pool entry: %w", err)
	}
	return nil
}

// ListKeyPool returns all key pool entries for a provider (or all if provider is empty).
func (s *Store) ListKeyPool(ctx context.Context, provider string) ([]*KeyPoolEntry, error) {
	var rows *sql.Rows
	var err error
	if provider != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, provider, encrypted_token, weight, active, created_at
			FROM key_pool WHERE provider = ? ORDER BY created_at`, provider)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, provider, encrypted_token, weight, active, created_at
			FROM key_pool ORDER BY provider, created_at`)
	}
	if err != nil {
		return nil, fmt.Errorf("listing key pool: %w", err)
	}
	defer rows.Close()

	var entries []*KeyPoolEntry
	for rows.Next() {
		var e KeyPoolEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Provider, &e.EncryptedToken, &e.Weight, &e.Active, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning key pool entry: %w", err)
		}
		e.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// DeleteKeyPoolEntry removes a key from the pool.
func (s *Store) DeleteKeyPoolEntry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM key_pool WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting key pool entry: %w", err)
	}
	return nil
}

// GetActiveKeyForProvider returns a weighted-random active key for a provider.
// Selection is proportional to each entry's weight (weight <= 0 is treated as 1),
// implementing load balancing across the operator's managed keys.
func (s *Store) GetActiveKeyForProvider(ctx context.Context, provider string) (*KeyPoolEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, encrypted_token, weight, active, created_at
		FROM key_pool WHERE provider = ? AND active = 1`, provider)
	if err != nil {
		return nil, fmt.Errorf("getting active keys for provider %s: %w", provider, err)
	}
	defer rows.Close()

	var entries []*KeyPoolEntry
	totalWeight := 0
	for rows.Next() {
		var e KeyPoolEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Provider, &e.EncryptedToken, &e.Weight, &e.Active, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning key pool entry: %w", err)
		}
		e.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
		totalWeight += effectiveWeight(e.Weight)
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no active key for provider %s", provider)
	}

	// Weighted random pick.
	r := randIntn(totalWeight)
	for _, e := range entries {
		w := effectiveWeight(e.Weight)
		if r < w {
			return e, nil
		}
		r -= w
	}
	return entries[len(entries)-1], nil
}

func effectiveWeight(w int) int {
	if w <= 0 {
		return 1
	}
	return w
}

// randIntn returns a cryptographically-sourced random int in [0, n).
// Used for key-pool load balancing; falls back to 0 on the (unreachable) error path.
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func (s *Store) scanTenant(row *sql.Row) (*Tenant, error) {
	var t Tenant
	var createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Tier, &t.APITokenHash,
		&t.MaxSessionsPerDay, &t.MaxConcurrentSessions, &t.MaxBudgetUSDPerSession,
		&t.AllowedCLIs, &t.AllowedModels, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning tenant: %w", err)
	}
	t.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
	t.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05.000", updatedAt)
	return &t, nil
}

func (s *Store) scanTenantRow(rows *sql.Rows) (*Tenant, error) {
	var t Tenant
	var createdAt, updatedAt string
	err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Tier, &t.APITokenHash,
		&t.MaxSessionsPerDay, &t.MaxConcurrentSessions, &t.MaxBudgetUSDPerSession,
		&t.AllowedCLIs, &t.AllowedModels, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning tenant row: %w", err)
	}
	t.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
	t.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05.000", updatedAt)
	return &t, nil
}
