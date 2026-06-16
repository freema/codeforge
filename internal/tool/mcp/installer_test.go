package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sentryServer() Server {
	return Server{
		Name:    "sentry",
		Command: "npx",
		Package: "@sentry/mcp-server",
		Env:     map[string]string{"SENTRY_ACCESS_TOKEN": "tok"},
	}
}

func TestWriteMCPConfigForCLI_Cursor(t *testing.T) {
	dir := t.TempDir()

	if err := WriteMCPConfigForCLI(dir, "cursor", []Server{sentryServer()}); err != nil {
		t.Fatalf("WriteMCPConfigForCLI: %v", err)
	}

	cursorPath := filepath.Join(dir, ".cursor", "cli.json")
	if _, err := os.Stat(cursorPath); err != nil {
		t.Fatalf("expected .cursor/cli.json to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("did not expect .mcp.json for cursor CLI")
	}

	data, _ := os.ReadFile(cursorPath)
	if !strings.Contains(string(data), "mcpServers") || !strings.Contains(string(data), "@sentry/mcp-server") {
		t.Errorf("unexpected cursor config contents: %s", data)
	}

	if got := ConfigPath(dir, "cursor"); got != cursorPath {
		t.Errorf("ConfigPath = %q, want %q", got, cursorPath)
	}

	assertGitignored(t, dir, ".cursor/cli.json")
}

func TestWriteMCPConfigForCLI_Cursor_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A repo's own committed config with non-MCP settings.
	if err := os.WriteFile(filepath.Join(cursorDir, "cli.json"),
		[]byte(`{"permissions":{"deny":["Write"]},"mcpServers":{"old":{"command":"x"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteMCPConfigForCLI(dir, "cursor", []Server{sentryServer()}); err != nil {
		t.Fatalf("WriteMCPConfigForCLI: %v", err)
	}

	var got map[string]interface{}
	data, _ := os.ReadFile(filepath.Join(cursorDir, "cli.json"))
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := got["permissions"]; !ok {
		t.Error("existing 'permissions' key was destroyed by the MCP write (must merge, not clobber)")
	}
	servers, _ := got["mcpServers"].(map[string]interface{})
	if _, ok := servers["sentry"]; !ok {
		t.Error("mcpServers.sentry was not written")
	}
	if _, ok := servers["old"]; ok {
		t.Error("mcpServers should be set wholesale (stale 'old' server should be gone)")
	}
}

func TestWriteMCPConfigForCLI_DefaultCLI(t *testing.T) {
	dir := t.TempDir()

	if err := WriteMCPConfigForCLI(dir, "claude-code", []Server{sentryServer()}); err != nil {
		t.Fatalf("WriteMCPConfigForCLI: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err != nil {
		t.Fatalf("expected .mcp.json to exist: %v", err)
	}
	assertGitignored(t, dir, ".mcp.json")
}

func TestEnsureGitignore_Idempotent(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		if err := ensureGitignore(dir, ".mcp.json"); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if got := strings.Count(string(data), ".mcp.json"); got != 1 {
		t.Errorf("expected .mcp.json once in .gitignore, got %d", got)
	}
}

func assertGitignored(t *testing.T, dir, entry string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return
		}
	}
	t.Errorf(".gitignore missing %q; contents:\n%s", entry, data)
}
