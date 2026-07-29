package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initSourceRepo builds a local source repository whose default branch is
// defaultBranch, with one commit, plus any extra branches (HEAD stays on the
// default branch). Local user.name/email so commit works in CI.
func initSourceRepo(t *testing.T, defaultBranch string, extraBranches ...string) string {
	t.Helper()
	dir := t.TempDir()

	gitRun := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", full...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	gitRun("init", "-b", defaultBranch)
	gitRun("config", "user.email", "test@example.com")
	gitRun("config", "user.name", "Test User")
	writeTestFile(t, dir, "README.md", "hello\n")
	gitRun("add", "-A")
	gitRun("commit", "-m", "initial commit")

	for _, b := range extraBranches {
		gitRun("branch", b)
	}

	return dir
}

// TestCloneDefaultBranch covers the clone forms the executor relies on for
// pr_review sessions: an empty Branch clones the repository's default branch
// (no -b flag), an explicit Branch is honored, and DefaultBranch detects what
// was checked out — regardless of whether the default is named "main".
func TestCloneDefaultBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tests := []struct {
		name           string
		defaultBranch  string
		extraBranches  []string
		cloneBranch    string
		wantCheckedOut string
	}{
		{
			name:           "empty branch clones repository default (master)",
			defaultBranch:  "master",
			cloneBranch:    "",
			wantCheckedOut: "master",
		},
		{
			name:           "empty branch clones repository default (trunk)",
			defaultBranch:  "trunk",
			cloneBranch:    "",
			wantCheckedOut: "trunk",
		},
		{
			name:           "explicit branch is honored",
			defaultBranch:  "trunk",
			extraBranches:  []string{"feature-x"},
			cloneBranch:    "feature-x",
			wantCheckedOut: "feature-x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			src := initSourceRepo(t, tt.defaultBranch, tt.extraBranches...)
			dest := filepath.Join(t.TempDir(), "clone")

			err := Clone(ctx, CloneOptions{
				RepoURL: src,
				DestDir: dest,
				Branch:  tt.cloneBranch,
			})
			if err != nil {
				t.Fatalf("Clone() error: %v", err)
			}

			out, err := exec.Command("git", "-C", dest, "rev-parse", "--abbrev-ref", "HEAD").Output()
			if err != nil {
				t.Fatalf("rev-parse HEAD: %v", err)
			}
			if got := strings.TrimSpace(string(out)); got != tt.wantCheckedOut {
				t.Errorf("checked-out branch = %q, want %q", got, tt.wantCheckedOut)
			}

			// The executor records the resolved branch via DefaultBranch after
			// a plain clone — verify detection on the default-branch clones.
			if tt.cloneBranch == "" {
				got, dErr := DefaultBranch(ctx, dest)
				if dErr != nil {
					t.Fatalf("DefaultBranch() error: %v", dErr)
				}
				if got != tt.defaultBranch {
					t.Errorf("DefaultBranch() = %q, want %q", got, tt.defaultBranch)
				}
			}
		})
	}
}
