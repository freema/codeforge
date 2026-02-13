package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PRResult holds the result of a PR/MR creation.
type PRResult struct {
	URL    string
	Number int
}

// GitHubPRCreator creates pull requests via the GitHub REST API.
type GitHubPRCreator struct {
	client *http.Client
}

// NewGitHubPRCreator creates a GitHub PR creator.
func NewGitHubPRCreator() *GitHubPRCreator {
	return &GitHubPRCreator{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// CreatePR creates a pull request on GitHub.
func (c *GitHubPRCreator) CreatePR(ctx context.Context, repo *RepoInfo, token string, opts PRCreateOptions) (*PRResult, error) {
	apiURL := repo.APIURL()
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", apiURL, repo.Owner, repo.Repo)

	body := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Description,
		"head":  opts.Branch,
		"base":  opts.BaseBranch,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling PR request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating PR request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading github response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing github PR response: %w", err)
	}

	// Try to add label (best effort)
	c.addLabel(ctx, repo, token, result.Number)

	return &PRResult{
		URL:    result.HTMLURL,
		Number: result.Number,
	}, nil
}

func (c *GitHubPRCreator) addLabel(ctx context.Context, repo *RepoInfo, token string, prNumber int) {
	apiURL := repo.APIURL()
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels", apiURL, repo.Owner, repo.Repo, prNumber)

	body, _ := json.Marshal(map[string]interface{}{
		"labels": []string{"codeforge"},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// UpdatePR updates an existing pull request's body on GitHub.
func (c *GitHubPRCreator) UpdatePR(ctx context.Context, repo *RepoInfo, token string, prNumber int, body string) error {
	apiURL := repo.APIURL()
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", apiURL, repo.Owner, repo.Repo, prNumber)

	bodyJSON, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github PATCH PR returned %d", resp.StatusCode)
	}
	return nil
}

func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
