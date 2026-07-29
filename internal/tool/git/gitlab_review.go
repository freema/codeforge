package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
// The /versions endpoint returns *_commit_sha field names; the position
// payload uses the shorter base_sha/start_sha/head_sha keys.
type gitlabMRVersion struct {
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_commit_sha"`
	HeadCommitSHA  string `json:"head_commit_sha"`
}

// gitlabDiscussion is the subset of the discussions API response we read.
type gitlabDiscussion struct {
	ID    string `json:"id"`
	Notes []struct {
		ID int `json:"id"`
	} `json:"notes"`
}

// PostMRReview posts review comments as discussions to a GitLab MR.
func (p *GitLabReviewPoster) PostMRReview(ctx context.Context, repo *RepoInfo, token string, mrIID int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string, formatIssue func(review.ReviewIssue) string) (*PostReviewResult, error) {
	apiURL := repo.APIURL()
	projectPath := url.PathEscape(repo.FullName())

	// Verdict mapping: approve → MR approvals API (best-effort).
	// request_changes has no core GitLab equivalent and stays summary text.
	if result.Verdict == review.VerdictApprove {
		p.approveMR(ctx, apiURL, projectPath, token, mrIID)
	}

	// Fetch MR version info for position-based comments
	version, err := p.getMRVersion(ctx, apiURL, projectPath, token, mrIID)
	if err != nil {
		// Fall back to summary-only comment if we can't get versions
		slog.Warn("failed to fetch MR versions, posting summary-only review", "error", err)
		return p.postSummaryOnly(ctx, repo, apiURL, projectPath, token, mrIID, result, formatSummary)
	}

	// Fetch the MR diff to validate inline comments and to resolve
	// old_path/old_line for context (unchanged) lines, which GitLab
	// requires in text positions.
	diffs, err := fetchMRDiffLines(ctx, p.client, apiURL, projectPath, token, mrIID)
	if err != nil {
		slog.Warn("failed to fetch MR diff lines, inline comments posted unvalidated", "error", err)
		diffs = nil
	}

	// Separate issues into file-level and non-file
	var fileIssues []review.ReviewIssue
	var nonFileIssues []review.ReviewIssue

	for _, issue := range result.Issues {
		if issue.File == "" || issue.Line <= 0 {
			nonFileIssues = append(nonFileIssues, issue)
			continue
		}
		// Validate line is in MR diff hunks
		if diffs != nil && !diffs.contains(issue.File, issue.Line) {
			slog.Debug("issue line not in MR diff, moving to summary",
				"file", issue.File, "line", issue.Line)
			nonFileIssues = append(nonFileIssues, issue)
			continue
		}
		fileIssues = append(fileIssues, issue)
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
		err := p.postDiscussion(ctx, apiURL, projectPath, token, mrIID, version, diffs, issue, formatIssue)
		if err != nil {
			// Non-fatal: move to summary
			slog.Warn("failed to post MR discussion, moving issue to summary",
				"file", issue.File, "line", issue.Line, "error", err)
			nonFileIssues = append(nonFileIssues, issue)
			continue
		}
		commentsPosted++
	}

	// Post summary discussion (no position)
	discussion, err := p.postDiscussionBody(ctx, apiURL, projectPath, token, mrIID, map[string]interface{}{
		"body": formatSummary(result, nonFileIssues),
	})
	if err != nil {
		return nil, fmt.Errorf("posting summary discussion: %w", err)
	}

	return &PostReviewResult{
		ReviewURL:      mrReviewURL(repo, mrIID, discussion),
		CommentsPosted: commentsPosted,
	}, nil
}

func (p *GitLabReviewPoster) getMRVersion(ctx context.Context, apiURL, projectPath, token string, mrIID int) (*gitlabMRVersion, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/versions", apiURL, projectPath, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

// postDiscussion posts a single positioned (line-level) discussion.
func (p *GitLabReviewPoster) postDiscussion(ctx context.Context, apiURL, projectPath, token string, mrIID int, version *gitlabMRVersion, diffs gitlabDiffSet, issue review.ReviewIssue, formatIssue func(review.ReviewIssue) string) error {
	position := map[string]interface{}{
		"base_sha":      version.BaseCommitSHA,
		"start_sha":     version.StartCommitSHA,
		"head_sha":      version.HeadCommitSHA,
		"position_type": "text",
		"new_path":      issue.File,
		"new_line":      issue.Line,
	}

	// Context (unchanged) lines require old_path/old_line, otherwise GitLab
	// rejects the position with 400. Added lines send only new_path/new_line.
	if fd, ok := diffs[issue.File]; ok {
		if oldLine, ok := fd.lines[issue.Line]; ok && oldLine > 0 {
			position["old_path"] = fd.oldPath
			position["old_line"] = oldLine
		}
	}

	_, err := p.postDiscussionBody(ctx, apiURL, projectPath, token, mrIID, map[string]interface{}{
		"body":     formatIssue(issue),
		"position": position,
	})
	return err
}

// postDiscussionBody POSTs a discussion payload and parses the response.
func (p *GitLabReviewPoster) postDiscussionBody(ctx context.Context, apiURL, projectPath, token string, mrIID int, payload map[string]interface{}) (*gitlabDiscussion, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions", apiURL, projectPath, mrIID)

	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling discussion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating discussion request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab discussion API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gitlab discussion API returned %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var discussion gitlabDiscussion
	if err := json.Unmarshal(respBody, &discussion); err != nil {
		slog.Warn("failed to parse gitlab discussion response", "error", err)
		return &gitlabDiscussion{}, nil
	}
	return &discussion, nil
}

// approveMR maps the approve verdict to the MR approvals API. Best-effort:
// failures (e.g. 401/403 missing rights, 404 approvals unavailable) are
// logged and never fail the review posting.
func (p *GitLabReviewPoster) approveMR(ctx context.Context, apiURL, projectPath, token string, mrIID int) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/approve", apiURL, projectPath, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		slog.Warn("creating MR approve request failed", "error", err)
		return
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := p.client.Do(req)
	if err != nil {
		slog.Warn("gitlab MR approve request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("gitlab MR approve rejected, verdict kept as summary text only",
			"status", resp.StatusCode, "response", truncateBytes(body, 300))
	}
}

// mrReviewURL builds a web link to the posted review discussion:
// <scheme://host[:port]>/<owner>/<repo>/-/merge_requests/<iid>#note_<id>.
func mrReviewURL(repo *RepoInfo, mrIID int, discussion *gitlabDiscussion) string {
	reviewURL := fmt.Sprintf("%s/%s/-/merge_requests/%d", repo.BaseURL(), repo.FullName(), mrIID)
	if discussion != nil && len(discussion.Notes) > 0 {
		reviewURL += fmt.Sprintf("#note_%d", discussion.Notes[0].ID)
	}
	return reviewURL
}

func (p *GitLabReviewPoster) postSummaryOnly(ctx context.Context, repo *RepoInfo, apiURL, projectPath, token string, mrIID int, result *review.ReviewResult, formatSummary func(*review.ReviewResult, []review.ReviewIssue) string) (*PostReviewResult, error) {
	// All issues go into summary since we can't do position-based comments
	discussion, err := p.postDiscussionBody(ctx, apiURL, projectPath, token, mrIID, map[string]interface{}{
		"body": formatSummary(result, result.Issues),
	})
	if err != nil {
		return nil, err
	}

	return &PostReviewResult{
		ReviewURL:      mrReviewURL(repo, mrIID, discussion),
		CommentsPosted: 0,
	}, nil
}
