package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// githubEvent represents the relevant fields from the GitHub event payload.
type githubEvent struct {
	PullRequest *githubPR `json:"pull_request"`
	Repository  struct {
		FullName string `json:"full_name"` // owner/repo
		CloneURL string `json:"clone_url"` // https://github.com/owner/repo.git
		HTMLURL  string `json:"html_url"`  // https://github.com/owner/repo
	} `json:"repository"`
}

type githubPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Head   struct {
		Ref string `json:"ref"` // source branch
		SHA string `json:"sha"` // head commit
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"` // target branch
	} `json:"base"`
}

// ParseGitHubContext reads the GitHub Actions event payload and environment.
func ParseGitHubContext() (*CIContext, error) {
	ctx := &CIContext{
		Platform: PlatformGitHub,
		WorkDir:  envDefault("GITHUB_WORKSPACE", "."),
	}

	// Parse GITHUB_REPOSITORY (owner/repo)
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo != "" {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			ctx.RepoOwner = parts[0]
			ctx.RepoName = parts[1]
		}
	}

	serverURL := envDefault("GITHUB_SERVER_URL", "https://github.com")
	if repo != "" {
		ctx.RepoURL = serverURL + "/" + repo
	}

	ctx.HeadSHA = os.Getenv("GITHUB_SHA")

	// Parse event payload for PR info
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath != "" {
		event, err := parseGitHubEventFile(eventPath)
		if err != nil {
			return ctx, fmt.Errorf("parsing github event: %w", err)
		}

		if event.PullRequest != nil {
			ctx.PRNumber = event.PullRequest.Number
			ctx.PRBranch = event.PullRequest.Head.Ref
			ctx.BaseBranch = event.PullRequest.Base.Ref
			ctx.HeadSHA = event.PullRequest.Head.SHA
		}

		if event.Repository.HTMLURL != "" {
			ctx.RepoURL = event.Repository.HTMLURL
		}
	}

	// Fallback branch detection
	if ctx.BaseBranch == "" {
		ctx.BaseBranch = envDefault("GITHUB_BASE_REF", "main")
	}
	if ctx.PRBranch == "" {
		ctx.PRBranch = os.Getenv("GITHUB_HEAD_REF")
	}

	return ctx, nil
}

func parseGitHubEventFile(path string) (*githubEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading event file: %w", err)
	}

	var event githubEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("unmarshaling event: %w", err)
	}

	return &event, nil
}
