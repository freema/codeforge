package main

import (
	"testing"
)

func TestParseGitLabContext_MREvent(t *testing.T) {
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PROJECT_DIR", "/builds/group/repo")
	t.Setenv("CI_PROJECT_URL", "https://gitlab.com/group/repo")
	t.Setenv("CI_PROJECT_PATH", "group/repo")
	t.Setenv("CI_COMMIT_SHA", "gitlab-sha-123")
	t.Setenv("CI_MERGE_REQUEST_IID", "55")
	t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "feature/mr-branch")
	t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "main")
	t.Setenv("CI_MERGE_REQUEST_DIFF_BASE_SHA", "")
	t.Setenv("CI_DEFAULT_BRANCH", "main")

	ctx, err := ParseGitLabContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Platform != PlatformGitLab {
		t.Errorf("Platform = %q, want %q", ctx.Platform, PlatformGitLab)
	}
	if ctx.WorkDir != "/builds/group/repo" {
		t.Errorf("WorkDir = %q, want %q", ctx.WorkDir, "/builds/group/repo")
	}
	if ctx.RepoURL != "https://gitlab.com/group/repo" {
		t.Errorf("RepoURL = %q, want %q", ctx.RepoURL, "https://gitlab.com/group/repo")
	}
	if ctx.RepoOwner != "group" {
		t.Errorf("RepoOwner = %q, want %q", ctx.RepoOwner, "group")
	}
	if ctx.RepoName != "repo" {
		t.Errorf("RepoName = %q, want %q", ctx.RepoName, "repo")
	}
	if ctx.PRNumber != 55 {
		t.Errorf("PRNumber = %d, want %d", ctx.PRNumber, 55)
	}
	if ctx.PRBranch != "feature/mr-branch" {
		t.Errorf("PRBranch = %q, want %q", ctx.PRBranch, "feature/mr-branch")
	}
	if ctx.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "main")
	}
	if ctx.HeadSHA != "gitlab-sha-123" {
		t.Errorf("HeadSHA = %q, want %q", ctx.HeadSHA, "gitlab-sha-123")
	}
}

func TestParseGitLabContext_PipelineEvent(t *testing.T) {
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PROJECT_DIR", "/builds/group/repo")
	t.Setenv("CI_PROJECT_URL", "https://gitlab.com/group/repo")
	t.Setenv("CI_PROJECT_PATH", "group/repo")
	t.Setenv("CI_COMMIT_SHA", "pipeline-sha")
	t.Setenv("CI_MERGE_REQUEST_IID", "")
	t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
	t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "")
	t.Setenv("CI_MERGE_REQUEST_DIFF_BASE_SHA", "")
	t.Setenv("CI_DEFAULT_BRANCH", "develop")
	t.Setenv("INPUT_MR_IID", "")
	t.Setenv("INPUT_PR_NUMBER", "")

	ctx, err := ParseGitLabContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.PRNumber != 0 {
		t.Errorf("PRNumber = %d, want 0", ctx.PRNumber)
	}
	if ctx.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "develop")
	}
}

func TestParseGitLabContext_ManualMROverride(t *testing.T) {
	tests := []struct {
		name       string
		ciMRIID    string
		inputMRIID string
		inputPRNum string
		want       int
	}{
		{
			name:       "INPUT_MR_IID targets a specific MR",
			inputMRIID: "77",
			want:       77,
		},
		{
			name:       "INPUT_PR_NUMBER works as alias",
			inputPRNum: "88",
			want:       88,
		},
		{
			name:       "INPUT_MR_IID takes precedence over alias",
			inputMRIID: "77",
			inputPRNum: "88",
			want:       77,
		},
		{
			name:       "CI_MERGE_REQUEST_IID takes precedence over overrides",
			ciMRIID:    "55",
			inputMRIID: "77",
			want:       55,
		},
		{
			name:       "invalid override is ignored",
			inputMRIID: "not-a-number",
			want:       0,
		},
		{
			name:       "zero override is ignored",
			inputMRIID: "0",
			want:       0,
		},
		{
			name: "no MR context and no override",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_CI", "true")
			t.Setenv("CI_PROJECT_DIR", "/builds/group/repo")
			t.Setenv("CI_PROJECT_URL", "https://gitlab.example.com/group/repo")
			t.Setenv("CI_PROJECT_PATH", "group/repo")
			t.Setenv("CI_COMMIT_SHA", "sha")
			t.Setenv("CI_MERGE_REQUEST_IID", tt.ciMRIID)
			t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
			t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "")
			t.Setenv("CI_MERGE_REQUEST_DIFF_BASE_SHA", "")
			t.Setenv("CI_DEFAULT_BRANCH", "main")
			t.Setenv("INPUT_MR_IID", tt.inputMRIID)
			t.Setenv("INPUT_PR_NUMBER", tt.inputPRNum)

			ctx, err := ParseGitLabContext()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ctx.PRNumber != tt.want {
				t.Errorf("PRNumber = %d, want %d", ctx.PRNumber, tt.want)
			}
		})
	}
}

func TestParseGitLabContext_Subgroup(t *testing.T) {
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PROJECT_DIR", "/builds/group/subgroup/repo")
	t.Setenv("CI_PROJECT_URL", "https://gitlab.com/group/subgroup/repo")
	t.Setenv("CI_PROJECT_PATH", "group/subgroup/repo")
	t.Setenv("CI_COMMIT_SHA", "sha")
	t.Setenv("CI_MERGE_REQUEST_IID", "")
	t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "")
	t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "")
	t.Setenv("CI_MERGE_REQUEST_DIFF_BASE_SHA", "")
	t.Setenv("CI_DEFAULT_BRANCH", "main")

	ctx, err := ParseGitLabContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With SplitN(path, "/", 2), owner="group", name="subgroup/repo"
	if ctx.RepoOwner != "group" {
		t.Errorf("RepoOwner = %q, want %q", ctx.RepoOwner, "group")
	}
	if ctx.RepoName != "subgroup/repo" {
		t.Errorf("RepoName = %q, want %q", ctx.RepoName, "subgroup/repo")
	}
}
