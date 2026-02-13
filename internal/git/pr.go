package git

import (
	"context"
	"fmt"
	"strings"
)

// PRCreateOptions holds parameters for PR/MR creation.
type PRCreateOptions struct {
	Title       string
	Description string
	Branch      string
	BaseBranch  string
}

// PRCreator is the interface for creating pull/merge requests.
type PRCreator interface {
	Create(ctx context.Context, repo *RepoInfo, token string, opts PRCreateOptions) (*PRResult, error)
}

// CreatePR creates a PR/MR on the appropriate provider.
func CreatePR(ctx context.Context, repo *RepoInfo, token string, opts PRCreateOptions) (*PRResult, error) {
	switch repo.Provider {
	case ProviderGitHub:
		return NewGitHubPRCreator().CreatePR(ctx, repo, token, opts)
	case ProviderGitLab:
		return NewGitLabMRCreator().CreateMR(ctx, repo, token, opts)
	default:
		return nil, fmt.Errorf("PR creation not supported for provider: %s", repo.Provider)
	}
}

// UpdatePRDescription updates an existing PR/MR description.
func UpdatePRDescription(ctx context.Context, repo *RepoInfo, token string, prNumber int, body string) error {
	switch repo.Provider {
	case ProviderGitHub:
		return NewGitHubPRCreator().UpdatePR(ctx, repo, token, prNumber, body)
	case ProviderGitLab:
		return NewGitLabMRCreator().UpdateMR(ctx, repo, token, prNumber, body)
	default:
		return fmt.Errorf("PR update not supported for provider: %s", repo.Provider)
	}
}

// PushExistingBranch stages, commits, and pushes to an existing branch.
func PushExistingBranch(ctx context.Context, opts BranchOptions) error {
	workDir := opts.WorkDir

	// Stage all changes
	if err := gitCmd(ctx, workDir, nil, "add", "-A"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	// Check if there's anything to commit
	statusOut, err := gitOutput(ctx, workDir, nil, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if strings.TrimSpace(statusOut) == "" {
		return nil // nothing new to commit
	}

	// Commit
	commitEnv := []string{
		"GIT_AUTHOR_NAME=" + opts.AuthorName,
		"GIT_AUTHOR_EMAIL=" + opts.AuthorEmail,
		"GIT_COMMITTER_NAME=" + opts.AuthorName,
		"GIT_COMMITTER_EMAIL=" + opts.AuthorEmail,
	}
	if err := gitCmd(ctx, workDir, commitEnv, "commit", "-m", opts.CommitMsg); err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}

	// Push via GIT_ASKPASS
	pushEnv, cleanup, err := AskPassEnv(opts.Token)
	if err != nil {
		return fmt.Errorf("preparing push credentials: %w", err)
	}
	defer cleanup()

	if err := gitCmd(ctx, workDir, pushEnv, "push", "origin", opts.BranchName); err != nil {
		return fmt.Errorf("pushing to branch: %w", err)
	}

	return nil
}
