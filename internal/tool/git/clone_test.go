package git

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runAskPass executes a generated askpass script with the given git prompt
// and returns its trimmed stdout and whether it answered (exit status 0).
func runAskPass(t *testing.T, scriptPath, prompt string) (string, bool) {
	t.Helper()
	out, err := exec.Command("sh", scriptPath, prompt).Output()
	return strings.TrimSuffix(string(out), "\n"), err == nil
}

const askPassRepoURL = "https://gitlab.example.com/group/repo.git"

func TestCreateAskPassScript(t *testing.T) {
	const promptUser = "Username for 'https://gitlab.example.com': "
	const promptPass = "Password for 'https://gitlab-ci-token@gitlab.example.com': "

	tests := []struct {
		name     string
		token    string
		username string
		prompt   string
		want     string
	}{
		{
			name:     "PAT answers username prompt with token",
			token:    "glpat-token",
			username: "",
			prompt:   promptUser,
			want:     "glpat-token",
		},
		{
			name:     "PAT answers password prompt with token",
			token:    "glpat-token",
			username: "",
			prompt:   promptPass,
			want:     "glpat-token",
		},
		{
			name:     "job token username prompt returns username",
			token:    "job-token-secret",
			username: GitLabCIJobTokenUsername,
			prompt:   promptUser,
			want:     "gitlab-ci-token",
		},
		{
			name:     "job token password prompt returns token",
			token:    "job-token-secret",
			username: GitLabCIJobTokenUsername,
			prompt:   promptPass,
			want:     "job-token-secret",
		},
		{
			name:     "token with single quote is escaped",
			token:    "to'ken",
			username: GitLabCIJobTokenUsername,
			prompt:   promptPass,
			want:     "to'ken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := createAskPassScript(tt.token, tt.username, askPassRepoURL)
			if err != nil {
				t.Fatalf("createAskPassScript: %v", err)
			}
			defer os.Remove(path)

			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat script: %v", err)
			}
			if info.Mode().Perm() != 0700 {
				t.Errorf("script mode = %o, want 0700", info.Mode().Perm())
			}

			if got, ok := runAskPass(t, path, tt.prompt); !ok || got != tt.want {
				t.Errorf("askpass(%q) = %q (answered %v), want %q", tt.prompt, got, ok, tt.want)
			}
		})
	}
}

// TestAskPassScriptOnlyAnswersRepoHost checks that the token is never handed
// to a host other than the one the server cloned from — e.g. when the
// workspace's origin URL or an HTTP redirect points somewhere else.
func TestAskPassScriptOnlyAnswersRepoHost(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		prompt  string
	}{
		{"other host username", askPassRepoURL, "Username for 'https://evil.example': "},
		{"other host password", askPassRepoURL, "Password for 'https://tok@evil.example': "},
		{"host as prefix of another host", askPassRepoURL, "Password for 'https://tok@gitlab.example.com.evil.example': "},
		{"host smuggled in username", askPassRepoURL, "Password for 'https://x@gitlab.example.com@evil.example': "},
		{"scheme downgrade", askPassRepoURL, "Password for 'http://tok@gitlab.example.com': "},
		{"other port", askPassRepoURL, "Password for 'https://tok@gitlab.example.com:8443': "},
		{"empty prompt", askPassRepoURL, ""},
		{"local repository never answers", "/srv/repos/project.git", "Password for 'https://tok@gitlab.example.com': "},
		{"file URL never answers", "file:///srv/repos/project.git", "Username for 'https://gitlab.example.com': "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := createAskPassScript("tok", "", tt.repoURL)
			if err != nil {
				t.Fatalf("createAskPassScript: %v", err)
			}
			defer os.Remove(path)

			if got, ok := runAskPass(t, path, tt.prompt); ok || got != "" {
				t.Errorf("askpass(%q) = %q (answered %v), want no answer", tt.prompt, got, ok)
			}
		})
	}
}

func TestAskPassScriptHostWithPort(t *testing.T) {
	path, err := createAskPassScript("tok", "", "http://127.0.0.1:8080/group/repo.git")
	if err != nil {
		t.Fatalf("createAskPassScript: %v", err)
	}
	defer os.Remove(path)

	for _, prompt := range []string{
		"Username for 'http://127.0.0.1:8080': ",
		"Password for 'http://tok@127.0.0.1:8080': ",
	} {
		if got, ok := runAskPass(t, path, prompt); !ok || got != "tok" {
			t.Errorf("askpass(%q) = %q (answered %v), want token", prompt, got, ok)
		}
	}
}
