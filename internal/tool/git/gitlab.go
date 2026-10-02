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
)

// GitLabMRCreator creates merge requests via the GitLab REST API.
type GitLabMRCreator struct {
	client *http.Client
}

// NewGitLabMRCreator creates a GitLab MR creator.
func NewGitLabMRCreator() *GitLabMRCreator {
	return &GitLabMRCreator{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// CreateMR creates a merge request on GitLab.
func (c *GitLabMRCreator) CreateMR(ctx context.Context, repo *RepoInfo, token string, opts PRCreateOptions) (*PRResult, error) {
	apiURL := repo.APIURL()
	projectPath := url.PathEscape(repo.FullName())
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", apiURL, projectPath)

	body := map[string]interface{}{
		"title":         opts.Title,
		"description":   opts.Description,
		"source_branch": opts.Branch,
		"target_branch": opts.BaseBranch,
		"labels":        "codeforge",
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling MR request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating MR request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading gitlab response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gitlab API returned %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var result struct {
		WebURL string `json:"web_url"`
		IID    int    `json:"iid"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing gitlab MR response: %w", err)
	}

	return &PRResult{
		URL:    result.WebURL,
		Number: result.IID,
	}, nil
}

// GetMRStatus fetches the current status of a merge request.
func (c *GitLabMRCreator) GetMRStatus(ctx context.Context, repo *RepoInfo, token string, mrIID int) (*PRStatus, error) {
	apiURL := repo.APIURL()
	projectPath := url.PathEscape(repo.FullName())
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", apiURL, projectPath, mrIID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var mr struct {
		State     string `json:"state"`
		Title     string `json:"title"`
		MergeUser *struct {
			Username string `json:"username"`
		} `json:"merge_user"`
		// merged_by is deprecated; modern GitLab returns null here and
		// populates merge_user instead. Kept as fallback for old instances.
		MergedBy *struct {
			Username string `json:"username"`
		} `json:"merged_by"`
	}
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// GitLab states: "opened", "closed", "merged", "locked".
	// "locked" is a transient state during merge — surface it as "open",
	// consumers only understand open/merged/closed.
	state := mr.State
	if state == "opened" || state == "locked" {
		state = "open"
	}

	status := &PRStatus{
		State:  state,
		Title:  mr.Title,
		Merged: state == "merged",
	}
	switch {
	case mr.MergeUser != nil:
		status.MergedBy = mr.MergeUser.Username
	case mr.MergedBy != nil:
		status.MergedBy = mr.MergedBy.Username
	}
	return status, nil
}

// GitLabDeveloperAccess is GitLab's Developer role, the lowest access level
// that can push to a project.
const GitLabDeveloperAccess = 30

// GitLabAccessLevel returns userID's effective access level on a GitLab
// project, including membership inherited from groups, or 0 when the user is
// not an active member. baseURL is the instance's "scheme://host[:port]".
//
// Redirects are refused so the token is never forwarded to another host.
func GitLabAccessLevel(ctx context.Context, baseURL, token string, projectID, userID int) (int, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%d/members/all/%d", baseURL, projectID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("creating member request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("reading gitlab response: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return 0, nil
	default:
		return 0, fmt.Errorf("gitlab API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var member struct {
		AccessLevel int    `json:"access_level"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(body, &member); err != nil {
		return 0, fmt.Errorf("parsing gitlab member response: %w", err)
	}
	if member.State != "" && member.State != "active" {
		return 0, nil
	}
	return member.AccessLevel, nil
}
