package session

import (
	"encoding/json"
	"time"

	"github.com/freema/codeforge/internal/review"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tools"
)

// Status represents the current state of a session.
type Status string

const (
	StatusPending             Status = "pending"
	StatusCloning             Status = "cloning"
	StatusRunning             Status = "running"
	StatusCompleted           Status = "completed"
	StatusFailed              Status = "failed"
	StatusAwaitingInstruction Status = "awaiting_instruction"
	StatusReviewing           Status = "reviewing"
	StatusCreatingPR          Status = "creating_pr"
	StatusPRCreated           Status = "pr_created"
)

// Session represents a code session in the system.
type Session struct {
	ID          string      `json:"id"`
	Status      Status  `json:"status"`
	RepoURL     string      `json:"repo_url"`
	ProviderKey string      `json:"provider_key,omitempty"`
	AccessToken string      `json:"-"` // NEVER in API responses
	Prompt      string      `json:"prompt"`
	SessionType    string      `json:"session_type,omitempty"`
	CallbackURL string      `json:"callback_url,omitempty"`
	Config      *Config `json:"config,omitempty"`

	// Result fields
	Result         string                 `json:"result,omitempty"`
	Error          string                 `json:"error,omitempty"`
	ChangesSummary *gitpkg.ChangesSummary `json:"changes_summary,omitempty"`
	Usage          *UsageInfo             `json:"usage,omitempty"`
	ReviewResult   *review.ReviewResult   `json:"review_result,omitempty"`

	// Iteration tracking
	Iteration     int         `json:"iteration"`
	CurrentPrompt string      `json:"current_prompt,omitempty"` // follow-up prompt for current iteration (set by Instruct)
	Iterations    []Iteration `json:"iterations,omitempty"`     // populated on demand via ?include=iterations

	// Git integration — PRNumber is the PR created by CodeForge (via create-pr).
	// For the input PR number on pr_review sessions, see Config.PRNumber.
	Branch   string `json:"branch,omitempty"`
	PRNumber int    `json:"pr_number,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`

	// Review params (set by StartReviewAsync, consumed by executor)
	ReviewCLI   string `json:"-"`
	ReviewModel string `json:"-"`

	// Metadata — optional key-value data (sentry URL, ticket link, etc.)
	Metadata map[string]string `json:"metadata,omitempty"`

	// Workflow linkage
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	// Observability
	TraceID string `json:"trace_id,omitempty"`

	// Timestamps
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// UsageInfo tracks token usage and duration.
type UsageInfo struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	DurationSeconds int `json:"duration_seconds"`
}

// Config holds per-session configuration overrides.
type Config struct {
	TimeoutSeconds  int              `json:"timeout_seconds,omitempty"`
	CLI             string           `json:"cli,omitempty"`
	AIModel         string           `json:"ai_model,omitempty"`
	AIApiKey        string           `json:"-"` // NEVER in responses (custom UnmarshalJSON accepts it)
	MaxTurns        int              `json:"max_turns,omitempty"`
	SourceBranch    string           `json:"source_branch,omitempty"` // branch to clone/checkout
	TargetBranch    string           `json:"target_branch,omitempty"`
	MaxBudgetUSD    float64          `json:"max_budget_usd,omitempty"`
	MCPServers      []MCPServer      `json:"mcp_servers,omitempty"`
	Tools              []tools.SessionTool `json:"tools,omitempty"`
	WorkspaceSessionID string              `json:"workspace_session_id,omitempty"` // reuse workspace from another session
	PRNumber           int              `json:"pr_number,omitempty"`            // input PR/MR number to review (for pr_review sessions)
	OutputMode         string           `json:"output_mode,omitempty"`          // "post_comments" or "api_only" (for pr_review sessions)
	AutoReviewAfterFix bool             `json:"auto_review_after_fix,omitempty"` // auto-start review after each fix iteration
	AutoPostReview     bool             `json:"auto_post_review,omitempty"`      // auto-post review result to MR comments
}

// UnmarshalJSON accepts ai_api_key from JSON input while json:"-" keeps it hidden in output.
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		AIApiKey string `json:"ai_api_key,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	c.AIApiKey = aux.AIApiKey
	return nil
}

// MCPServer defines an MCP server configuration.
type MCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"` // "stdio" (default) or "http"
	// stdio fields
	Package string            `json:"package,omitempty"` // NPM package or binary path
	Command string            `json:"command,omitempty"` // e.g. "npx", "uvx", "docker"
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	// http fields
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Iteration stores result data for a single iteration.
type Iteration struct {
	Number    int                    `json:"number"`
	Prompt    string                 `json:"prompt"`
	Result    string                 `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Status    Status             `json:"status"`
	Changes   *gitpkg.ChangesSummary `json:"changes,omitempty"`
	Usage     *UsageInfo             `json:"usage,omitempty"`
	StartedAt time.Time              `json:"started_at"`
	EndedAt   *time.Time             `json:"ended_at,omitempty"`
}

// MarshalConfig serializes Config to JSON string for Redis storage.
func MarshalConfig(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	b, _ := json.Marshal(cfg)
	return string(b)
}

// UnmarshalConfig deserializes Config from JSON string.
func UnmarshalConfig(data string) *Config {
	if data == "" {
		return nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return nil
	}
	return &cfg
}

// MarshalChangesSummary serializes ChangesSummary to JSON string for Redis.
func MarshalChangesSummary(cs *gitpkg.ChangesSummary) string {
	if cs == nil {
		return ""
	}
	b, _ := json.Marshal(cs)
	return string(b)
}

// UnmarshalChangesSummary deserializes ChangesSummary from JSON string.
func UnmarshalChangesSummary(data string) *gitpkg.ChangesSummary {
	if data == "" {
		return nil
	}
	var cs gitpkg.ChangesSummary
	if err := json.Unmarshal([]byte(data), &cs); err != nil {
		return nil
	}
	return &cs
}

// MarshalUsageInfo serializes UsageInfo to JSON string for Redis.
func MarshalUsageInfo(u *UsageInfo) string {
	if u == nil {
		return ""
	}
	b, _ := json.Marshal(u)
	return string(b)
}

// UnmarshalUsageInfo deserializes UsageInfo from JSON string.
func UnmarshalUsageInfo(data string) *UsageInfo {
	if data == "" {
		return nil
	}
	var u UsageInfo
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return nil
	}
	return &u
}
