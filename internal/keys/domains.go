package keys

import (
	"context"
	"log/slog"
)

// Git provider identifiers as stored on keys.
const (
	gitProviderGitHub = "github"
	gitProviderGitLab = "gitlab"
)

// DomainSource derives host → provider mappings for repo URL detection at
// call time: the static base map (config merged with GITLAB_URL/GITHUB_URL
// env vars) is enriched with hostnames extracted from the base_url of every
// stored github/gitlab key. It implements git.DomainsSource, so a key
// created at runtime (API/UI) makes its self-hosted instance recognized on
// all write paths (clone auth, MR create/status, review posting) without a
// server restart.
type DomainSource struct {
	registry Registry
	static   map[string]string
}

// NewDomainSource creates a DomainSource over the given registry and static
// config/env map. Either argument may be nil.
func NewDomainSource(registry Registry, static map[string]string) *DomainSource {
	return &DomainSource{registry: registry, static: static}
}

// ProviderDomains returns the merged host → provider map. Static entries
// (explicit config or env) take precedence over key-derived ones; well-known
// public hosts are skipped (auto-detected without any mapping).
func (s *DomainSource) ProviderDomains(ctx context.Context) map[string]string {
	merged := make(map[string]string, len(s.static))
	for k, v := range s.static {
		merged[k] = v
	}
	if s.registry == nil {
		return merged
	}

	storedKeys, err := s.registry.List(ctx)
	if err != nil {
		slog.Warn("domain source: listing keys failed, using static provider domains only", "error", err)
		return merged
	}

	for _, k := range storedKeys {
		if k.Provider != gitProviderGitHub && k.Provider != gitProviderGitLab {
			continue
		}
		if k.BaseURL == "" {
			continue
		}
		host := extractHost(k.BaseURL)
		if host == "" || host == "github.com" || host == "gitlab.com" {
			continue
		}
		if _, exists := merged[host]; !exists {
			merged[host] = k.Provider
		}
	}

	return merged
}
