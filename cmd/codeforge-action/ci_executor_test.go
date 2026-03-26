package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSystemContext(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .codeforge/ directory with knowledge files
	codeforgeDir := filepath.Join(tmpDir, ".codeforge")
	if err := os.MkdirAll(codeforgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(codeforgeDir, "OVERVIEW.md"), []byte("Project overview content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codeforgeDir, "ARCHITECTURE.md"), []byte("Architecture content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("Project instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	executor := NewCIExecutor(Config{})
	ctx := executor.buildSystemContext(tmpDir)

	if ctx == "" {
		t.Fatal("expected non-empty system context")
	}

	// Check all files were included
	if !containsStr(ctx, "Project overview content") {
		t.Error("missing OVERVIEW.md content")
	}
	if !containsStr(ctx, "Architecture content") {
		t.Error("missing ARCHITECTURE.md content")
	}
	if !containsStr(ctx, "Project instructions") {
		t.Error("missing CLAUDE.md content")
	}

	// CONVENTIONS.md doesn't exist — should be silently skipped (no header written)
}

func TestBuildSystemContext_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewCIExecutor(Config{})
	ctx := executor.buildSystemContext(tmpDir)

	if ctx != "" {
		t.Errorf("expected empty system context, got %q", ctx)
	}
}

func TestBuildPrompt_Custom(t *testing.T) {
	executor := NewCIExecutor(Config{
		SessionType: "custom",
		Prompt:   "fix the bug",
	})

	prompt, err := executor.buildPrompt(&CIContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != "fix the bug" {
		t.Errorf("prompt = %q, want %q", prompt, "fix the bug")
	}
}

func TestBuildPrompt_PRReview(t *testing.T) {
	executor := NewCIExecutor(Config{
		SessionType: "pr_review",
	})

	ciCtx := &CIContext{
		PRNumber:   42,
		PRBranch:   "feature/test",
		BaseBranch: "main",
	}

	prompt, err := executor.buildPrompt(ciCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsStr(prompt, "42") {
		t.Error("prompt should contain PR number")
	}
	if !containsStr(prompt, "main") {
		t.Error("prompt should contain base branch")
	}
	if !containsStr(prompt, "verdict") {
		t.Error("prompt should contain review output format")
	}
}

func TestBuildPrompt_CodeReview(t *testing.T) {
	executor := NewCIExecutor(Config{
		SessionType: "code_review",
	})

	ciCtx := &CIContext{
		BaseBranch: "develop",
	}

	prompt, err := executor.buildPrompt(ciCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
}

func TestBuildPrompt_KnowledgeUpdate(t *testing.T) {
	executor := NewCIExecutor(Config{
		SessionType: "knowledge_update",
	})

	prompt, err := executor.buildPrompt(&CIContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsStr(prompt, ".codeforge") {
		t.Error("prompt should reference .codeforge/ directory")
	}
}

func TestBuildPrompt_InvalidType(t *testing.T) {
	executor := NewCIExecutor(Config{
		SessionType: "invalid",
	})

	_, err := executor.buildPrompt(&CIContext{})
	if err == nil {
		t.Fatal("expected error for invalid session type")
	}
}

func TestCreateRunner(t *testing.T) {
	tests := []struct {
		cli      string
		wantType string
	}{
		{"claude-code", "*runner.ClaudeRunner"},
		{"codex", "*runner.CodexRunner"},
		{"unknown", "*runner.ClaudeRunner"}, // defaults to claude
	}

	for _, tt := range tests {
		t.Run(tt.cli, func(t *testing.T) {
			executor := NewCIExecutor(Config{CLI: tt.cli})
			r := executor.createRunner()
			if r == nil {
				t.Fatal("expected non-nil runner")
			}
		})
	}
}

func TestWriteMCPConfig_Empty(t *testing.T) {
	executor := NewCIExecutor(Config{})
	path, err := executor.writeMCPConfig(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestWriteMCPConfig_JSONString(t *testing.T) {
	tmpDir := t.TempDir()
	mcpJSON := `{"mcpServers":{"test":{"command":"echo"}}}`

	executor := NewCIExecutor(Config{MCPConfig: mcpJSON})
	path, err := executor.writeMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading MCP config: %v", err)
	}
	if string(data) != mcpJSON {
		t.Errorf("MCP config = %q, want %q", string(data), mcpJSON)
	}
}

func TestWriteMCPConfig_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	mcpPath := filepath.Join(tmpDir, "custom.mcp.json")
	if err := os.WriteFile(mcpPath, []byte(`{"test":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	executor := NewCIExecutor(Config{MCPConfig: mcpPath})
	path, err := executor.writeMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != mcpPath {
		t.Errorf("path = %q, want %q", path, mcpPath)
	}
}
