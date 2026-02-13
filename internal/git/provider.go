package git

import (
	"fmt"
	"net/url"
	"strings"
)

// Provider represents a git hosting provider.
type Provider string

const (
	ProviderGitHub  Provider = "github"
	ProviderGitLab  Provider = "gitlab"
	ProviderUnknown Provider = "unknown"
)

// RepoInfo holds parsed repository information.
type RepoInfo struct {
	Provider Provider
	Host     string
	Owner    string
	Repo     string
}

// FullName returns "owner/repo".
func (r RepoInfo) FullName() string {
	return r.Owner + "/" + r.Repo
}

// APIURL returns the base API URL for the provider.
func (r RepoInfo) APIURL() string {
	switch r.Provider {
	case ProviderGitHub:
		if r.Host == "github.com" {
			return "https://api.github.com"
		}
		return "https://" + r.Host + "/api/v3" // GitHub Enterprise
	case ProviderGitLab:
		return "https://" + r.Host
	default:
		return ""
	}
}

// ParseRepoURL extracts provider, owner, and repo from a git URL.
// Supports HTTPS URLs like https://github.com/owner/repo.git
// Custom domain mapping via providerDomains config.
func ParseRepoURL(repoURL string, providerDomains map[string]string) (*RepoInfo, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("invalid repo URL: %w", err)
	}

	host := strings.ToLower(u.Hostname())
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("cannot extract owner/repo from URL: %s", repoURL)
	}

	owner := parts[0]
	repo := parts[1]
	// Handle GitLab subgroups: group/subgroup/repo
	if len(parts) > 2 {
		owner = strings.Join(parts[:len(parts)-1], "/")
		repo = parts[len(parts)-1]
	}

	provider := detectProvider(host, providerDomains)

	return &RepoInfo{
		Provider: provider,
		Host:     host,
		Owner:    owner,
		Repo:     repo,
	}, nil
}

func detectProvider(host string, customDomains map[string]string) Provider {
	// Check custom domains first
	if customDomains != nil {
		if p, ok := customDomains[host]; ok {
			switch strings.ToLower(p) {
			case "github":
				return ProviderGitHub
			case "gitlab":
				return ProviderGitLab
			}
		}
	}

	// Standard detection
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return ProviderGitHub
	case host == "gitlab.com" || strings.HasSuffix(host, ".gitlab.com"):
		return ProviderGitLab
	default:
		return ProviderUnknown
	}
}
