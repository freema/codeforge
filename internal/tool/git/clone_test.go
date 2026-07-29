package git

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runAskPass executes a generated askpass script with the given git prompt
// and returns its trimmed stdout.
func runAskPass(t *testing.T, scriptPath, prompt string) string {
	t.Helper()
	out, err := exec.Command("sh", scriptPath, prompt).Output()
	if err != nil {
		t.Fatalf("running askpass script: %v", err)
	}
	return strings.TrimSuffix(string(out), "\n")
}

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
		{
			name:     "empty prompt with username falls through to token",
			token:    "job-token-secret",
			username: GitLabCIJobTokenUsername,
			prompt:   "",
			want:     "job-token-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := createAskPassScript(tt.token, tt.username)
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

			if got := runAskPass(t, path, tt.prompt); got != tt.want {
				t.Errorf("askpass(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}
