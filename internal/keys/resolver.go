package keys

import (
	"context"
	"fmt"
	"os"

	gitpkg "github.com/freema/codeforge/internal/tool/git"
)

// Resolver resolves access tokens using a priority chain:
// 1. Inline token on session (access_token field)
// 2. Registered key by provider_key name
// 3. Environment variable fallback (GITHUB_TOKEN / GITLAB_TOKEN)
type Resolver struct {
	registry        Registry
	providerDomains map[string]string
}

// NewResolver creates a new key resolver.
func NewResolver(registry Registry, providerDomains map[string]string) *Resolver {
	return &Resolver{registry: registry, providerDomains: providerDomains}
}

// ResolveToken resolves the access token for a session.
func (r *Resolver) ResolveToken(ctx context.Context, repoURL, accessToken, providerKey string) (string, error) {
	// 1. Inline token
	if accessToken != "" {
		return accessToken, nil
	}

	// Detect provider from repo URL
	repo, err := gitpkg.ParseRepoURL(repoURL, r.providerDomains)
	if err != nil {
		return "", fmt.Errorf("parsing repo URL: %w", err)
	}

	// 2. Registered key by name
	if providerKey != "" {
		token, err := r.registry.Resolve(ctx, string(repo.Provider), providerKey)
		if err == nil {
			return token, nil
		}
		// Fall through to env if key not found
	}

	// 3. Env var fallback
	switch repo.Provider {
	case gitpkg.ProviderGitHub:
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t, nil
		}
	case gitpkg.ProviderGitLab:
		if t := os.Getenv("GITLAB_TOKEN"); t != "" {
			return t, nil
		}
	case gitpkg.ProviderUnknown:
		// Self-hosted instances with unrecognized domains: try both env vars.
		// GITLAB_TOKEN first — self-hosted GitLab is far more common than GitHub Enterprise.
		if t := os.Getenv("GITLAB_TOKEN"); t != "" {
			return t, nil
		}
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t, nil
		}
	}

	return "", fmt.Errorf("no access token available for %s (provide access_token, provider_key, or set %s env var)",
		repoURL, envHint(repo.Provider))
}

// ResolveAIKey tries to resolve an AI provider API key from the registry.
// It looks up keys by the well-known env-sourced name ("<provider>-env") first,
// then falls back to any key matching the given provider.
func (r *Resolver) ResolveAIKey(ctx context.Context, provider string) (string, error) {
	// Try the well-known env-sourced key name first (e.g. "anthropic-env").
	token, _, err := r.registry.ResolveByName(ctx, provider+"-env")
	if err == nil {
		return token, nil
	}

	// Try resolving by provider with a conventional default name.
	token, err = r.registry.Resolve(ctx, provider, "default")
	if err == nil {
		return token, nil
	}

	return "", fmt.Errorf("no AI key found for provider %q", provider)
}

// envHint returns a human-readable hint for the error message about which env var to set.
func envHint(p gitpkg.Provider) string {
	switch p {
	case gitpkg.ProviderGitHub:
		return "GITHUB_TOKEN"
	case gitpkg.ProviderGitLab:
		return "GITLAB_TOKEN"
	default:
		return "GITLAB_TOKEN or GITHUB_TOKEN"
	}
}
