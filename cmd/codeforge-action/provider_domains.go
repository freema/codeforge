package main

import (
	"net/url"
	"os"
	"strings"

	"github.com/freema/codeforge/internal/tool/git"
)

// envProviderDomains builds a host → provider map from CI environment
// variables so self-managed instances are recognized without configuration.
// The CI action has no key registry (that mechanism is server-only), so
// env-derived domains are the single source here:
//
//   - CI_SERVER_URL      — set automatically by GitLab CI on every job
//   - GITLAB_URL         — manual override, matches server-side config
//   - GITHUB_SERVER_URL  — set automatically by GitHub Actions (GHE included)
//   - GITHUB_URL         — manual override, matches server-side config
//
// Well-known public hosts are skipped (detected natively by ParseRepoURL).
// When the URL carries a port, a "host:port" entry is added alongside the
// bare host entry; earlier sources win on conflicts. Returns nil when no
// custom domain is found.
func envProviderDomains() map[string]string {
	domains := make(map[string]string)

	sources := []struct {
		envVar   string
		provider string
	}{
		{"CI_SERVER_URL", string(git.ProviderGitLab)},
		{"GITLAB_URL", string(git.ProviderGitLab)},
		{"GITHUB_SERVER_URL", string(git.ProviderGitHub)},
		{"GITHUB_URL", string(git.ProviderGitHub)},
	}

	for _, s := range sources {
		raw := strings.TrimSpace(os.Getenv(s.envVar))
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "" || host == "github.com" || host == "gitlab.com" {
			continue
		}
		if port := u.Port(); port != "" {
			key := host + ":" + port
			if _, ok := domains[key]; !ok {
				domains[key] = s.provider
			}
		}
		if _, ok := domains[host]; !ok {
			domains[host] = s.provider
		}
	}

	if len(domains) == 0 {
		return nil
	}
	return domains
}
