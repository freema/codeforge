package tenant

import "time"

// Tier constants for subscription levels.
const (
	TierFree       = "free"
	TierPro        = "pro"
	TierEnterprise = "enterprise"
)

// Tenant represents a registered organization with subscription-based access.
type Tenant struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Tier                  string    `json:"tier"`
	APITokenHash          string    `json:"-"`
	MaxSessionsPerDay     int       `json:"max_sessions_per_day"`
	MaxConcurrentSessions int       `json:"max_concurrent_sessions"`
	MaxBudgetUSDPerSession float64  `json:"max_budget_usd_per_session"`
	AllowedCLIs           string    `json:"allowed_clis"`
	AllowedModels         *string   `json:"allowed_models,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// UsageLog records a session's resource usage for a tenant.
type UsageLog struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	SessionID       string    `json:"session_id"`
	CLI             string    `json:"cli"`
	Model           string    `json:"model"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	EstimatedCostUSD float64  `json:"estimated_cost_usd"`
	CreatedAt       time.Time `json:"created_at"`
}

// KeyPoolEntry represents a managed API key in the operator's key pool.
type KeyPoolEntry struct {
	ID             string    `json:"id"`
	Provider       string    `json:"provider"`
	EncryptedToken string    `json:"-"`
	Weight         int       `json:"weight"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"created_at"`
}

// UsageSummary holds aggregated usage stats for a tenant.
type UsageSummary struct {
	TotalSessions    int     `json:"total_sessions"`
	TotalInputTokens int     `json:"total_input_tokens"`
	TotalOutputTokens int    `json:"total_output_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}
