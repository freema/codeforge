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

// UpdateMR updates an existing merge request's description on GitLab.
func (c *GitLabMRCreator) UpdateMR(ctx context.Context, repo *RepoInfo, token string, mrIID int, description string) error {
	apiURL := repo.APIURL()
	projectPath := url.PathEscape(repo.FullName())
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", apiURL, projectPath, mrIID)

	bodyJSON, err := json.Marshal(map[string]string{"description": description})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab PUT MR returned %d", resp.StatusCode)
	}
	return nil
}
