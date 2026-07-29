package git

import (
	"context"
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

// DomainsSource supplies host → provider mappings for repository URL
// detection. It is evaluated at call time, so provider keys created at
// runtime (e.g. via the keys API/UI) are honored without a server restart.
type DomainsSource interface {
	ProviderDomains(ctx context.Context) map[string]string
}

// StaticDomains adapts a fixed map (e.g. from config) to a DomainsSource.
type StaticDomains map[string]string

// ProviderDomains returns the underlying map.
func (m StaticDomains) ProviderDomains(context.Context) map[string]string { return m }

// RepoInfo holds parsed repository information.
type RepoInfo struct {
	Provider Provider
	Host     string // hostname without port
	Port     string // port from the repo URL, "" when none
	Scheme   string // "http" or "https"; defaults to https
	Owner    string
	Repo     string
}

// FullName returns "owner/repo".
func (r RepoInfo) FullName() string {
	return r.Owner + "/" + r.Repo
}

// BaseURL returns "scheme://host[:port]", preserving the scheme and port of
// the original repository URL. Bare hosts default to https.
func (r RepoInfo) BaseURL() string {
	scheme := r.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := r.Host
	if r.Port != "" {
		host += ":" + r.Port
	}
	return scheme + "://" + host
}

// APIURL returns the base API URL for the provider.
func (r RepoInfo) APIURL() string {
	switch r.Provider {
	case ProviderGitHub:
		if r.Host == "github.com" {
			return "https://api.github.com"
		}
		return r.BaseURL() + "/api/v3" // GitHub Enterprise
	case ProviderGitLab:
		return r.BaseURL()
	default:
		return ""
	}
}

// ParseRepoURL extracts provider, owner, and repo from a git URL.
// Supports HTTP(S) URLs like https://github.com/owner/repo.git, including
// self-hosted instances with custom ports.
// Custom domain mapping via providerDomains config.
func ParseRepoURL(repoURL string, providerDomains map[string]string) (*RepoInfo, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("invalid repo URL: %w", err)
	}

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "https"
	}
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

	provider := detectProvider(host, port, providerDomains)

	return &RepoInfo{
		Provider: provider,
		Host:     host,
		Port:     port,
		Scheme:   scheme,
		Owner:    owner,
		Repo:     repo,
	}, nil
}

func detectProvider(host, port string, customDomains map[string]string) Provider {
	// Check custom domains first — "host:port" entries take precedence over
	// bare "host" entries so instances on non-standard ports can be pinned.
	if customDomains != nil {
		lookups := []string{host}
		if port != "" {
			lookups = []string{host + ":" + port, host}
		}
		for _, key := range lookups {
			if p, ok := customDomains[key]; ok {
				switch strings.ToLower(p) {
				case string(ProviderGitHub):
					return ProviderGitHub
				case string(ProviderGitLab):
					return ProviderGitLab
				}
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
