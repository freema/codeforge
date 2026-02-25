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
	}

	for _, tt := range tests {
		got := tt.info.APIURL()
		if got != tt.want {
			t.Errorf("APIURL() = %q, want %q", got, tt.want)
		}
	}
}
