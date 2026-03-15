package git

import (
	"context"
	"fmt"

	"github.com/freema/codeforge/internal/review"
)

// PostReviewComments posts review results as comments to a PR/MR on the appropriate provider.
func PostReviewComments(ctx context.Context, repo *RepoInfo, token string, prNumber int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string, formatIssue func(review.ReviewIssue) string) (*PostReviewResult, error) {
	switch repo.Provider {
	case ProviderGitHub:
		return NewGitHubReviewPoster().PostPRReview(ctx, repo, token, prNumber, result, formatSummary, formatIssue)
	case ProviderGitLab:
		return NewGitLabReviewPoster().PostMRReview(ctx, repo, token, prNumber, result, formatSummary, formatIssue)
	default:
		return nil, fmt.Errorf("review comments not supported for provider: %s", repo.Provider)
	}
}
