package tools

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToolDefinition_JSONRoundTrip(t *testing.T) {
	def := ToolDefinition{
		Name:        "sentry",
		Type:        ToolTypeMCP,
		Description: "Sentry error tracking",
		Version:     "1.0.0",
		MCPPackage:  "@sentry/mcp-server",
		MCPCommand:  "npx",
		MCPArgs:     []string{"--stdio"},
		RequiredConfig: []ConfigField{
			{Name: "auth_token", Description: "Sentry auth token", Type: "secret", EnvVar: "SENTRY_AUTH_TOKEN", Sensitive: true},
		},
		OptionalConfig: []ConfigField{
			{Name: "org", Description: "Sentry org slug", Type: "string", EnvVar: "SENTRY_ORG"},
		},
		Capabilities: []string{"error-tracking", "issues"},
		Builtin:      true,
		CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolDefinition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != def.Name {
		t.Errorf("Name = %q, want %q", got.Name, def.Name)
	}
	if got.Type != def.Type {
		t.Errorf("Type = %q, want %q", got.Type, def.Type)
	}
	if len(got.MCPArgs) != 1 || got.MCPArgs[0] != "--stdio" {
		t.Errorf("MCPArgs = %v, want [--stdio]", got.MCPArgs)
	}
	if len(got.RequiredConfig) != 1 || got.RequiredConfig[0].Sensitive != true {
		t.Error("RequiredConfig round-trip failed")
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities = %v, want 2 items", got.Capabilities)
	}
}

func TestToolDefinition_OmitemptyFields(t *testing.T) {
	def := ToolDefinition{
		Name:        "minimal",
		Type:        ToolTypeCustom,
		Description: "minimal tool",
		Builtin:     false,
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	for _, key := range []string{"version", "mcp_package", "mcp_command", "mcp_args", "required_config", "optional_config", "capabilities"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected %q to be omitted, but it was present", key)
		}
	}
}

func TestConfigField_JSONRoundTrip(t *testing.T) {
	field := ConfigField{
		Name:        "api_key",
		Description: "API key",
		Type:        "secret",
		EnvVar:      "API_KEY",
		Sensitive:   true,
	}

	data, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ConfigField
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != field {
		t.Errorf("got %+v, want %+v", got, field)
	}
}

func TestTaskTool_JSONRoundTrip(t *testing.T) {
	tt := TaskTool{
		Name:   "sentry",
		Config: map[string]string{"auth_token": "tok123"},
	}

	data, err := json.Marshal(tt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got TaskTool
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != tt.Name {
		t.Errorf("Name = %q, want %q", got.Name, tt.Name)
	}
	if got.Config["auth_token"] != "tok123" {
		t.Errorf("Config = %v, want auth_token=tok123", got.Config)
	}
}
