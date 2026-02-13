package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Redis      RedisConfig      `koanf:"redis"`
	Workers    WorkersConfig    `koanf:"workers"`
	Tasks      TasksConfig      `koanf:"tasks"`
	CLI        CLIConfig        `koanf:"cli"`
	Git        GitConfig        `koanf:"git"`
	Encryption EncryptionConfig `koanf:"encryption"`
	MCP        MCPConfig        `koanf:"mcp"`
	Webhooks   WebhookConfig    `koanf:"webhooks"`
	RateLimit  RateLimitConfig  `koanf:"rate_limit"`
	Tracing    TracingConfig    `koanf:"tracing"`
	Logging    LoggingConfig    `koanf:"logging"`
}

type ServerConfig struct {
	Port      int    `koanf:"port"`
	AuthToken string `koanf:"auth_token"`
}

type RedisConfig struct {
	URL      string `koanf:"url"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
	Prefix   string `koanf:"prefix"`
}

type WorkersConfig struct {
	Concurrency int    `koanf:"concurrency"`
	QueueName   string `koanf:"queue_name"`
}

type TasksConfig struct {
	DefaultTimeout         int    `koanf:"default_timeout"`
	MaxTimeout             int    `koanf:"max_timeout"`
	WorkspaceTTL           int    `koanf:"workspace_ttl"`
	WorkspaceBase          string `koanf:"workspace_base"`
	StateTTL               int    `koanf:"state_ttl"`
	ResultTTL              int    `koanf:"result_ttl"`
	DiskWarningThresholdGB int    `koanf:"disk_warning_threshold_gb"`
	DiskCriticalThresholdGB int   `koanf:"disk_critical_threshold_gb"`
}

type CLIConfig struct {
	Default    string          `koanf:"default"`
	ClaudeCode ClaudeCodeConfig `koanf:"claude_code"`
}

type ClaudeCodeConfig struct {
	Path         string `koanf:"path"`
	Version      string `koanf:"version"`
	DefaultModel string `koanf:"default_model"`
}

type GitConfig struct {
	BranchPrefix    string            `koanf:"branch_prefix"`
	CommitAuthor    string            `koanf:"commit_author"`
	CommitEmail     string            `koanf:"commit_email"`
	ProviderDomains map[string]string `koanf:"provider_domains"`
}

type EncryptionConfig struct {
	Key string `koanf:"key"`
}

type MCPConfig struct {
	GlobalServers []interface{} `koanf:"global_servers"`
}

type WebhookConfig struct {
	HMACSecret string        `koanf:"hmac_secret"`
	RetryCount int           `koanf:"retry_count"`
	RetryDelay time.Duration `koanf:"retry_delay"`
}

type RateLimitConfig struct {
	Enabled        bool `koanf:"enabled"`
	TasksPerMinute int  `koanf:"tasks_per_minute"`
}

type TracingConfig struct {
	Enabled      bool    `koanf:"enabled"`
	Exporter     string  `koanf:"exporter"`
	Endpoint     string  `koanf:"endpoint"`
	SamplingRate float64 `koanf:"sampling_rate"`
}

type LoggingConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Redis: RedisConfig{
			DB:     0,
			Prefix: "codeforge:",
		},
		Workers: WorkersConfig{
			Concurrency: 3,
			QueueName:   "queue:tasks",
		},
		Tasks: TasksConfig{
			DefaultTimeout:          300,
			MaxTimeout:              1800,
			WorkspaceTTL:            86400,
			WorkspaceBase:           "/data/workspaces",
			StateTTL:                604800,
			ResultTTL:               604800,
			DiskWarningThresholdGB:  10,
			DiskCriticalThresholdGB: 20,
		},
		CLI: CLIConfig{
			Default: "claude-code",
			ClaudeCode: ClaudeCodeConfig{
				Path:         "claude",
				DefaultModel: "claude-sonnet-4-20250514",
			},
		},
		Git: GitConfig{
			BranchPrefix:    "codeforge/",
			CommitAuthor:    "CodeForge Bot",
			CommitEmail:     "codeforge@noreply",
			ProviderDomains: map[string]string{},
		},
		Webhooks: WebhookConfig{
			RetryCount: 3,
			RetryDelay: 5 * time.Second,
		},
		RateLimit: RateLimitConfig{
			Enabled:        true,
			TasksPerMinute: 10,
		},
		Tracing: TracingConfig{
			Exporter:     "otlp",
			SamplingRate: 0.1,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads configuration from YAML file + environment variables.
// Loading order: defaults → YAML file → env vars (later overrides earlier).
func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	cfg := Defaults()

	// Load YAML file (optional — may not exist)
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
			// Only fail if the file was explicitly specified and can't be read
			return nil, fmt.Errorf("loading config file %s: %w", configPath, err)
		}
	} else {
		// Try default path, ignore if not found
		_ = k.Load(file.Provider("codeforge.yaml"), yaml.Parser())
	}

	// Load environment variables.
	// CODEFORGE_SERVER__AUTH_TOKEN → server.auth_token
	// Double underscore (__) separates nesting levels.
	// Single underscore within a level is preserved (e.g., auth_token).
	err := k.Load(env.Provider("CODEFORGE_", ".", func(s string) string {
		s = strings.TrimPrefix(s, "CODEFORGE_")
		s = strings.ToLower(s)
		// Replace __ with a placeholder, then _ within words stays,
		// then restore placeholder as "." for nesting.
		s = strings.ReplaceAll(s, "__", ".")
		return s
	}), nil)
	if err != nil {
		return nil, fmt.Errorf("loading env vars: %w", err)
	}

	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Redis.URL == "" {
		return fmt.Errorf("config: redis.url is required (set CODEFORGE_REDIS__URL)")
	}
	if cfg.Server.AuthToken == "" {
		return fmt.Errorf("config: server.auth_token is required (set CODEFORGE_SERVER__AUTH_TOKEN)")
	}
	if cfg.Encryption.Key == "" {
		return fmt.Errorf("config: encryption.key is required (set CODEFORGE_ENCRYPTION__KEY)")
	}
	return nil
}
