package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// Repository represents a git repository from a provider.
type Repository struct {
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	CloneURL      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	Description   string    `json:"description"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ListRepos lists repositories from a provider using the given token.
// baseURL is optional — when set, it overrides the default API URL (for self-hosted instances).
func ListRepos(ctx context.Context, provider Provider, token, baseURL string, page, perPage int) ([]Repository, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	switch provider {
	case ProviderGitHub:
		return listGitHubRepos(ctx, token, baseURL, page, perPage)
	case ProviderGitLab:
		return listGitLabRepos(ctx, token, baseURL, page, perPage)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func listGitHubRepos(ctx context.Context, token, baseURL string, page, perPage int) ([]Repository, error) {
	apiBase := "https://api.github.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	url := fmt.Sprintf("%s/user/repos?per_page=%d&page=%d&sort=updated&type=all", apiBase, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var ghRepos []struct {
		Name          string    `json:"name"`
		FullName      string    `json:"full_name"`
		CloneURL      string    `json:"clone_url"`
		DefaultBranch string    `json:"default_branch"`
		Private       bool      `json:"private"`
		Description   *string   `json:"description"`
		UpdatedAt     time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &ghRepos); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	repos := make([]Repository, 0, len(ghRepos))
	for _, r := range ghRepos {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		repos = append(repos, Repository{
			Name:          r.Name,
			FullName:      r.FullName,
			CloneURL:      r.CloneURL,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			Description:   desc,
			UpdatedAt:     r.UpdatedAt,
		})
	}

	return repos, nil
}

// Branch represents a git branch from a provider.
type Branch struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// ListBranches lists branches for a repository from a provider using the given token.
func ListBranches(ctx context.Context, provider Provider, token, baseURL string, repoFullName string) ([]Branch, error) {
	switch provider {
	case ProviderGitHub:
		return listGitHubBranches(ctx, token, baseURL, repoFullName)
	case ProviderGitLab:
		return listGitLabBranches(ctx, token, baseURL, repoFullName)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func listGitHubBranches(ctx context.Context, token, baseURL string, repoFullName string) ([]Branch, error) {
	apiBase := "https://api.github.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	url := fmt.Sprintf("%s/repos/%s/branches?per_page=100", apiBase, repoFullName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var ghBranches []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &ghBranches); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	branches := make([]Branch, 0, len(ghBranches))
	for _, b := range ghBranches {
		branches = append(branches, Branch{Name: b.Name})
	}
	return branches, nil
}

func listGitLabBranches(ctx context.Context, token, baseURL string, repoFullName string) ([]Branch, error) {
	apiBase := "https://gitlab.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/")
	}
	// GitLab uses URL-encoded project path (e.g. "user/repo" -> "user%2Frepo")
	encoded := neturl.PathEscape(repoFullName)
	url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=100", apiBase, encoded)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var glBranches []struct {
		Name    string `json:"name"`
		Default bool   `json:"default"`
	}
	if err := json.Unmarshal(body, &glBranches); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	branches := make([]Branch, 0, len(glBranches))
	for _, b := range glBranches {
		branches = append(branches, Branch{Name: b.Name, Default: b.Default})
	}
	return branches, nil
}

// PullRequest represents an open pull request / merge request from a provider.
type PullRequest struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	State        string    `json:"state"`
	Author       string    `json:"author"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListPullRequests lists open pull requests / merge requests for a repository.
func ListPullRequests(ctx context.Context, provider Provider, token, baseURL, repoFullName string) ([]PullRequest, error) {
	switch provider {
	case ProviderGitHub:
		return listGitHubPullRequests(ctx, token, baseURL, repoFullName)
	case ProviderGitLab:
		return listGitLabMergeRequests(ctx, token, baseURL, repoFullName)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func listGitHubPullRequests(ctx context.Context, token, baseURL, repoFullName string) ([]PullRequest, error) {
	apiBase := "https://api.github.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/") + "/api/v3"
	}
	url := fmt.Sprintf("%s/repos/%s/pulls?state=open&per_page=50&sort=updated&direction=desc", apiBase, repoFullName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var ghPRs []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		State     string    `json:"state"`
		User      struct{ Login string } `json:"user"`
		Head      struct{ Ref string } `json:"head"`
		Base      struct{ Ref string } `json:"base"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &ghPRs); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	prs := make([]PullRequest, 0, len(ghPRs))
	for _, p := range ghPRs {
		prs = append(prs, PullRequest{
			Number:       p.Number,
			Title:        p.Title,
			State:        p.State,
			Author:       p.User.Login,
			SourceBranch: p.Head.Ref,
			TargetBranch: p.Base.Ref,
			UpdatedAt:    p.UpdatedAt,
		})
	}
	return prs, nil
}

func listGitLabMergeRequests(ctx context.Context, token, baseURL, repoFullName string) ([]PullRequest, error) {
	apiBase := "https://gitlab.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/")
	}
	encoded := neturl.PathEscape(repoFullName)
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=opened&per_page=50&order_by=updated_at&sort=desc", apiBase, encoded)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var glMRs []struct {
		IID          int       `json:"iid"`
		Title        string    `json:"title"`
		State        string    `json:"state"`
		Author       struct{ Username string } `json:"author"`
		SourceBranch string    `json:"source_branch"`
		TargetBranch string    `json:"target_branch"`
		UpdatedAt    time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &glMRs); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	prs := make([]PullRequest, 0, len(glMRs))
	for _, m := range glMRs {
		prs = append(prs, PullRequest{
			Number:       m.IID,
			Title:        m.Title,
			State:        m.State,
			Author:       m.Author.Username,
			SourceBranch: m.SourceBranch,
			TargetBranch: m.TargetBranch,
			UpdatedAt:    m.UpdatedAt,
		})
	}
	return prs, nil
}

func listGitLabRepos(ctx context.Context, token, baseURL string, page, perPage int) ([]Repository, error) {
	apiBase := "https://gitlab.com"
	if baseURL != "" {
		apiBase = strings.TrimRight(baseURL, "/")
	}
	url := fmt.Sprintf("%s/api/v4/projects?membership=true&per_page=%d&page=%d&order_by=last_activity_at", apiBase, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
	}

	var glProjects []struct {
		Name              string    `json:"name"`
		PathWithNamespace string    `json:"path_with_namespace"`
		HTTPURLToRepo     string    `json:"http_url_to_repo"`
		DefaultBranch     string    `json:"default_branch"`
		Visibility        string    `json:"visibility"`
		Description       string    `json:"description"`
		LastActivityAt    time.Time `json:"last_activity_at"`
	}
	if err := json.Unmarshal(body, &glProjects); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	repos := make([]Repository, 0, len(glProjects))
	for _, p := range glProjects {
		repos = append(repos, Repository{
			Name:          p.Name,
			FullName:      p.PathWithNamespace,
			CloneURL:      p.HTTPURLToRepo,
			DefaultBranch: p.DefaultBranch,
			Private:       p.Visibility != "public",
			Description:   p.Description,
			UpdatedAt:     p.LastActivityAt,
		})
	}

	return repos, nil
}
