package git

import "testing"

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		url      string
		provider Provider
		owner    string
		repo     string
	}{
		{"https://github.com/freema/codeforge.git", ProviderGitHub, "freema", "codeforge"},
		{"https://github.com/freema/codeforge", ProviderGitHub, "freema", "codeforge"},
		{"https://gitlab.com/group/project.git", ProviderGitLab, "group", "project"},
		{"https://gitlab.com/group/subgroup/project.git", ProviderGitLab, "group/subgroup", "project"},
		{"https://example.com/owner/repo.git", ProviderUnknown, "owner", "repo"},
	}

	for _, tt := range tests {
		info, err := ParseRepoURL(tt.url, nil)
		if err != nil {
			t.Fatalf("ParseRepoURL(%q): %v", tt.url, err)
		}
		if info.Provider != tt.provider {
			t.Errorf("ParseRepoURL(%q).Provider = %q, want %q", tt.url, info.Provider, tt.provider)
		}
		if info.Owner != tt.owner {
			t.Errorf("ParseRepoURL(%q).Owner = %q, want %q", tt.url, info.Owner, tt.owner)
		}
		if info.Repo != tt.repo {
			t.Errorf("ParseRepoURL(%q).Repo = %q, want %q", tt.url, info.Repo, tt.repo)
		}
	}
}

func TestParseRepoURL_CustomDomains(t *testing.T) {
	domains := map[string]string{
		"git.company.com": "gitlab",
	}

	info, err := ParseRepoURL("https://git.company.com/team/project.git", domains)
	if err != nil {
		t.Fatalf("ParseRepoURL: %v", err)
	}
	if info.Provider != ProviderGitLab {
		t.Errorf("expected gitlab, got %q", info.Provider)
	}
}

func TestParseRepoURL_SchemeAndPort(t *testing.T) {
	domains := map[string]string{
		"gitlab.example.com":      "gitlab",
		"ghe.example.com":         "github",
		"pinned.example.com:8443": "gitlab",
	}

	tests := []struct {
		name     string
		url      string
		provider Provider
		host     string
		port     string
		scheme   string
	}{
		{"self-hosted gitlab with port", "https://gitlab.example.com:8443/group/project.git", ProviderGitLab, "gitlab.example.com", "8443", "https"},
		{"self-hosted gitlab plain http", "http://gitlab.example.com/group/project.git", ProviderGitLab, "gitlab.example.com", "", "http"},
		{"self-hosted gitlab http with port", "http://gitlab.example.com:8080/group/sub/project.git", ProviderGitLab, "gitlab.example.com", "8080", "http"},
		{"ghe with port", "https://ghe.example.com:8443/owner/repo.git", ProviderGitHub, "ghe.example.com", "8443", "https"},
		{"host:port mapping entry", "https://pinned.example.com:8443/group/project.git", ProviderGitLab, "pinned.example.com", "8443", "https"},
		{"gitlab.com default stays https", "https://gitlab.com/group/project.git", ProviderGitLab, "gitlab.com", "", "https"},
		{"github.com default stays https", "https://github.com/owner/repo.git", ProviderGitHub, "github.com", "", "https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseRepoURL(tt.url, domains)
			if err != nil {
				t.Fatalf("ParseRepoURL(%q): %v", tt.url, err)
			}
			if info.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", info.Provider, tt.provider)
			}
			if info.Host != tt.host {
				t.Errorf("Host = %q, want %q", info.Host, tt.host)
			}
			if info.Port != tt.port {
				t.Errorf("Port = %q, want %q", info.Port, tt.port)
			}
			if info.Scheme != tt.scheme {
				t.Errorf("Scheme = %q, want %q", info.Scheme, tt.scheme)
			}
		})
	}
}

func TestParseRepoURL_Invalid(t *testing.T) {
	_, err := ParseRepoURL("https://github.com/onlyone", nil)
	if err == nil {
		t.Fatal("expected error for URL without repo")
	}
}

func TestRepoInfo_APIURL(t *testing.T) {
	tests := []struct {
		info RepoInfo
		want string
	}{
		{RepoInfo{Provider: ProviderGitHub, Host: "github.com"}, "https://api.github.com"},
		{RepoInfo{Provider: ProviderGitHub, Host: "github.company.com"}, "https://github.company.com/api/v3"},
		{RepoInfo{Provider: ProviderGitLab, Host: "gitlab.com"}, "https://gitlab.com"},
		{RepoInfo{Provider: ProviderGitLab, Host: "git.company.com"}, "https://git.company.com"},
		// Scheme and port from the original URL must survive (self-hosted).
		{RepoInfo{Provider: ProviderGitLab, Host: "gitlab.example.com", Port: "8443", Scheme: "https"}, "https://gitlab.example.com:8443"},
		{RepoInfo{Provider: ProviderGitLab, Host: "gitlab.example.com", Scheme: "http"}, "http://gitlab.example.com"},
		{RepoInfo{Provider: ProviderGitLab, Host: "gitlab.example.com", Port: "8080", Scheme: "http"}, "http://gitlab.example.com:8080"},
		{RepoInfo{Provider: ProviderGitHub, Host: "ghe.example.com", Port: "8443", Scheme: "https"}, "https://ghe.example.com:8443/api/v3"},
		{RepoInfo{Provider: ProviderUnknown, Host: "example.com"}, ""},
	}

	for _, tt := range tests {
		got := tt.info.APIURL()
		if got != tt.want {
			t.Errorf("APIURL() = %q, want %q", got, tt.want)
		}
	}
}

func TestParseRepoURL_APIURL_EndToEnd(t *testing.T) {
	domains := map[string]string{"gitlab.example.com": "gitlab"}

	info, err := ParseRepoURL("http://gitlab.example.com:8080/group/project.git", domains)
	if err != nil {
		t.Fatalf("ParseRepoURL: %v", err)
	}
	if got, want := info.APIURL(), "http://gitlab.example.com:8080"; got != want {
		t.Errorf("APIURL() = %q, want %q", got, want)
	}
}
