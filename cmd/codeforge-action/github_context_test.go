package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubContext_PREvent(t *testing.T) {
	// Create temp event file
	eventJSON := `{
		"pull_request": {
			"number": 42,
			"title": "Fix bug",
			"head": {
				"ref": "feature/fix-bug",
				"sha": "abc123def456"
			},
			"base": {
				"ref": "main"
			}
		},
		"repository": {
			"full_name": "freema/codeforge",
			"clone_url": "https://github.com/freema/codeforge.git",
			"html_url": "https://github.com/freema/codeforge"
		}
	}`

	tmpDir := t.TempDir()
	eventPath := filepath.Join(tmpDir, "event.json")
	if err := os.WriteFile(eventPath, []byte(eventJSON), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	t.Setenv("GITHUB_WORKSPACE", "/workspace")
	t.Setenv("GITHUB_REPOSITORY", "freema/codeforge")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_SHA", "original-sha")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_HEAD_REF", "")

	ctx, err := ParseGitHubContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Platform != PlatformGitHub {
		t.Errorf("Platform = %q, want %q", ctx.Platform, PlatformGitHub)
	}
	if ctx.WorkDir != "/workspace" {
		t.Errorf("WorkDir = %q, want %q", ctx.WorkDir, "/workspace")
	}
	if ctx.PRNumber != 42 {
		t.Errorf("PRNumber = %d, want %d", ctx.PRNumber, 42)
	}
	if ctx.PRBranch != "feature/fix-bug" {
		t.Errorf("PRBranch = %q, want %q", ctx.PRBranch, "feature/fix-bug")
	}
	if ctx.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "main")
	}
	if ctx.HeadSHA != "abc123def456" {
		t.Errorf("HeadSHA = %q, want %q", ctx.HeadSHA, "abc123def456")
	}
	if ctx.RepoURL != "https://github.com/freema/codeforge" {
		t.Errorf("RepoURL = %q, want %q", ctx.RepoURL, "https://github.com/freema/codeforge")
	}
	if ctx.RepoOwner != "freema" {
		t.Errorf("RepoOwner = %q, want %q", ctx.RepoOwner, "freema")
	}
	if ctx.RepoName != "codeforge" {
		t.Errorf("RepoName = %q, want %q", ctx.RepoName, "codeforge")
	}
}

func TestParseGitHubContext_PushEvent(t *testing.T) {
	// Push event — no PR
	eventJSON := `{
		"repository": {
			"full_name": "freema/codeforge",
			"html_url": "https://github.com/freema/codeforge"
		}
	}`

	tmpDir := t.TempDir()
	eventPath := filepath.Join(tmpDir, "event.json")
	if err := os.WriteFile(eventPath, []byte(eventJSON), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	t.Setenv("GITHUB_WORKSPACE", "/workspace")
	t.Setenv("GITHUB_REPOSITORY", "freema/codeforge")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_SHA", "push-sha-123")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_HEAD_REF", "")

	ctx, err := ParseGitHubContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.PRNumber != 0 {
		t.Errorf("PRNumber = %d, want 0", ctx.PRNumber)
	}
	if ctx.HeadSHA != "push-sha-123" {
		t.Errorf("HeadSHA = %q, want %q", ctx.HeadSHA, "push-sha-123")
	}
	if ctx.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "main")
	}
}

func TestParseGitHubContext_NoEventFile(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_WORKSPACE", "/workspace")
	t.Setenv("GITHUB_REPOSITORY", "freema/codeforge")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_SHA", "sha-123")
	t.Setenv("GITHUB_BASE_REF", "develop")
	t.Setenv("GITHUB_HEAD_REF", "feature/test")

	ctx, err := ParseGitHubContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", ctx.BaseBranch, "develop")
	}
	if ctx.PRBranch != "feature/test" {
		t.Errorf("PRBranch = %q, want %q", ctx.PRBranch, "feature/test")
	}
}

func TestParseGitHubContext_WorkflowDispatch(t *testing.T) {
	// workflow_dispatch event — PR number in inputs, not in pull_request
	eventJSON := `{
		"inputs": {
			"pr_number": "5",
			"cli": "claude-code"
		},
		"repository": {
			"full_name": "owner/repo",
			"html_url": "https://github.com/owner/repo"
		}
	}`

	tmpDir := t.TempDir()
	eventPath := filepath.Join(tmpDir, "event.json")
	if err := os.WriteFile(eventPath, []byte(eventJSON), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	t.Setenv("GITHUB_WORKSPACE", "/workspace")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_SHA", "dispatch-sha")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_HEAD_REF", "")

	ctx, err := ParseGitHubContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.PRNumber != 5 {
		t.Errorf("PRNumber = %d, want 5", ctx.PRNumber)
	}
	if ctx.RepoURL != "https://github.com/owner/repo" {
		t.Errorf("RepoURL = %q, want %q", ctx.RepoURL, "https://github.com/owner/repo")
	}
}

func TestParseGitHubContext_GHEServer(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_WORKSPACE", "/workspace")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_SERVER_URL", "https://github.example.com")
	t.Setenv("GITHUB_SHA", "sha-456")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_HEAD_REF", "")

	ctx, err := ParseGitHubContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.RepoURL != "https://github.example.com/org/repo" {
		t.Errorf("RepoURL = %q, want %q", ctx.RepoURL, "https://github.example.com/org/repo")
	}
}

func TestParseGitHubEventFile(t *testing.T) {
	t.Run("valid PR event", func(t *testing.T) {
		eventJSON := `{"pull_request":{"number":1,"head":{"ref":"b","sha":"c"},"base":{"ref":"main"}}}`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "event.json")
		if err := os.WriteFile(path, []byte(eventJSON), 0644); err != nil {
			t.Fatal(err)
		}

		event, err := parseGitHubEventFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.PullRequest == nil {
			t.Fatal("expected PullRequest to be non-nil")
		}
		if event.PullRequest.Number != 1 {
			t.Errorf("Number = %d, want 1", event.PullRequest.Number)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "bad.json")
		if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := parseGitHubEventFile(path)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := parseGitHubEventFile("/nonexistent/path/event.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}
