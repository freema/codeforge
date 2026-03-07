package workflow

import (
	"encoding/json"
	"time"
)

// StepType defines what kind of action a workflow step performs.
type StepType string

const (
	StepTypeFetch  StepType = "fetch"
	StepTypeTask   StepType = "task"
	StepTypeAction StepType = "action"
)

// RunStatus represents the current state of a workflow run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

// StepStatus represents the current state of a workflow step execution.
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

// ActionKind identifies a built-in action.
type ActionKind string

const (
	ActionCreatePR ActionKind = "create_pr"
	ActionNotify   ActionKind = "notify"
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

// FetchConfig is the configuration for a fetch step.
type FetchConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	KeyName string            `json:"key_name,omitempty"`
	Outputs map[string]string `json:"outputs"` // field name → JSONPath expression
}

// TaskStepConfig is the configuration for a task step.
type TaskStepConfig struct {
	RepoURL          string `json:"repo_url"`
	Prompt           string `json:"prompt"`
	TaskType         string `json:"task_type,omitempty"`          // task type override: "code", "plan", "review"
	ProviderKey      string `json:"provider_key,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	CLI              string `json:"cli,omitempty"`                // CLI runner override (e.g. "claude-code", "codex")
	AIModel          string `json:"ai_model,omitempty"`           // AI model override
	SourceBranch     string `json:"source_branch,omitempty"`      // branch to clone/checkout
	WorkspaceTaskRef string `json:"workspace_task_ref,omitempty"` // step name whose workspace to reuse
}

// ActionConfig is the configuration for an action step.
type ActionConfig struct {
	Kind        ActionKind `json:"kind"`
	TaskStepRef string     `json:"task_step_ref,omitempty"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
}

// ParameterDefinition describes a workflow input parameter.
type ParameterDefinition struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// WorkflowRun represents a single execution of a workflow.
type WorkflowRun struct {
	ID           string            `json:"id"`
	WorkflowName string            `json:"workflow_name"`
	Status       RunStatus         `json:"status"`
	Params       map[string]string `json:"params,omitempty"`
	Error        string            `json:"error,omitempty"`
	Steps        []WorkflowRunStep `json:"steps,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
}

// WorkflowRunStep records the execution state of a single step within a run.
type WorkflowRunStep struct {
	RunID      string            `json:"run_id"`
	StepName   string            `json:"step_name"`
	StepType   StepType          `json:"step_type"`
	Status     StepStatus        `json:"status"`
	Result     map[string]string `json:"result,omitempty"`
	TaskID     string            `json:"task_id,omitempty"`
	Error      string            `json:"error,omitempty"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
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
