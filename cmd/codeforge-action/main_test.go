package main

import (
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	// Clear env to get defaults
	clearCIEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := parseConfig()

	if cfg.SessionType != "pr_review" {
		t.Errorf("SessionType = %q, want %q", cfg.SessionType, "pr_review")
	}
	if cfg.CLI != "claude-code" {
		t.Errorf("CLI = %q, want %q", cfg.CLI, "claude-code")
	}
	if !cfg.PostComments {
		t.Error("PostComments should default to true")
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", cfg.OutputFormat, "json")
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
}

func TestParseConfig_InputOverrides(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("INPUT_TASK_TYPE", "custom")
	t.Setenv("INPUT_CLI", "codex")
	t.Setenv("INPUT_MODEL", "gpt-4o")
	t.Setenv("INPUT_PROMPT", "fix the bug")
	t.Setenv("INPUT_POST_COMMENTS", "false")
	t.Setenv("INPUT_MAX_TURNS", "5")
	t.Setenv("INPUT_ALLOWED_TOOLS", "bash,read")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	cfg := parseConfig()

	if cfg.SessionType != "custom" {
		t.Errorf("SessionType = %q, want %q", cfg.SessionType, "custom")
	}
	if cfg.CLI != "codex" {
		t.Errorf("CLI = %q, want %q", cfg.CLI, "codex")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o")
	}
	if cfg.Prompt != "fix the bug" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "fix the bug")
	}
	if cfg.PostComments {
		t.Error("PostComments should be false")
	}
	if cfg.MaxTurns != 5 {
		t.Errorf("MaxTurns = %d, want %d", cfg.MaxTurns, 5)
	}
	if cfg.AllowedTools != "bash,read" {
		t.Errorf("AllowedTools = %q, want %q", cfg.AllowedTools, "bash,read")
	}
	if cfg.APIKey != "openai-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "openai-key")
	}
}

func TestParseConfig_ProviderTokenFallback(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	// GitHub token fallback
	t.Setenv("GITHUB_TOKEN", "gh-token")
	cfg := parseConfig()
	if cfg.ProviderToken != "gh-token" {
		t.Errorf("ProviderToken = %q, want %q", cfg.ProviderToken, "gh-token")
	}
}

func TestParseConfig_CLIEnvFallback(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CODEFORGE_CLI", "codex")
	t.Setenv("OPENAI_API_KEY", "key")

	cfg := parseConfig()
	if cfg.CLI != "codex" {
		t.Errorf("CLI = %q, want %q", cfg.CLI, "codex")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid pr_review",
			cfg: Config{
				SessionType: "pr_review",
				CLI:      "claude-code",
				APIKey:   "key",
			},
			wantErr: false,
		},
		{
			name: "valid custom with prompt",
			cfg: Config{
				SessionType: "custom",
				CLI:      "claude-code",
				APIKey:   "key",
				Prompt:   "do something",
			},
			wantErr: false,
		},
		{
			name: "invalid session type",
			cfg: Config{
				SessionType: "invalid",
				CLI:      "claude-code",
				APIKey:   "key",
			},
			wantErr: true,
			errMsg:  "invalid session_type",
		},
		{
			name: "invalid CLI",
			cfg: Config{
				SessionType: "pr_review",
				CLI:      "unknown",
				APIKey:   "key",
			},
			wantErr: true,
			errMsg:  "invalid cli",
		},
		{
			name: "missing API key claude",
			cfg: Config{
				SessionType: "pr_review",
				CLI:      "claude-code",
			},
			wantErr: true,
			errMsg:  "ANTHROPIC_API_KEY",
		},
		{
			name: "missing API key codex",
			cfg: Config{
				SessionType: "pr_review",
				CLI:      "codex",
			},
			wantErr: true,
			errMsg:  "OPENAI_API_KEY",
		},
		{
			name: "custom without prompt",
			cfg: Config{
				SessionType: "custom",
				CLI:      "claude-code",
				APIKey:   "key",
			},
			wantErr: true,
			errMsg:  "prompt is required",
		},
		{
			name: "valid knowledge_update",
			cfg: Config{
				SessionType: "knowledge_update",
				CLI:      "claude-code",
				APIKey:   "key",
			},
			wantErr: false,
		},
		{
			name: "valid code_review",
			cfg: Config{
				SessionType: "code_review",
				CLI:      "codex",
				APIKey:   "key",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEnvDefault(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")
	if got := envDefault("TEST_VAR", "world"); got != "hello" {
		t.Errorf("envDefault = %q, want %q", got, "hello")
	}
	if got := envDefault("NONEXISTENT_VAR_12345", "world"); got != "world" {
		t.Errorf("envDefault = %q, want %q", got, "world")
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		val      string
		fallback bool
		want     bool
	}{
		{"true", false, true},
		{"TRUE", false, true},
		{"1", false, true},
		{"false", true, false},
		{"0", true, false},
		{"", true, true},
		{"", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			if tt.val != "" {
				t.Setenv("TEST_BOOL", tt.val)
			}
			key := "TEST_BOOL"
			if tt.val == "" {
				key = "NONEXISTENT_BOOL_12345"
			}
			if got := envBool(key, tt.fallback); got != tt.want {
				t.Errorf("envBool(%q, %v) = %v, want %v", tt.val, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := envInt("TEST_INT", 0); got != 42 {
		t.Errorf("envInt = %d, want %d", got, 42)
	}

	t.Setenv("TEST_INT_BAD", "abc")
	if got := envInt("TEST_INT_BAD", 99); got != 99 {
		t.Errorf("envInt (bad) = %d, want %d", got, 99)
	}

	if got := envInt("NONEXISTENT_INT_12345", 7); got != 7 {
		t.Errorf("envInt (missing) = %d, want %d", got, 7)
	}
}

func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"INPUT_TASK_TYPE", "INPUT_PROMPT", "INPUT_CLI", "INPUT_MODEL",
		"INPUT_API_KEY", "INPUT_PROVIDER_TOKEN", "INPUT_MCP_CONFIG",
		"INPUT_POST_COMMENTS", "INPUT_OUTPUT_FORMAT", "INPUT_MAX_TURNS",
		"INPUT_ALLOWED_TOOLS", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
		"GITHUB_TOKEN", "GITLAB_TOKEN", "CI_JOB_TOKEN", "CODEFORGE_CLI",
	} {
		t.Setenv(key, "")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
