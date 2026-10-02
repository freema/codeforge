package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/keys"
)

// mockRegistry is a test double for Registry.
type mockRegistry struct {
	tools map[string]map[string]ToolDefinition // scope → name → def
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{tools: make(map[string]map[string]ToolDefinition)}
}

func (m *mockRegistry) add(scope string, def ToolDefinition) {
	if m.tools[scope] == nil {
		m.tools[scope] = make(map[string]ToolDefinition)
	}
	m.tools[scope][def.Name] = def
}

func (m *mockRegistry) Create(_ context.Context, scope string, def ToolDefinition) error {
	if m.tools[scope] == nil {
		m.tools[scope] = make(map[string]ToolDefinition)
	}
	if _, exists := m.tools[scope][def.Name]; exists {
		return apperror.Conflict("tool '%s' already exists", def.Name)
	}
	m.tools[scope][def.Name] = def
	return nil
}

func (m *mockRegistry) Get(_ context.Context, scope, name string) (*ToolDefinition, error) {
	if defs, ok := m.tools[scope]; ok {
		if def, ok := defs[name]; ok {
			return &def, nil
		}
	}
	return nil, apperror.NotFound("tool '%s' not found", name)
}

func (m *mockRegistry) List(_ context.Context, scope string) ([]ToolDefinition, error) {
	var result []ToolDefinition
	if defs, ok := m.tools[scope]; ok {
		for _, def := range defs {
			result = append(result, def)
		}
	}
	return result, nil
}

func (m *mockRegistry) Delete(_ context.Context, scope, name string) error {
	if defs, ok := m.tools[scope]; ok {
		if _, ok := defs[name]; ok {
			delete(defs, name)
			return nil
		}
	}
	return apperror.NotFound("tool '%s' not found", name)
}

func TestResolver_GlobalFound(t *testing.T) {
	reg := newMockRegistry()
	reg.add("global", ToolDefinition{
		Name:       "custom-tool",
		Type:       ToolTypeCustom,
		MCPPackage: "my-package",
	})

	resolver := NewResolver(reg)
	instances, err := resolver.Resolve(context.Background(), "", []SessionTool{
		{Name: "custom-tool"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Definition.Name != "custom-tool" {
		t.Errorf("Name = %q, want custom-tool", instances[0].Definition.Name)
	}
}

func TestResolver_BuiltinFallback(t *testing.T) {
	reg := newMockRegistry() // empty registry
	resolver := NewResolver(reg)

	instances, err := resolver.Resolve(context.Background(), "", []SessionTool{
		{Name: "sentry", Config: map[string]string{"auth_token": "tok"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Definition.MCPPackage != "@sentry/mcp-server" {
		t.Errorf("MCPPackage = %q, want @sentry/mcp-server", instances[0].Definition.MCPPackage)
	}
	if instances[0].Definition.MCPCommand != "npx" {
		t.Errorf("MCPCommand = %q, want npx", instances[0].Definition.MCPCommand)
	}
}

func TestResolver_ProjectScopePriority(t *testing.T) {
	reg := newMockRegistry()
	reg.add("global", ToolDefinition{Name: "tool", Type: ToolTypeCustom, Description: "global"})
	reg.add("project-1", ToolDefinition{Name: "tool", Type: ToolTypeCustom, Description: "project"})

	resolver := NewResolver(reg)
	instances, err := resolver.Resolve(context.Background(), "project-1", []SessionTool{
		{Name: "tool"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instances[0].Definition.Description != "project" {
		t.Errorf("expected project-scoped tool, got %q", instances[0].Definition.Description)
	}
}

func TestResolver_UnknownTool(t *testing.T) {
	reg := newMockRegistry()
	resolver := NewResolver(reg)

	_, err := resolver.Resolve(context.Background(), "", []SessionTool{
		{Name: "nonexistent"},
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error = %q, want to contain 'unknown tool'", err.Error())
	}
}

func TestResolver_RequiredConfigMissing(t *testing.T) {
	reg := newMockRegistry()
	resolver := NewResolver(reg)

	// sentry is a builtin with required auth_token
	_, err := resolver.Resolve(context.Background(), "", []SessionTool{
		{Name: "sentry", Config: map[string]string{}},
	})
	if err == nil {
		t.Fatal("expected error for missing required config")
	}
	if !strings.Contains(err.Error(), "auth_token") {
		t.Errorf("error = %q, want to contain 'auth_token'", err.Error())
	}
}

func TestResolver_EmptyTools(t *testing.T) {
	reg := newMockRegistry()
	resolver := NewResolver(reg)

	instances, err := resolver.Resolve(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instances != nil {
		t.Errorf("expected nil for empty tools, got %v", instances)
	}
}

// operatorKeyRegistry holds an operator GitHub key that auto-fill would use.
type operatorKeyRegistry struct{}

func (operatorKeyRegistry) Create(context.Context, keys.Key) error   { return nil }
func (operatorKeyRegistry) List(context.Context) ([]keys.Key, error) { return nil, nil }
func (operatorKeyRegistry) Delete(context.Context, string) error     { return nil }
func (operatorKeyRegistry) Resolve(context.Context, string, string) (string, error) {
	return "", apperror.NotFound("no key")
}
func (operatorKeyRegistry) Verify(context.Context, string) (*keys.VerifyResult, string, error) {
	return nil, "", nil
}
func (operatorKeyRegistry) ResolveByName(_ context.Context, name string) (string, string, error) {
	if name == "github-env" {
		return "operator-token", "github", nil
	}
	return "", "", apperror.NotFound("no key")
}
func (operatorKeyRegistry) ResolveFullByName(context.Context, string) (string, string, string, error) {
	return "", "", "", apperror.NotFound("no key")
}

func TestResolver_ResolveOwnConfigSkipsAutoFill(t *testing.T) {
	ctx := context.Background()
	resolver := NewResolver(newMockRegistry(), operatorKeyRegistry{})

	filled, err := resolver.Resolve(ctx, "", []SessionTool{{Name: "github"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if filled[0].Config["token"] != "operator-token" {
		t.Fatalf("Resolve did not auto-fill: %v", filled[0].Config)
	}

	if _, err := resolver.ResolveOwnConfig(ctx, "", []SessionTool{{Name: "github"}}); err == nil {
		t.Error("ResolveOwnConfig accepted a tool with no token, so it must have auto-filled one")
	}

	own, err := resolver.ResolveOwnConfig(ctx, "", []SessionTool{{Name: "github", Config: map[string]string{"token": "tenant-token"}}})
	if err != nil {
		t.Fatalf("ResolveOwnConfig: %v", err)
	}
	if own[0].Config["token"] != "tenant-token" {
		t.Errorf("token = %q, want tenant-token", own[0].Config["token"])
	}
}
