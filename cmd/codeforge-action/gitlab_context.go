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

	// Manual override for pipelines triggered outside an MR context
	// (e.g. web/trigger pipelines with variables). INPUT_MR_IID is the
	// GitLab-native name; INPUT_PR_NUMBER is accepted as a cross-platform
	// alias matching the GitHub pr_number input.
	if ctx.PRNumber == 0 {
		for _, key := range []string{"INPUT_MR_IID", "INPUT_PR_NUMBER"} {
			if s := os.Getenv(key); s != "" {
				if n, err := strconv.Atoi(s); err == nil && n > 0 {
					ctx.PRNumber = n
					break
				}
			}
		}
	}

	ctx.PRBranch = os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
	ctx.BaseBranch = envDefault("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", envDefault("CI_DEFAULT_BRANCH", defaultBranch))

	if sha := os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"); sha != "" {
		// This is the merge base, prefer the commit SHA for head
		ctx.HeadSHA = envDefault("CI_COMMIT_SHA", sha)
	}

	return ctx, nil
}
