package tools

import "time"

// ToolType identifies the kind of tool.
type ToolType string

const (
	ToolTypeMCP     ToolType = "mcp"
	ToolTypeBuiltin ToolType = "builtin"
	ToolTypeCustom  ToolType = "custom"
)

// ToolDefinition describes a tool that can be attached to tasks.
type ToolDefinition struct {
	Name           string        `json:"name"`
	Type           ToolType      `json:"type"`
	Description    string        `json:"description"`
	Version        string        `json:"version,omitempty"`
	MCPPackage     string        `json:"mcp_package,omitempty"`
	MCPCommand     string        `json:"mcp_command,omitempty"`
	MCPArgs        []string      `json:"mcp_args,omitempty"`
	RequiredConfig []ConfigField `json:"required_config,omitempty"`
	OptionalConfig []ConfigField `json:"optional_config,omitempty"`
	Capabilities   []string      `json:"capabilities,omitempty"`
	Builtin        bool          `json:"builtin"`
	CreatedAt      time.Time     `json:"created_at,omitempty"`
}

// ConfigField describes a single configuration parameter for a tool.
type ConfigField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	EnvVar      string `json:"env_var"`
	Sensitive   bool   `json:"sensitive"`
}

// ToolInstance is a resolved tool definition paired with user-supplied config values.
type ToolInstance struct {
	Definition *ToolDefinition
	Config     map[string]string
}

// TaskTool is the per-task tool request (used in CreateTaskRequest).
type TaskTool struct {
	Name   string            `json:"name" validate:"required"`
	Config map[string]string `json:"config,omitempty"`
}
