package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/freema/codeforge/internal/tool/runner"
)

func TestCLIHandler_List(t *testing.T) {
	registry := runner.NewRegistry("claude-code")
	registry.Register("claude-code", runner.NewClaudeRunner("claude"))
	registry.Register("codex", runner.NewCodexRunner("codex"))

	configs := map[string]CLIInfo{
		"claude-code": {Name: "claude-code", BinaryPath: "claude", DefaultModel: "claude-sonnet-4-20250514"},
		"codex":       {Name: "codex", BinaryPath: "codex"},
	}

	h := NewCLIHandler(registry, configs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/cli", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		CLI []struct {
			Name         string `json:"name"`
			BinaryPath   string `json:"binary_path"`
			DefaultModel string `json:"default_model"`
			Available    bool   `json:"available"`
			IsDefault    bool   `json:"is_default"`
		} `json:"cli"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.CLI) != 2 {
		t.Fatalf("expected 2 CLIs, got %d", len(resp.CLI))
	}

	// Sorted alphabetically: claude-code, codex
	if resp.CLI[0].Name != "claude-code" {
		t.Errorf("expected first CLI to be claude-code, got %s", resp.CLI[0].Name)
	}
	if !resp.CLI[0].IsDefault {
		t.Error("expected claude-code to be default")
	}
	if resp.CLI[1].Name != "codex" {
		t.Errorf("expected second CLI to be codex, got %s", resp.CLI[1].Name)
	}
	if resp.CLI[1].IsDefault {
		t.Error("expected codex to not be default")
	}
}

func TestCLIHandler_Health_Available(t *testing.T) {
	// Use "go" as binary — it's always available in test environments
	registry := runner.NewRegistry("test-cli")
	registry.Register("test-cli", runner.NewClaudeRunner("go"))

	configs := map[string]CLIInfo{
		"test-cli": {Name: "test-cli", BinaryPath: "go"},
	}

	h := NewCLIHandler(registry, configs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/cli/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestCLIHandler_Health_Unavailable(t *testing.T) {
	registry := runner.NewRegistry("missing-cli")
	registry.Register("missing-cli", runner.NewClaudeRunner("nonexistent-binary-abc123"))

	configs := map[string]CLIInfo{
		"missing-cli": {Name: "missing-cli", BinaryPath: "nonexistent-binary-abc123"},
	}

	h := NewCLIHandler(registry, configs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/cli/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "unavailable" {
		t.Errorf("expected status unavailable, got %v", resp["status"])
	}
}

func TestCLIHandler_Health_NotConfigured(t *testing.T) {
	registry := runner.NewRegistry("unknown")

	configs := map[string]CLIInfo{} // empty — default not in configs

	h := NewCLIHandler(registry, configs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/cli/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "error" {
		t.Errorf("expected status error, got %v", resp["status"])
	}
}
