package main

import (
	"os"
	"strconv"
	"strings"
)

// ParseGitLabContext reads GitLab CI environment variables.
func ParseGitLabContext() (*CIContext, error) {
	ctx := &CIContext{
		Platform: PlatformGitLab,
		WorkDir:  envDefault("CI_PROJECT_DIR", "."),
		RepoURL:  os.Getenv("CI_PROJECT_URL"),
		HeadSHA:  os.Getenv("CI_COMMIT_SHA"),
	}

	// Parse project path (group/repo or group/subgroup/repo)
	projectPath := os.Getenv("CI_PROJECT_PATH")
	if projectPath != "" {
		parts := strings.SplitN(projectPath, "/", 2)
		if len(parts) == 2 {
			ctx.RepoOwner = parts[0]
			ctx.RepoName = parts[1]
		}
	}

	// MR info
	if mrIID := os.Getenv("CI_MERGE_REQUEST_IID"); mrIID != "" {
		if n, err := strconv.Atoi(mrIID); err == nil {
			ctx.PRNumber = n
		}
	}

	ctx.PRBranch = os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
	ctx.BaseBranch = envDefault("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", envDefault("CI_DEFAULT_BRANCH", "main"))

	if sha := os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"); sha != "" {
		// This is the merge base, prefer the commit SHA for head
		ctx.HeadSHA = envDefault("CI_COMMIT_SHA", sha)
	}

	return ctx, nil
}
