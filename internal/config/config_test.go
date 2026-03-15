package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"server.port", cfg.Server.Port, 8080},
		{"redis.prefix", cfg.Redis.Prefix, "codeforge:"},
		{"sqlite.path", cfg.SQLite.Path, "/data/codeforge.db"},
		{"workers.concurrency", cfg.Workers.Concurrency, 3},
		{"workers.queue_name", cfg.Workers.QueueName, "queue:tasks"},
		{"tasks.default_timeout", cfg.Tasks.DefaultTimeout, 300},
		{"tasks.max_timeout", cfg.Tasks.MaxTimeout, 1800},
		{"cli.default", cfg.CLI.Default, "claude-code"},
		{"cli.claude_code.path", cfg.CLI.ClaudeCode.Path, "claude"},
		{"cli.codex.path", cfg.CLI.Codex.Path, "codex"},
		{"git.branch_prefix", cfg.Git.BranchPrefix, "codeforge/"},
		{"rate_limit.enabled", cfg.RateLimit.Enabled, true},
		{"rate_limit.tasks_per_minute", cfg.RateLimit.TasksPerMinute, 10},
		{"logging.level", cfg.Logging.Level, "info"},
		{"logging.format", cfg.Logging.Format, "json"},
		{"code_review.webhook_dedup_ttl", cfg.CodeReview.WebhookDedupTTL, 3600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
  auth_token: "test-token"
redis:
  url: "redis://localhost:6379"
  prefix: "test:"
encryption:
  key: "0123456789abcdef0123456789abcdef"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("server.port: got %d, want 9090", cfg.Server.Port)
	}
	if cfg.Redis.Prefix != "test:" {
		t.Errorf("redis.prefix: got %s, want test:", cfg.Redis.Prefix)
	}
	// Defaults preserved for unset fields
	if cfg.Workers.Concurrency != 3 {
		t.Errorf("workers.concurrency: got %d, want 3 (default)", cfg.Workers.Concurrency)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
  auth_token: "test-token"
redis:
  url: "redis://localhost:6379"
encryption:
  key: "0123456789abcdef0123456789abcdef"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Env var overrides YAML
	t.Setenv("CODEFORGE_SERVER__PORT", "7070")
	t.Setenv("CODEFORGE_WORKERS__CONCURRENCY", "5")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 7070 {
		t.Errorf("server.port: got %d, want 7070 (env override)", cfg.Server.Port)
	}
	if cfg.Workers.Concurrency != 5 {
		t.Errorf("workers.concurrency: got %d, want 5 (env override)", cfg.Workers.Concurrency)
	}
}

func TestLoad_Validation_MissingRedis(t *testing.T) {
	// Clear any env vars that could provide redis.url
	t.Setenv("CODEFORGE_REDIS__URL", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  auth_token: "test-token"
encryption:
  key: "somekey"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing redis.url")
	}
}

func TestLoad_Validation_MissingAuthToken(t *testing.T) {
	t.Setenv("CODEFORGE_SERVER__AUTH_TOKEN", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
redis:
  url: "redis://localhost:6379"
encryption:
  key: "somekey"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing auth_token")
	}
}

func TestLoad_Validation_MissingEncryptionKey(t *testing.T) {
	t.Setenv("CODEFORGE_ENCRYPTION__KEY", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  auth_token: "test-token"
redis:
  url: "redis://localhost:6379"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing encryption.key")
	}
}

func TestLoad_InvalidConfigPath(t *testing.T) {
	_, err := Load("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
}

func TestLoad_NoConfigPath_DefaultFallback(t *testing.T) {
	// Set required env vars so validation passes
	t.Setenv("CODEFORGE_REDIS__URL", "redis://localhost:6379")
	t.Setenv("CODEFORGE_SERVER__AUTH_TOKEN", "test-token")
	t.Setenv("CODEFORGE_ENCRYPTION__KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error with env vars, got: %v", err)
	}

	if cfg.Redis.URL != "redis://localhost:6379" {
		t.Errorf("redis.url: got %s, want redis://localhost:6379", cfg.Redis.URL)
	}
}
