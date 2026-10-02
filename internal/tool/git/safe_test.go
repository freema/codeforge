package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// rawGit runs git without any hardening, the way the AI CLI (or a script it
// runs) would in the workspace.
func rawGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// cloneWorkspace clones a fresh workspace from a local bare remote and returns
// (workspace, remote).
func cloneWorkspace(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := initSourceRepo(t, "main")
	remote := filepath.Join(t.TempDir(), "remote.git")
	rawGit(t, src, "clone", "--bare", src, remote)

	ws := filepath.Join(t.TempDir(), "ws")
	if err := Clone(context.Background(), CloneOptions{RepoURL: remote, DestDir: ws}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	return ws, remote
}

// writeExecutable writes a shell script and returns its path.
func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestServerGitIgnoresWorkspaceExecutors plants every kind of command the
// workspace's git configuration can make git run — fsmonitor, hooks, filter
// and diff drivers, in .git/config, in an included file and in the global
// config — then drives the server-side git operations over that workspace.
// None of the planted programs may run, and the server's secrets must not
// reach any git process.
func TestServerGitIgnoresWorkspaceExecutors(t *testing.T) {
	ws, remote := cloneWorkspace(t)
	ctx := context.Background()

	t.Setenv("CODEFORGE_ENCRYPTION__KEY", "server-secret-key")
	t.Setenv("GITHUB_TOKEN", "server-github-token")

	tools := t.TempDir()
	marker := filepath.Join(tools, "ran")
	record := "echo \"$0 $*\" >> " + marker + "\nenv >> " + marker + "\n"
	run := writeExecutable(t, tools, "run.sh", record+"exit 0\n")
	filter := writeExecutable(t, tools, "filter.sh", record+"cat\n")
	textconv := writeExecutable(t, tools, "textconv.sh", record+"cat \"$1\"\n")

	hooks := filepath.Join(tools, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"pre-commit", "commit-msg", "post-commit", "pre-push", "post-checkout", "reference-transaction"} {
		writeExecutable(t, hooks, hook, record+"exit 0\n")
		writeExecutable(t, filepath.Join(ws, ".git", "hooks"), hook, record+"exit 0\n")
	}

	included := filepath.Join(tools, "included.gitconfig")
	if err := os.WriteFile(included, []byte("[diff \"evil2\"]\n\ttextconv = "+textconv+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rawGit(t, ws, "config", "core.fsmonitor", run)
	rawGit(t, ws, "config", "core.hooksPath", hooks)
	rawGit(t, ws, "config", "filter.evil.clean", filter)
	rawGit(t, ws, "config", "filter.evil.smudge", filter)
	rawGit(t, ws, "config", "diff.evil.textconv", textconv)
	rawGit(t, ws, "config", "diff.external", run)
	rawGit(t, ws, "config", "include.path", included)

	// The CLI shares HOME with the server, so it can write the global config too.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	globalCfg := "[filter \"evil3\"]\n\tclean = " + filter + "\n[core]\n\tfsmonitor = " + run + "\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(globalCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	attrs := "*.md filter=evil diff=evil\n*.txt filter=evil3 diff=evil2\n"
	writeTestFile(t, ws, ".gitattributes", attrs)
	writeTestFile(t, ws, "README.md", "changed by the session\n")
	writeTestFile(t, ws, "notes.txt", "new file\n")

	if _, err := CalculateChanges(ctx, ws); err != nil {
		t.Fatalf("CalculateChanges: %v", err)
	}
	if _, err := Diff(ctx, ws); err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if _, err := GetUnstagedDiff(ctx, ws); err != nil {
		t.Fatalf("GetUnstagedDiff: %v", err)
	}
	if _, err := DefaultBranch(ctx, ws); err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	err := CreateBranchAndPush(ctx, BranchOptions{
		WorkDir:     ws,
		RepoURL:     remote,
		BranchName:  "codeforge/test",
		CommitMsg:   "test",
		AuthorName:  "CodeForge",
		AuthorEmail: "codeforge@example.com",
	})
	if err != nil {
		t.Fatalf("CreateBranchAndPush: %v", err)
	}

	if out, err := os.ReadFile(marker); err == nil {
		t.Fatalf("workspace-controlled program ran during server git operations:\n%s", out)
	}
	if got := rawGit(t, remote, "rev-parse", "--verify", "refs/heads/codeforge/test"); got == "" {
		t.Error("branch was not pushed")
	}
}

// TestPushIgnoresWorkspaceOrigin points the workspace's origin (and an
// insteadOf rewrite of the real remote) at a server that asks for
// credentials. The push must go to the URL the server cloned from and the
// token must never reach the other server.
func TestPushIgnoresWorkspaceOrigin(t *testing.T) {
	ws, remote := cloneWorkspace(t)
	ctx := context.Background()

	var (
		mu       sync.Mutex
		requests []string
	)
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.String()+" auth="+r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer evil.Close()

	rawGit(t, ws, "remote", "set-url", "origin", evil.URL+"/group/repo.git")
	rawGit(t, ws, "config", "remote.origin.pushurl", evil.URL+"/group/repo.git")
	rawGit(t, ws, "config", "url."+evil.URL+"/.insteadOf", remote)
	writeTestFile(t, ws, "README.md", "changed by the session\n")

	err := CreateBranchAndPush(ctx, BranchOptions{
		WorkDir:     ws,
		RepoURL:     remote,
		BranchName:  "codeforge/test",
		CommitMsg:   "test",
		AuthorName:  "CodeForge",
		AuthorEmail: "codeforge@example.com",
		Token:       "secret-token",
	})

	mu.Lock()
	defer mu.Unlock()
	if len(requests) > 0 {
		t.Fatalf("push contacted the workspace-chosen remote: %v", requests)
	}
	if err != nil {
		t.Fatalf("CreateBranchAndPush: %v", err)
	}
	if got := rawGit(t, ws, "config", "--get", "remote.origin.url"); got != remote {
		t.Errorf("origin = %q, want %q", got, remote)
	}
}

func TestCommandEnvironmentExcludesServerSecrets(t *testing.T) {
	secrets := map[string]string{
		"CODEFORGE_ENCRYPTION__KEY":    "k",
		"CODEFORGE_SERVER__AUTH_TOKEN": "t",
		"CODEFORGE_REDIS__URL":         "redis://:pw@redis:6379",
		"GITHUB_TOKEN":                 "ghp",
		"GITLAB_TOKEN":                 "glpat",
		"AWS_SECRET_ACCESS_KEY":        "aws",
		"GIT_DIR":                      "/elsewhere",
		"GIT_CONFIG_PARAMETERS":        "'core.fsmonitor'='/tmp/x'",
		"GIT_SSH_COMMAND":              "/tmp/x",
	}
	for k, v := range secrets {
		t.Setenv(k, v)
	}
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")

	cmd := Command(context.Background(), t.TempDir(), []string{"GIT_ASKPASS=/tmp/askpass"}, "status")
	env := map[string]string{}
	for _, kv := range cmd.Env {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	for k := range secrets {
		if _, ok := env[k]; ok {
			t.Errorf("%s reached the git environment", k)
		}
	}
	for k, want := range map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "/tmp/askpass",
		"HTTPS_PROXY":         "http://proxy:3128",
	} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
	if env["PATH"] == "" {
		t.Error("PATH missing from the git environment")
	}
}

func TestSanitizeRepoConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("keeps origin as data for local commands", func(t *testing.T) {
		ws, remote := cloneWorkspace(t)
		rawGit(t, ws, "config", "user.name", "someone")
		if err := SanitizeRepoConfig(ctx, ws, ""); err != nil {
			t.Fatal(err)
		}
		if got := rawGit(t, ws, "config", "--get", "remote.origin.url"); got != remote {
			t.Errorf("origin = %q, want %q", got, remote)
		}
		if out, err := exec.Command("git", "-C", ws, "config", "--local", "--get", "user.name").Output(); err == nil {
			t.Errorf("user.name survived: %q", out)
		}
		// The repository still works.
		rawGit(t, ws, "status", "--porcelain")
		rawGit(t, ws, "rev-parse", "--verify", "refs/remotes/origin/main")
	})

	t.Run("sets the given origin", func(t *testing.T) {
		ws, _ := cloneWorkspace(t)
		want := `https://gitlab.example.com/group/re"po.git`
		if err := SanitizeRepoConfig(ctx, ws, want); err != nil {
			t.Fatal(err)
		}
		if got := rawGit(t, ws, "config", "--get", "remote.origin.url"); got != want {
			t.Errorf("origin = %q, want %q", got, want)
		}
	})

	t.Run("refuses a gitfile", func(t *testing.T) {
		ws := t.TempDir()
		writeTestFile(t, ws, ".git", "gitdir: /elsewhere\n")
		if err := SanitizeRepoConfig(ctx, ws, ""); err == nil {
			t.Error("expected an error for a .git file")
		}
	})

	t.Run("refuses a symlinked .git", func(t *testing.T) {
		other, _ := cloneWorkspace(t)
		ws := t.TempDir()
		if err := os.Symlink(filepath.Join(other, ".git"), filepath.Join(ws, ".git")); err != nil {
			t.Fatal(err)
		}
		if err := SanitizeRepoConfig(ctx, ws, ""); err == nil {
			t.Error("expected an error for a symlinked .git")
		}
	})

	t.Run("refuses commondir", func(t *testing.T) {
		ws, _ := cloneWorkspace(t)
		writeTestFile(t, filepath.Join(ws, ".git"), "commondir", "/elsewhere\n")
		if err := SanitizeRepoConfig(ctx, ws, ""); err == nil {
			t.Error("expected an error for .git/commondir")
		}
	})

	t.Run("refuses a multi-line origin", func(t *testing.T) {
		ws, _ := cloneWorkspace(t)
		if err := SanitizeRepoConfig(ctx, ws, "https://example.com/a\n[core]\nfsmonitor=x"); err == nil {
			t.Error("expected an error for a multi-line origin")
		}
	})
}
