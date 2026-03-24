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

// PRStatus represents the state of a PR/MR on the provider.
type PRStatus struct {
	State    string `json:"state"`    // "open", "merged", "closed"
	Title    string `json:"title"`
	Merged   bool   `json:"merged"`
	MergedBy string `json:"merged_by,omitempty"`
}

// GetPRStatus fetches the current status of a PR/MR from the provider.
func GetPRStatus(ctx context.Context, repo *RepoInfo, token string, prNumber int) (*PRStatus, error) {
	switch repo.Provider {
	case ProviderGitHub:
		return NewGitHubPRCreator().GetPRStatus(ctx, repo, token, prNumber)
	case ProviderGitLab:
		return NewGitLabMRCreator().GetMRStatus(ctx, repo, token, prNumber)
	default:
		return nil, fmt.Errorf("PR status not supported for provider: %s", repo.Provider)
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
