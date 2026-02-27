package git

import (
	"context"
	"fmt"
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

