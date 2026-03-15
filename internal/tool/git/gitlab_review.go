package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/freema/codeforge/internal/review"
)

// GitLabReviewPoster posts review comments to GitLab MRs.
type GitLabReviewPoster struct {
	client *http.Client
}

// NewGitLabReviewPoster creates a new GitLab review poster.
func NewGitLabReviewPoster() *GitLabReviewPoster {
	return &GitLabReviewPoster{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// gitlabMRVersion holds the commit SHAs needed for position-based discussions.
type gitlabMRVersion struct {
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_sha"`
	HeadCommitSHA  string `json:"head_sha"`
}

// PostMRReview posts review comments as discussions to a GitLab MR.
func (p *GitLabReviewPoster) PostMRReview(ctx context.Context, repo *RepoInfo, token string, mrIID int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string, formatIssue func(review.ReviewIssue) string) (*PostReviewResult, error) {
	apiURL := repo.APIURL()
	projectPath := url.PathEscape(repo.FullName())

	// Fetch MR version info for position-based comments
	version, err := p.getMRVersion(ctx, apiURL, projectPath, token, mrIID)
	if err != nil {
		// Fall back to summary-only comment if we can't get versions
		return p.postSummaryOnly(ctx, apiURL, projectPath, token, mrIID, result, formatSummary)
	}

	// Separate issues into file-level and non-file
	var fileIssues []review.ReviewIssue
	var nonFileIssues []review.ReviewIssue

	for _, issue := range result.Issues {
		if issue.File != "" && issue.Line > 0 {
			fileIssues = append(fileIssues, issue)
		} else {
			nonFileIssues = append(nonFileIssues, issue)
		}
	}

	// Limit line comments
	const maxLineComments = 20
	if len(fileIssues) > maxLineComments {
		nonFileIssues = append(nonFileIssues, fileIssues[maxLineComments:]...)
		fileIssues = fileIssues[:maxLineComments]
	}

	commentsPosted := 0

	// Post line-level discussions
	for _, issue := range fileIssues {
		err := p.postDiscussion(ctx, apiURL, projectPath, token, mrIID, version, issue, formatIssue)
		if err != nil {
			// Non-fatal: move to summary
			nonFileIssues = append(nonFileIssues, issue)
			continue
		}
		commentsPosted++
	}

	// Post summary discussion (no position)
	summaryBody := formatSummary(result, nonFileIssues)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions", apiURL, projectPath, mrIID)
	body, _ := json.Marshal(map[string]string{"body": summaryBody})

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating summary discussion request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab discussion API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab discussion API returned %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var respData struct {
		ID string `json:"id"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(respBody, &respData)

	return &PostReviewResult{
		CommentsPosted: commentsPosted,
	}, nil
}

func (p *GitLabReviewPoster) getMRVersion(ctx context.Context, apiURL, projectPath, token string, mrIID int) (*gitlabMRVersion, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/versions", apiURL, projectPath, mrIID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab versions API returned %d", resp.StatusCode)
	}

	var versions []gitlabMRVersion
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &versions); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no MR versions found")
	}

	// First version is the latest
	return &versions[0], nil
}

func (p *GitLabReviewPoster) postDiscussion(ctx context.Context, apiURL, projectPath, token string, mrIID int, version *gitlabMRVersion, issue review.ReviewIssue, formatIssue func(review.ReviewIssue) string) error {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions", apiURL, projectPath, mrIID)

	body := map[string]interface{}{
		"body": formatIssue(issue),
		"position": map[string]interface{}{
			"base_sha":      version.BaseCommitSHA,
			"start_sha":     version.StartCommitSHA,
			"head_sha":      version.HeadCommitSHA,
			"position_type": "text",
			"new_path":      issue.File,
			"new_line":      issue.Line,
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab discussion returned %d: %s", resp.StatusCode, truncateBytes(respBody, 300))
	}

	return nil
}

func (p *GitLabReviewPoster) postSummaryOnly(ctx context.Context, apiURL, projectPath, token string, mrIID int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string) (*PostReviewResult, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions", apiURL, projectPath, mrIID)

	// All issues go into summary since we can't do position-based comments
	summaryBody := formatSummary(result, result.Issues)
	body, _ := json.Marshal(map[string]string{"body": summaryBody})

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gitlab summary discussion returned %d", resp.StatusCode)
	}

	return &PostReviewResult{CommentsPosted: 0}, nil
}
