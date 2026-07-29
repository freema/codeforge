package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo builds a temp git repository with an initial commit containing
// modify.txt and delete.txt. Local user.name/email so commit works in CI.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitCmd := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", full...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	gitCmd("init")
	gitCmd("config", "user.email", "test@example.com")
	gitCmd("config", "user.name", "Test User")

	writeTestFile(t, dir, "modify.txt", "line1\nline2\nline3\n")
	writeTestFile(t, dir, "delete.txt", "to be deleted\n")

	gitCmd("add", "-A")
	gitCmd("commit", "-m", "initial commit")

	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func removeTestFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
}

func TestDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tests := []struct {
		name      string
		mutate    func(t *testing.T, dir string)
		wantFiles map[string]FileDiff // path → expected status/counts
		wantAdds  int
		wantDels  int
		wantDiff  []string // substrings expected in the unified diff
	}{
		{
			name:      "no changes",
			mutate:    func(t *testing.T, dir string) {},
			wantFiles: map[string]FileDiff{},
		},
		{
			name: "modified file",
			mutate: func(t *testing.T, dir string) {
				writeTestFile(t, dir, "modify.txt", "line1\nCHANGED\nline3\n")
			},
			wantFiles: map[string]FileDiff{
				"modify.txt": {Status: FileStatusModified, Additions: 1, Deletions: 1},
			},
			wantAdds: 1,
			wantDels: 1,
			wantDiff: []string{"diff --git a/modify.txt b/modify.txt", "+CHANGED", "-line2"},
		},
		{
			name: "created untracked file",
			mutate: func(t *testing.T, dir string) {
				writeTestFile(t, dir, "new.txt", "alpha\nbeta\n")
			},
			wantFiles: map[string]FileDiff{
				"new.txt": {Status: FileStatusAdded, Additions: 2, Deletions: 0},
			},
			wantAdds: 2,
			wantDels: 0,
			wantDiff: []string{"diff --git a/new.txt b/new.txt", "+alpha", "+beta"},
		},
		{
			name: "deleted file",
			mutate: func(t *testing.T, dir string) {
				removeTestFile(t, dir, "delete.txt")
			},
			wantFiles: map[string]FileDiff{
				"delete.txt": {Status: FileStatusDeleted, Additions: 0, Deletions: 1},
			},
			wantAdds: 0,
			wantDels: 1,
			wantDiff: []string{"diff --git a/delete.txt b/delete.txt", "-to be deleted"},
		},
		{
			name: "file in subdirectory",
			mutate: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, dir, filepath.Join("sub", "nested.txt"), "nested\n")
			},
			wantFiles: map[string]FileDiff{
				"sub/nested.txt": {Status: FileStatusAdded, Additions: 1, Deletions: 0},
			},
			wantAdds: 1,
			wantDels: 0,
			wantDiff: []string{"diff --git a/sub/nested.txt b/sub/nested.txt"},
		},
		{
			name: "mixed create modify delete",
			mutate: func(t *testing.T, dir string) {
				writeTestFile(t, dir, "modify.txt", "line1\nCHANGED\nline3\n")
				writeTestFile(t, dir, "new.txt", "alpha\nbeta\n")
				removeTestFile(t, dir, "delete.txt")
			},
			wantFiles: map[string]FileDiff{
				"modify.txt": {Status: FileStatusModified, Additions: 1, Deletions: 1},
				"new.txt":    {Status: FileStatusAdded, Additions: 2, Deletions: 0},
				"delete.txt": {Status: FileStatusDeleted, Additions: 0, Deletions: 1},
			},
			wantAdds: 3,
			wantDels: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initTestRepo(t)
			tt.mutate(t, dir)

			got, err := Diff(context.Background(), dir)
			if err != nil {
				t.Fatalf("Diff() error: %v", err)
			}

			if len(got.Files) != len(tt.wantFiles) {
				t.Fatalf("Files count = %d, want %d (files: %+v)", len(got.Files), len(tt.wantFiles), got.Files)
			}
			for _, f := range got.Files {
				want, ok := tt.wantFiles[f.Path]
				if !ok {
					t.Errorf("unexpected file %q in diff", f.Path)
					continue
				}
				if f.Status != want.Status {
					t.Errorf("file %q status = %q, want %q", f.Path, f.Status, want.Status)
				}
				if f.Additions != want.Additions || f.Deletions != want.Deletions {
					t.Errorf("file %q = +%d -%d, want +%d -%d", f.Path, f.Additions, f.Deletions, want.Additions, want.Deletions)
				}
			}

			if got.TotalAdditions != tt.wantAdds {
				t.Errorf("TotalAdditions = %d, want %d", got.TotalAdditions, tt.wantAdds)
			}
			if got.TotalDeletions != tt.wantDels {
				t.Errorf("TotalDeletions = %d, want %d", got.TotalDeletions, tt.wantDels)
			}
			if got.Truncated {
				t.Error("Truncated = true, want false")
			}
			if len(tt.wantFiles) == 0 && got.Diff != "" {
				t.Errorf("Diff = %q, want empty", got.Diff)
			}
			for _, sub := range tt.wantDiff {
				if !strings.Contains(got.Diff, sub) {
					t.Errorf("Diff missing substring %q", sub)
				}
			}
		})
	}
}

func TestDiff_NotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := Diff(context.Background(), t.TempDir()); err == nil {
		t.Fatal("Diff() on a non-repo directory: expected error, got nil")
	}
}

func TestTruncateAtLineBoundary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"under limit", "a\nb\n", 100, "a\nb\n"},
		{"exactly at limit", "abcd", 4, "abcd"},
		{"cut at newline", "line1\nline2\nline3\n", 10, "line1\n"},
		{"cut keeps full lines", "line1\nline2\nline3\n", 12, "line1\nline2\n"},
		{"no newline within limit", "abcdefghij", 5, "abcde"},
		{"empty", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateAtLineBoundary(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateAtLineBoundary(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

func TestParsePorcelainStatuses(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"untracked", "?? new.txt\n", map[string]string{"new.txt": FileStatusAdded}},
		{"intent to add worktree", " A new.txt\n", map[string]string{"new.txt": FileStatusAdded}},
		{"staged add", "A  new.txt\n", map[string]string{"new.txt": FileStatusAdded}},
		{"worktree modified", " M mod.txt\n", map[string]string{"mod.txt": FileStatusModified}},
		{"worktree deleted", " D gone.txt\n", map[string]string{"gone.txt": FileStatusDeleted}},
		{"staged rename", "R  old.txt -> new.txt\n", map[string]string{"old.txt": FileStatusRenamed, "new.txt": FileStatusRenamed}},
		{
			"mixed",
			"?? a.txt\n M b.txt\n D c.txt\n",
			map[string]string{"a.txt": FileStatusAdded, "b.txt": FileStatusModified, "c.txt": FileStatusDeleted},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePorcelainStatuses(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got: %v)", len(got), len(tt.want), got)
			}
			for path, st := range tt.want {
				if got[path] != st {
					t.Errorf("status[%q] = %q, want %q", path, got[path], st)
				}
			}
		})
	}
}
