package task

import (
	"encoding/json"
	"time"

	gitpkg "github.com/freema/codeforge/internal/git"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	StatusPending             TaskStatus = "pending"
	StatusCloning             TaskStatus = "cloning"
	StatusRunning             TaskStatus = "running"
	StatusCompleted           TaskStatus = "completed"
	StatusFailed              TaskStatus = "failed"
	StatusAwaitingInstruction TaskStatus = "awaiting_instruction"
	StatusCreatingPR          TaskStatus = "creating_pr"
	StatusPRCreated           TaskStatus = "pr_created"
)

// Task represents a code task in the system.
type Task struct {
	ID          string     `json:"id"`
	Status      TaskStatus `json:"status"`
	RepoURL     string     `json:"repo_url"`
	ProviderKey string     `json:"provider_key,omitempty"`
	AccessToken string     `json:"-"` // NEVER in API responses
	Prompt      string     `json:"prompt"`
	CallbackURL string     `json:"callback_url,omitempty"`
	Config      *TaskConfig `json:"config,omitempty"`

	// Result fields
	Result         string          `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	ChangesSummary *gitpkg.ChangesSummary `json:"changes_summary,omitempty"`
	Usage          *UsageInfo      `json:"usage,omitempty"`

	// Iteration tracking (Phase 3)
	Iteration     int    `json:"iteration"`
	CurrentPrompt string `json:"current_prompt,omitempty"`

	// Git integration (Phase 2)
	Branch   string `json:"branch,omitempty"`
	PRNumber int    `json:"pr_number,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`

	// Observability (Phase 6)
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

// TaskConfig holds per-task configuration overrides.
type TaskConfig struct {
	TimeoutSeconds int         `json:"timeout_seconds,omitempty"`
	CLI            string      `json:"cli,omitempty"`
	AIModel        string      `json:"ai_model,omitempty"`
	AIApiKey       string      `json:"-"` // NEVER in responses
	MaxTurns       int         `json:"max_turns,omitempty"`
	TargetBranch   string      `json:"target_branch,omitempty"`
	MaxBudgetUSD   float64     `json:"max_budget_usd,omitempty"`
	MCPServers     []MCPServer `json:"mcp_servers,omitempty"`
}

// MCPServer defines an MCP server configuration.
type MCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// Iteration stores result data for a single iteration (Phase 3).
type Iteration struct {
	Number    int             `json:"number"`
	Prompt    string          `json:"prompt"`
	Result    string          `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Status    TaskStatus      `json:"status"`
	Changes   *gitpkg.ChangesSummary `json:"changes,omitempty"`
	Usage     *UsageInfo      `json:"usage,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
}

// MarshalConfig serializes TaskConfig to JSON string for Redis storage.
func MarshalConfig(cfg *TaskConfig) string {
	if cfg == nil {
		return ""
	}
	b, _ := json.Marshal(cfg)
	return string(b)
}

// UnmarshalConfig deserializes TaskConfig from JSON string.
func UnmarshalConfig(data string) *TaskConfig {
	if data == "" {
		return nil
	}
	var cfg TaskConfig
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
