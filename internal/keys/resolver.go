package keys

import (
	"context"
	"fmt"
	"os"

	gitpkg "github.com/freema/codeforge/internal/git"
)

// Resolver resolves access tokens using a priority chain:
// 1. Inline token on task (access_token field)
// 2. Registered key by provider_key name
// 3. Environment variable fallback (GITHUB_TOKEN / GITLAB_TOKEN)
type Resolver struct {
	registry        *Registry
	providerDomains map[string]string
}

// NewResolver creates a new key resolver.
func NewResolver(registry *Registry, providerDomains map[string]string) *Resolver {
	return &Resolver{registry: registry, providerDomains: providerDomains}
}

// ResolveToken resolves the access token for a task.
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
	}

	return "", fmt.Errorf("no access token available for %s (provide access_token, provider_key, or set %s_TOKEN env var)",
		repoURL, string(repo.Provider))
}
