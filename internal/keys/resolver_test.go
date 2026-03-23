package keys

import (
	"context"
	"fmt"
	"testing"
)

// stubRegistry is a minimal Registry implementation for testing.
type stubRegistry struct {
	tokens map[string]string // key: "provider:name" → token
}

func (s *stubRegistry) Create(_ context.Context, _ Key) error                    { return nil }
func (s *stubRegistry) List(_ context.Context) ([]Key, error)                    { return nil, nil }
func (s *stubRegistry) Delete(_ context.Context, _ string) error                 { return nil }
func (s *stubRegistry) Verify(_ context.Context, _ string) (*VerifyResult, string, error) {
	return nil, "", fmt.Errorf("not implemented")
}
func (s *stubRegistry) ResolveByName(_ context.Context, _ string) (string, string, error) {
	return "", "", fmt.Errorf("not found")
}
func (s *stubRegistry) ResolveFullByName(_ context.Context, _ string) (string, string, string, error) {
	return "", "", "", fmt.Errorf("not found")
}

func (s *stubRegistry) Resolve(_ context.Context, provider, name string) (string, error) {
	key := provider + ":" + name
	if tok, ok := s.tokens[key]; ok {
		return tok, nil
	}
	return "", fmt.Errorf("key %q not found", key)
}

func TestResolveToken_InlineToken(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	tok, err := r.ResolveToken(context.Background(), "https://github.com/owner/repo", "my-inline-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "my-inline-token" {
		t.Errorf("got %q, want %q", tok, "my-inline-token")
	}
}

func TestResolveToken_RegistryKey(t *testing.T) {
	reg := &stubRegistry{
		tokens: map[string]string{"github:my-key": "registry-token"},
	}
	r := NewResolver(reg, nil)

	tok, err := r.ResolveToken(context.Background(), "https://github.com/owner/repo", "", "my-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "registry-token" {
		t.Errorf("got %q, want %q", tok, "registry-token")
	}
}

func TestResolveToken_GitHubEnvFallback(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "gh-env-token")

	tok, err := r.ResolveToken(context.Background(), "https://github.com/owner/repo", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gh-env-token" {
		t.Errorf("got %q, want %q", tok, "gh-env-token")
	}
}

func TestResolveToken_GitLabEnvFallback(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "gl-env-token")

	tok, err := r.ResolveToken(context.Background(), "https://gitlab.com/group/project", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gl-env-token" {
		t.Errorf("got %q, want %q", tok, "gl-env-token")
	}
}

func TestResolveToken_UnknownProvider_GitLabEnv(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "gl-self-hosted-token")

	tok, err := r.ResolveToken(context.Background(), "https://code.denik.cz/vlp/paywallbox3.git", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gl-self-hosted-token" {
		t.Errorf("got %q, want %q", tok, "gl-self-hosted-token")
	}
}

func TestResolveToken_UnknownProvider_GitHubEnv(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "") // ensure no GitLab token so GitHub fallback is tested
	t.Setenv("GITHUB_TOKEN", "gh-enterprise-token")

	tok, err := r.ResolveToken(context.Background(), "https://git.company.internal/team/project", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gh-enterprise-token" {
		t.Errorf("got %q, want %q", tok, "gh-enterprise-token")
	}
}

func TestResolveToken_UnknownProvider_GitLabPreferredOverGitHub(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "gl-token")
	t.Setenv("GITHUB_TOKEN", "gh-token")

	tok, err := r.ResolveToken(context.Background(), "https://code.denik.cz/vlp/paywallbox3.git", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gl-token" {
		t.Errorf("got %q, want %q — GITLAB_TOKEN should be preferred for unknown providers", tok, "gl-token")
	}
}

func TestResolveToken_UnknownProvider_NoEnvVars(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := r.ResolveToken(context.Background(), "https://code.denik.cz/vlp/paywallbox3.git", "", "")
	if err == nil {
		t.Fatal("expected error when no token is available")
	}
	if want := "GITLAB_TOKEN or GITHUB_TOKEN"; !contains(err.Error(), want) {
		t.Errorf("error should mention %q, got: %v", want, err)
	}
}

func TestResolveToken_CustomDomain_OverridesUnknown(t *testing.T) {
	domains := map[string]string{"code.denik.cz": "gitlab"}
	r := NewResolver(&stubRegistry{}, domains)

	t.Setenv("GITLAB_TOKEN", "gl-configured-token")

	tok, err := r.ResolveToken(context.Background(), "https://code.denik.cz/vlp/paywallbox3.git", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gl-configured-token" {
		t.Errorf("got %q, want %q", tok, "gl-configured-token")
	}
}

func TestResolveToken_InlineTokenTakesPrecedence(t *testing.T) {
	reg := &stubRegistry{
		tokens: map[string]string{"github:my-key": "registry-token"},
	}
	r := NewResolver(reg, nil)

	t.Setenv("GITHUB_TOKEN", "env-token")

	tok, err := r.ResolveToken(context.Background(), "https://github.com/owner/repo", "inline-token", "my-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "inline-token" {
		t.Errorf("got %q, want %q — inline token should take precedence", tok, "inline-token")
	}
}

func TestResolveToken_RegistryKeyFallsToEnv(t *testing.T) {
	// Registry has no matching key → should fall through to env var
	r := NewResolver(&stubRegistry{}, nil)

	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "env-fallback")

	tok, err := r.ResolveToken(context.Background(), "https://github.com/owner/repo", "", "nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "env-fallback" {
		t.Errorf("got %q, want %q", tok, "env-fallback")
	}
}

func TestResolveToken_InvalidURL(t *testing.T) {
	r := NewResolver(&stubRegistry{}, nil)

	_, err := r.ResolveToken(context.Background(), "://invalid", "", "")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
