package worker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tool/mcp"
	"github.com/freema/codeforge/internal/tools"
)

// operatorKeys is a key registry holding the operator's credentials.
type operatorKeys struct{}

func (operatorKeys) Create(context.Context, keys.Key) error { return nil }
func (operatorKeys) List(context.Context) ([]keys.Key, error) {
	return []keys.Key{{Name: "operator-github", Provider: "github"}}, nil
}
func (operatorKeys) Delete(context.Context, string) error { return nil }
func (operatorKeys) Resolve(_ context.Context, _, name string) (string, error) {
	if name == "operator-github" {
		return "operator-registry-token", nil
	}
	return "", errors.New("not found")
}
func (operatorKeys) Verify(context.Context, string) (*keys.VerifyResult, string, error) {
	return nil, "", nil
}
func (operatorKeys) ResolveByName(_ context.Context, name string) (string, string, error) {
	if name == "operator-github" || name == "github-env" {
		return "operator-registry-token", "github", nil
	}
	return "", "", errors.New("not found")
}
func (operatorKeys) ResolveFullByName(ctx context.Context, name string) (string, string, string, error) {
	token, provider, err := operatorKeys{}.ResolveByName(ctx, name)
	return token, provider, "", err
}

// noTools is an empty tool registry; sessions fall back to the built-in catalog.
type noTools struct{}

func (noTools) Create(context.Context, string, tools.ToolDefinition) error { return nil }
func (noTools) Get(_ context.Context, _, name string) (*tools.ToolDefinition, error) {
	return nil, apperror.NotFound("tool %s not found", name)
}
func (noTools) List(context.Context, string) ([]tools.ToolDefinition, error) { return nil, nil }
func (noTools) Delete(context.Context, string, string) error                 { return nil }

// operatorMCP holds one operator-registered MCP server carrying a secret.
type operatorMCP struct{}

var operatorServer = mcp.Server{Name: "operator-sentry", Command: "npx", Package: "sentry-mcp", Env: map[string]string{"SENTRY_TOKEN": "operator-mcp-secret"}}

func (operatorMCP) CreateGlobal(context.Context, mcp.Server) error { return nil }
func (operatorMCP) ListGlobal(context.Context) ([]mcp.Server, error) {
	return []mcp.Server{operatorServer}, nil
}
func (operatorMCP) DeleteGlobal(context.Context, string) error { return nil }
func (operatorMCP) ResolveGlobal(context.Context, string) (*mcp.Server, error) {
	return &operatorServer, nil
}
func (operatorMCP) CreateProject(context.Context, string, mcp.Server) error   { return nil }
func (operatorMCP) ListProject(context.Context, string) ([]mcp.Server, error) { return nil, nil }
func (operatorMCP) DeleteProject(context.Context, string, string) error       { return nil }
func (operatorMCP) ResolveMCPServers(_ context.Context, _ string, task []mcp.Server) ([]mcp.Server, error) {
	return append([]mcp.Server{operatorServer}, task...), nil
}

const tenantTestRepo = "https://github.com/acme/repo.git"

func TestResolveToken_TenantSessionGetsNoOperatorToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "operator-env-token")
	e := &Executor{keyResolver: keys.NewResolver(operatorKeys{}, nil)}
	log := slog.Default()

	tenantSession := &session.Session{RepoURL: tenantTestRepo, TenantID: "acme", ProviderKey: "operator-github"}
	e.resolveToken(context.Background(), tenantSession, log)
	if tenantSession.AccessToken != "" {
		t.Errorf("tenant session got token %q", tenantSession.AccessToken)
	}

	tenantNoKey := &session.Session{RepoURL: tenantTestRepo, TenantID: "acme"}
	e.resolveToken(context.Background(), tenantNoKey, log)
	if tenantNoKey.AccessToken != "" {
		t.Errorf("tenant session got env token %q", tenantNoKey.AccessToken)
	}

	own := &session.Session{RepoURL: tenantTestRepo, TenantID: "acme", AccessToken: "tenant-token"}
	e.resolveToken(context.Background(), own, log)
	if own.AccessToken != "tenant-token" {
		t.Errorf("tenant's own token replaced with %q", own.AccessToken)
	}

	operator := &session.Session{RepoURL: tenantTestRepo, ProviderKey: "operator-github"}
	e.resolveToken(context.Background(), operator, log)
	if operator.AccessToken != "operator-registry-token" {
		t.Errorf("operator session token = %q, want the registry token", operator.AccessToken)
	}
}

func TestSetupMCP_TenantSessionGetsNoOperatorCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "operator-env-token")
	e := &Executor{
		mcpInstaller: mcp.NewInstaller(operatorMCP{}),
		toolResolver: tools.NewResolver(noTools{}, operatorKeys{}),
	}
	log := slog.Default()

	readConfig := func(t *testing.T, dir string) string {
		t.Helper()
		out, err := os.ReadFile(mcp.ConfigPath(dir, ""))
		if err != nil {
			t.Fatalf("reading MCP config: %v", err)
		}
		return string(out)
	}

	t.Run("operator registry servers and tool auto-fill are not used", func(t *testing.T) {
		dir := t.TempDir()
		tenantSession := &session.Session{RepoURL: tenantTestRepo, TenantID: "acme", Config: &session.Config{
			MCPServers: []session.MCPServer{{Name: "own", Command: "npx", Package: "own-mcp"}},
		}}
		if _, err := e.setupMCP(context.Background(), tenantSession, dir, log); err != nil {
			t.Fatalf("setupMCP: %v", err)
		}
		cfg := readConfig(t, dir)
		if !strings.Contains(cfg, "own-mcp") {
			t.Errorf("tenant's own server missing from %s", cfg)
		}
		if strings.Contains(cfg, "operator-sentry") || strings.Contains(cfg, "operator-mcp-secret") {
			t.Errorf("operator server written into tenant workspace: %s", cfg)
		}
	})

	t.Run("tool config is not auto-filled from operator keys", func(t *testing.T) {
		dir := t.TempDir()
		tenantSession := &session.Session{RepoURL: tenantTestRepo, TenantID: "acme", Config: &session.Config{
			Tools: []tools.SessionTool{{Name: "github"}},
		}}
		_, err := e.setupMCP(context.Background(), tenantSession, dir, log)
		if err == nil || !strings.Contains(err.Error(), "token") {
			t.Fatalf("err = %v, want tool resolution to fail on the missing token", err)
		}
		if out, readErr := os.ReadFile(mcp.ConfigPath(dir, "")); readErr == nil && strings.Contains(string(out), "operator-registry-token") {
			t.Errorf("operator key written into tenant workspace: %s", out)
		}
	})

	t.Run("operator sessions keep registry servers and auto-fill", func(t *testing.T) {
		dir := t.TempDir()
		operator := &session.Session{RepoURL: tenantTestRepo, Config: &session.Config{
			Tools: []tools.SessionTool{{Name: "github"}},
		}}
		if _, err := e.setupMCP(context.Background(), operator, dir, log); err != nil {
			t.Fatalf("setupMCP: %v", err)
		}
		cfg := readConfig(t, dir)
		if !strings.Contains(cfg, "operator-sentry") || !strings.Contains(cfg, "operator-registry-token") {
			t.Errorf("operator session lost registry servers or auto-fill: %s", cfg)
		}
	})
}
