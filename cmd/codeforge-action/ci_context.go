package main

import "os"

// CIPlatform identifies the CI environment.
type CIPlatform string

const (
	PlatformGitHub  CIPlatform = "github"
	PlatformGitLab  CIPlatform = "gitlab"
	PlatformUnknown CIPlatform = "unknown"
)

// CIContext holds information extracted from the CI environment.
type CIContext struct {
	Platform   CIPlatform
	RepoURL    string // e.g., https://github.com/owner/repo
	PRNumber   int    // PR/MR number (0 if not a PR event)
	PRBranch   string // source branch name
	BaseBranch string // target branch name
	HeadSHA    string // commit SHA being reviewed
	WorkDir    string // workspace directory (already checked out)
	RepoOwner  string // owner/org
	RepoName   string // repository name
}

// DetectPlatform determines the CI platform from environment variables.
func DetectPlatform() CIPlatform {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return PlatformGitHub
	}
	if os.Getenv("GITLAB_CI") == "true" {
		return PlatformGitLab
	}
	return PlatformUnknown
}

// DetectCIContext builds a CIContext from the current environment.
func DetectCIContext() (*CIContext, error) {
	platform := DetectPlatform()

	switch platform {
	case PlatformGitHub:
		return ParseGitHubContext()
	case PlatformGitLab:
		return ParseGitLabContext()
	default:
		// Fallback: try to detect from environment
		return &CIContext{
			Platform: PlatformUnknown,
			WorkDir:  envDefault("WORKSPACE", "."),
		}, nil
	}
}
