package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// BranchOptions configures branch creation and push.
type BranchOptions struct {
	WorkDir     string
	BranchName  string
	BaseBranch  string // If set, the feature branch is created from origin/<BaseBranch> instead of the current HEAD.
	CommitMsg   string
	AuthorName  string
	AuthorEmail string
	Token       string
}

// CreateBranchAndPush creates a new branch, stages all changes, commits, and pushes.
// Token is passed via GIT_ASKPASS (never in URL or .git/config).
func CreateBranchAndPush(ctx context.Context, opts BranchOptions) error {
	workDir := opts.WorkDir

	// Create and checkout branch from current HEAD.
	// The branch is based on whatever was cloned — the MR/PR target branch
	// is specified separately in the API call, not via git ancestry.
	if err := gitCmd(ctx, workDir, nil, "checkout", "-b", opts.BranchName); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}
	slog.Info("branch created", "branch", opts.BranchName)

	// Remove generated files that must not be committed
	for _, f := range []string{".mcp.json"} {
		os.Remove(workDir + "/" + f) // best-effort, ignore errors
	}

	// Stage all changes
	if err := gitCmd(ctx, workDir, nil, "add", "-A"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	// Check if there's anything to commit
	statusOut, err := gitOutput(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if strings.TrimSpace(statusOut) == "" {
		return fmt.Errorf("nothing to commit")
	}

	// Commit with author info
	commitEnv := []string{
		"GIT_AUTHOR_NAME=" + opts.AuthorName,
		"GIT_AUTHOR_EMAIL=" + opts.AuthorEmail,
		"GIT_COMMITTER_NAME=" + opts.AuthorName,
		"GIT_COMMITTER_EMAIL=" + opts.AuthorEmail,
	}
	if err := gitCmd(ctx, workDir, commitEnv, "commit", "-m", opts.CommitMsg); err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}
	slog.Info("changes committed", "branch", opts.BranchName)

	// Push via GIT_ASKPASS
	pushEnv, cleanup, err := AskPassEnv(opts.Token)
	if err != nil {
		return fmt.Errorf("preparing push credentials: %w", err)
	}
	defer cleanup()

	if err := gitCmd(ctx, workDir, pushEnv, "push", "-u", "origin", opts.BranchName); err != nil {
		return fmt.Errorf("pushing branch: %w", err)
	}
	slog.Info("branch pushed", "branch", opts.BranchName)

	return nil
}

// AskPassEnv prepares GIT_ASKPASS environment for authenticated git operations.
// Returns extra env vars and a cleanup function.
func AskPassEnv(token string) ([]string, func(), error) {
	if token == "" {
		return nil, func() {}, nil
	}

	askPassFile, err := createAskPassScript(token)
	if err != nil {
		return nil, nil, err
	}

	env := []string{
		"GIT_ASKPASS=" + askPassFile,
		"GIT_TERMINAL_PROMPT=0",
	}
	cleanup := func() { os.Remove(askPassFile) }
	return env, cleanup, nil
}

// gitCmd runs a git command in the given directory with optional extra env vars.
func gitCmd(ctx context.Context, workDir string, extraEnv []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitOutput runs a git command and returns stdout.
func gitOutput(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GenerateBranchName creates a branch name with prefix and slug, adding numeric suffix if needed.
func GenerateBranchName(ctx context.Context, workDir, prefix, slug string) string {
	base := prefix + slug
	name := base

	// Check if branch exists locally or in remote refs
	for i := 1; i <= 99; i++ {
		if !branchExists(ctx, workDir, name) {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}

	return name
}

// DefaultBranch detects the default branch of the cloned repository
// by reading the symbolic-ref of origin/HEAD.
func DefaultBranch(ctx context.Context, workDir string) (string, error) {
	// Try symbolic-ref first (set by clone)
	out, err := gitOutput(ctx, workDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// Output: "refs/remotes/origin/master" → extract "master"
		ref := strings.TrimSpace(out)
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/"), nil
		}
	}
	// Fallback: check current branch name (what was cloned)
	out, err = gitOutput(ctx, workDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		branch := strings.TrimSpace(out)
		if branch != "" && branch != "HEAD" {
			return branch, nil
		}
	}
	return "", fmt.Errorf("could not detect default branch")
}

func branchExists(ctx context.Context, workDir, name string) bool {
	// Check local
	err := gitCmd(ctx, workDir, nil, "rev-parse", "--verify", name)
	if err == nil {
		return true
	}
	// Check remote
	err = gitCmd(ctx, workDir, nil, "rev-parse", "--verify", "origin/"+name)
	return err == nil
}

// GetUnstagedDiff returns the diff of all uncommitted changes in the workspace.
func GetUnstagedDiff(ctx context.Context, workDir string) (string, error) {
	return gitOutput(ctx, workDir, "diff", "HEAD")
}

// PushExistingOptions configures pushing follow-up changes to an existing branch.
type PushExistingOptions struct {
	WorkDir     string
	BranchName  string
	CommitMsg   string
	AuthorName  string
	AuthorEmail string
	Token       string
}

// CommitAndPushToExisting stages all changes, commits, and pushes to an existing branch.
// Returns an error if there are no new changes to push.
func CommitAndPushToExisting(ctx context.Context, opts PushExistingOptions) error {
	workDir := opts.WorkDir

	// Stage all changes
	if err := gitCmd(ctx, workDir, nil, "add", "-A"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	// Check if there are any changes to commit
	statusOut, err := gitOutput(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if strings.TrimSpace(statusOut) == "" {
		return fmt.Errorf("no new changes to push")
	}

	// Commit with author info
	commitEnv := []string{
		"GIT_AUTHOR_NAME=" + opts.AuthorName,
		"GIT_AUTHOR_EMAIL=" + opts.AuthorEmail,
		"GIT_COMMITTER_NAME=" + opts.AuthorName,
		"GIT_COMMITTER_EMAIL=" + opts.AuthorEmail,
	}
	if err := gitCmd(ctx, workDir, commitEnv, "commit", "-m", opts.CommitMsg); err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}
	slog.Info("follow-up changes committed", "branch", opts.BranchName)

	// Push via GIT_ASKPASS
	pushEnv, cleanup, err := AskPassEnv(opts.Token)
	if err != nil {
		return fmt.Errorf("preparing push credentials: %w", err)
	}
	defer cleanup()

	if err := gitCmd(ctx, workDir, pushEnv, "push", "origin", opts.BranchName); err != nil {
		return fmt.Errorf("pushing to branch: %w", err)
	}
	slog.Info("follow-up changes pushed", "branch", opts.BranchName)

	return nil
}
