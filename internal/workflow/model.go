package workflow

import (
	"encoding/json"
	"time"
)

// StepType defines what kind of action a workflow step performs.
type StepType string

const (
	StepTypeSession StepType = "session"
)

// WorkflowDefinition describes a reusable workflow template.
type WorkflowDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Builtin     bool                  `json:"builtin"`
	Steps       []StepDefinition      `json:"steps"`
	Parameters  []ParameterDefinition `json:"parameters"`
	CreatedAt   time.Time             `json:"created_at,omitempty"`
}

// StepDefinition describes a single step in a workflow.
type StepDefinition struct {
	Name   string          `json:"name"`
	Type   StepType        `json:"type"`
	Config json.RawMessage `json:"config"`
}

// SessionStepConfig is the configuration for a session step.
type SessionStepConfig struct {
	RepoURL      string `json:"repo_url"`
	Prompt       string `json:"prompt"`
	SessionType  string `json:"session_type,omitempty"`
	ProviderKey  string `json:"provider_key,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	CLI          string `json:"cli,omitempty"`
	AIModel      string `json:"ai_model,omitempty"`
	SourceBranch string `json:"source_branch,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`

	// PR review fields
	PRNumber   int    `json:"pr_number,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`

	// Tool/MCP overrides
	Tools      json.RawMessage `json:"tools,omitempty"`
	MCPServers json.RawMessage `json:"mcp_servers,omitempty"`
	ToolKeyRef string          `json:"tool_key_ref,omitempty"`
}

// ParameterDefinition describes a workflow input parameter.
type ParameterDefinition struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// MarshalMapJSON serializes a map to a JSON string.
func MarshalMapJSON(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// UnmarshalMapJSON deserializes a JSON string to a map.
func UnmarshalMapJSON(data string) map[string]string {
	if data == "" || data == "{}" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil
	}
	return m
}
