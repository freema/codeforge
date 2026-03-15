package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/review"
)

// GitHubReviewPoster posts review comments to GitHub PRs.
type GitHubReviewPoster struct {
	client *http.Client
}

// NewGitHubReviewPoster creates a new GitHub review poster.
func NewGitHubReviewPoster() *GitHubReviewPoster {
	return &GitHubReviewPoster{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// PostReviewResult represents the outcome of posting a review.
type PostReviewResult struct {
	ReviewURL      string
	CommentsPosted int
}

// githubReviewComment is a single line-level comment in a GitHub review.
type githubReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Body string `json:"body"`
	Side string `json:"side,omitempty"` // "RIGHT" for new file lines
}

// PostPRReview posts a review with line-level comments to a GitHub PR.
func (p *GitHubReviewPoster) PostPRReview(ctx context.Context, repo *RepoInfo, token string, prNumber int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string, formatIssue func(review.ReviewIssue) string) (*PostReviewResult, error) {
	apiURL := repo.APIURL()
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", apiURL, repo.Owner, repo.Repo, prNumber)

	// Separate issues: those with file+line go as review comments, others go in summary body
	comments := make([]githubReviewComment, 0)
	var nonFileIssues []review.ReviewIssue

	for _, issue := range result.Issues {
		if issue.File != "" && issue.Line > 0 {
			comments = append(comments, githubReviewComment{
				Path: issue.File,
				Line: issue.Line,
				Body: formatIssue(issue),
				Side: "RIGHT",
			})
		} else {
			nonFileIssues = append(nonFileIssues, issue)
		}
	}

	// Limit to 20 line comments (GitHub has limits), move rest to summary
	const maxLineComments = 20
	if len(comments) > maxLineComments {
		for _, c := range comments[maxLineComments:] {
			nonFileIssues = append(nonFileIssues, review.ReviewIssue{
				File:        c.Path,
				Line:        c.Line,
				Description: c.Body,
			})
		}
		comments = comments[:maxLineComments]
	}

	// Map verdict to GitHub review event
	event := "COMMENT"
	switch result.Verdict {
	case review.VerdictApprove:
		event = "APPROVE"
	case review.VerdictRequestChanges:
		event = "REQUEST_CHANGES"
	}

	body := map[string]interface{}{
		"body":     formatSummary(result, nonFileIssues),
		"event":    event,
		"comments": comments,
	}

	postResult, err := p.doPostReview(ctx, url, token, body)
	if err != nil && len(comments) > 0 && resp422(err) {
		// Line comments failed (lines not in PR diff) — retry without them,
		// putting all issues into the summary body instead.
		slog.Warn("line comments rejected by GitHub, retrying without inline comments",
			"error", err, "comments_dropped", len(comments))

		body = map[string]interface{}{
			"body":     formatSummary(result, result.Issues),
			"event":    event,
			"comments": []githubReviewComment{},
		}
		postResult, err = p.doPostReview(ctx, url, token, body)
	}
	if err != nil {
		return nil, err
	}

	return postResult, nil
}

// doPostReview sends the review request to GitHub API.
func (p *GitHubReviewPoster) doPostReview(ctx context.Context, url, token string, body map[string]interface{}) (*PostReviewResult, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling review request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating review request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github review API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading github review response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github review API returned %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var respData struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		slog.Warn("failed to parse github review response", "error", err)
	}

	commentsField, _ := body["comments"].([]githubReviewComment)
	return &PostReviewResult{
		ReviewURL:      respData.HTMLURL,
		CommentsPosted: len(commentsField),
	}, nil
}

// resp422 checks if an error message indicates a 422 response.
func resp422(err error) bool {
	return err != nil && strings.Contains(err.Error(), "422")
}
