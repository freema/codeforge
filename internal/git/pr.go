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
