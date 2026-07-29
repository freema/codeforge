package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

const (
	cliClaudeCode = "claude-code"
	cliCodex      = "codex"
)

// Provider token sources — tracked so platform-specific limitations
// (e.g. GitLab CI job tokens cannot post MR discussions) surface as clear
// errors instead of silent 401s.
const (
	tokenSourceInput      = "input"        // INPUT_PROVIDER_TOKEN
	tokenSourceGitHubEnv  = "github_token" // GITHUB_TOKEN
	tokenSourceGitLabEnv  = "gitlab_token" // GITLAB_TOKEN
	tokenSourceCIJobToken = "ci_job_token" // CI_JOB_TOKEN (clone-only on GitLab)
)

// Config holds all configuration for a CI Action run.
type Config struct {
	SessionType          string // pr_review, code_review, knowledge_update, custom
	Prompt               string // user prompt (or auto-detect from PR)
	CLI                  string // claude-code, codex
	Model                string // AI model override
	APIKey               string // AI provider API key
	ProviderToken        string // GitHub/GitLab token for PR operations
	ProviderTokenSource  string // where ProviderToken came from (tokenSource* consts)
	MCPConfig            string // JSON string or path to .mcp.json
	PostComments         bool   // post review as PR comments
	OutputFormat         string // json, markdown, text
	MaxTurns             int    // max conversation turns
	AllowedTools         string // comma-separated tool allowlist
	FailOnRequestChanges bool   // exit 1 when verdict is request_changes
}

func main() {
	// Structured logging to stderr (stdout reserved for result output)
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))

	os.Exit(run())
}

func run() int {
	cfg := parseConfig()

	if err := validateConfig(cfg); err != nil {
		slog.Error("invalid configuration", "error", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	executor := NewCIExecutor(cfg)
	return executor.Execute(ctx)
}

// parseConfig reads configuration from environment variables.
// In GitHub Actions, inputs are exposed as INPUT_<NAME> env vars.
// In GitLab CI, they come from CI/CD variables.
func parseConfig() Config {
	cfg := Config{
		SessionType:          envDefault("INPUT_SESSION_TYPE", envDefault("INPUT_TASK_TYPE", "pr_review")),
		Prompt:               envDefault("INPUT_PROMPT", ""),
		CLI:                  envDefault("INPUT_CLI", envDefault("CODEFORGE_CLI", cliClaudeCode)),
		Model:                envDefault("INPUT_MODEL", ""),
		ProviderToken:        envDefault("INPUT_PROVIDER_TOKEN", ""),
		MCPConfig:            envDefault("INPUT_MCP_CONFIG", ""),
		PostComments:         envBool("INPUT_POST_COMMENTS", true),
		OutputFormat:         envDefault("INPUT_OUTPUT_FORMAT", "json"),
		MaxTurns:             envInt("INPUT_MAX_TURNS", 0),
		AllowedTools:         envDefault("INPUT_ALLOWED_TOOLS", ""),
		FailOnRequestChanges: envBool("INPUT_FAIL_ON_REQUEST_CHANGES", false),
	}

	// Resolve API key based on CLI
	switch cfg.CLI {
	case cliClaudeCode:
		cfg.APIKey = envDefault("INPUT_API_KEY", os.Getenv("ANTHROPIC_API_KEY"))
	case cliCodex:
		cfg.APIKey = envDefault("INPUT_API_KEY", os.Getenv("OPENAI_API_KEY"))
	}

	// Resolve provider token from platform-specific env vars, tracking the
	// source so token-capability checks can produce actionable errors.
	if cfg.ProviderToken != "" {
		cfg.ProviderTokenSource = tokenSourceInput
	}
	if cfg.ProviderToken == "" {
		if v := os.Getenv("GITHUB_TOKEN"); v != "" {
			cfg.ProviderToken = v
			cfg.ProviderTokenSource = tokenSourceGitHubEnv
		}
	}
	if cfg.ProviderToken == "" {
		if v := os.Getenv("GITLAB_TOKEN"); v != "" {
			cfg.ProviderToken = v
			cfg.ProviderTokenSource = tokenSourceGitLabEnv
		} else if v := os.Getenv("CI_JOB_TOKEN"); v != "" {
			cfg.ProviderToken = v
			cfg.ProviderTokenSource = tokenSourceCIJobToken
		}
	}

	return cfg
}

func validateConfig(cfg Config) error {
	validTypes := map[string]bool{
		"pr_review":        true,
		"code_review":      true,
		"knowledge_update": true,
		"custom":           true,
	}
	if !validTypes[cfg.SessionType] {
		return fmt.Errorf("invalid session_type: %q (valid: pr_review, code_review, knowledge_update, custom)", cfg.SessionType)
	}

	if cfg.CLI != cliClaudeCode && cfg.CLI != cliCodex {
		return fmt.Errorf("invalid cli: %q (valid: claude-code, codex)", cfg.CLI)
	}

	if cfg.APIKey == "" {
		switch cfg.CLI {
		case cliClaudeCode:
			return fmt.Errorf("ANTHROPIC_API_KEY is required for claude-code CLI")
		case cliCodex:
			return fmt.Errorf("OPENAI_API_KEY is required for codex CLI")
		}
	}

	if cfg.SessionType == "custom" && cfg.Prompt == "" {
		return fmt.Errorf("prompt is required for session_type=custom")
	}

	return nil
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return strings.EqualFold(v, "true") || v == "1"
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
