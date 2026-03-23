package keys

import (
	"context"
	"os"
	"time"

	"github.com/freema/codeforge/internal/apperror"
)

// envKeyMapping maps an environment variable to a provider key.
type envKeyMapping struct {
	EnvVar      string
	URLEnvVar   string // optional: env var for custom base URL
	Provider    string
	Name        string
}

// knownEnvKeys lists all recognized environment variable → provider mappings.
var knownEnvKeys = []envKeyMapping{
	{"GITHUB_TOKEN", "GITHUB_URL", "github", "github-env"},
	{"GITLAB_TOKEN", "GITLAB_URL", "gitlab", "gitlab-env"},
	{"SENTRY_AUTH_TOKEN", "SENTRY_URL", "sentry", "sentry-env"},
	{"ANTHROPIC_API_KEY", "", "anthropic", "anthropic-env"},
	{"OPENAI_API_KEY", "", "openai", "openai-env"},
}

// EnvAwareRegistry wraps a Registry and surfaces environment-variable-sourced
// keys alongside database keys. Env keys are read-only (cannot be deleted).
type EnvAwareRegistry struct {
	inner Registry
}

// NewEnvAwareRegistry wraps the given registry to also expose env-var keys.
func NewEnvAwareRegistry(inner Registry) *EnvAwareRegistry {
	return &EnvAwareRegistry{inner: inner}
}

func (r *EnvAwareRegistry) Create(ctx context.Context, key Key) error {
	return r.inner.Create(ctx, key)
}

func (r *EnvAwareRegistry) List(ctx context.Context) ([]Key, error) {
	dbKeys, err := r.inner.List(ctx)
	if err != nil {
		return nil, err
	}

	// Mark DB keys
	for i := range dbKeys {
		dbKeys[i].Source = "db"
	}

	// Append env keys
	for _, m := range knownEnvKeys {
		if os.Getenv(m.EnvVar) != "" {
			k := Key{
				Name:      m.Name,
				Provider:  m.Provider,
				Source:    "env",
				Scope:     m.EnvVar,
				CreatedAt: time.Time{},
			}
			if m.URLEnvVar != "" {
				k.BaseURL = os.Getenv(m.URLEnvVar)
			}
			dbKeys = append(dbKeys, k)
		}
	}

	return dbKeys, nil
}

func (r *EnvAwareRegistry) Delete(ctx context.Context, name string) error {
	if r.isEnvKey(name) {
		return apperror.Validation("cannot delete environment-sourced key '%s'", name)
	}
	return r.inner.Delete(ctx, name)
}

func (r *EnvAwareRegistry) Resolve(ctx context.Context, provider, name string) (string, error) {
	// Check env keys first
	for _, m := range knownEnvKeys {
		if m.Name == name && m.Provider == provider {
			if t := os.Getenv(m.EnvVar); t != "" {
				return t, nil
			}
		}
	}
	return r.inner.Resolve(ctx, provider, name)
}

func (r *EnvAwareRegistry) ResolveByName(ctx context.Context, name string) (string, string, error) {
	token, provider, _, err := r.ResolveFullByName(ctx, name)
	return token, provider, err
}

func (r *EnvAwareRegistry) ResolveFullByName(ctx context.Context, name string) (string, string, string, error) {
	for _, m := range knownEnvKeys {
		if m.Name == name {
			if t := os.Getenv(m.EnvVar); t != "" {
				baseURL := ""
				if m.URLEnvVar != "" {
					baseURL = os.Getenv(m.URLEnvVar)
				}
				return t, m.Provider, baseURL, nil
			}
		}
	}
	return r.inner.ResolveFullByName(ctx, name)
}

func (r *EnvAwareRegistry) Verify(ctx context.Context, name string) (*VerifyResult, string, error) {
	// Handle env keys: read token from env and verify
	for _, m := range knownEnvKeys {
		if m.Name == name {
			token := os.Getenv(m.EnvVar)
			if token == "" {
				return nil, "", apperror.NotFound("env key '%s' (%s) is not set", name, m.EnvVar)
			}
			baseURL := ""
			if m.URLEnvVar != "" {
				baseURL = os.Getenv(m.URLEnvVar)
			}
			result := verifyToken(ctx, m.Provider, token, baseURL)
			return result, m.Provider, nil
		}
	}
	return r.inner.Verify(ctx, name)
}

func (r *EnvAwareRegistry) isEnvKey(name string) bool {
	for _, m := range knownEnvKeys {
		if m.Name == name {
			return true
		}
	}
	return false
}
