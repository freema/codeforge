package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func ListRepos(ctx context.Context, provider Provider, token string, page, perPage int) ([]Repository, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	switch provider {
	case ProviderGitHub:
		return listGitHubRepos(ctx, token, page, perPage)
	case ProviderGitLab:
		return listGitLabRepos(ctx, token, page, perPage)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func listGitHubRepos(ctx context.Context, token string, page, perPage int) ([]Repository, error) {
	url := fmt.Sprintf("https://api.github.com/user/repos?per_page=%d&page=%d&sort=updated&type=all", perPage, page)

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

func listGitLabRepos(ctx context.Context, token string, page, perPage int) ([]Repository, error) {
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects?membership=true&per_page=%d&page=%d&order_by=last_activity_at", perPage, page)

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
