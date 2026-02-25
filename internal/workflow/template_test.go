package workflow

import (
	"strings"
	"testing"
)

func TestRender_HappyPath(t *testing.T) {
	ctx := TemplateContext{
		Params: map[string]string{"repo_url": "https://github.com/owner/repo"},
		Steps: map[string]map[string]string{
			"fetch_issue": {"title": "Fix bug", "number": "42"},
		},
	}

	result, err := Render("Fix: {{.Steps.fetch_issue.title}} (#{{.Steps.fetch_issue.number}}) in {{.Params.repo_url}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Fix: Fix bug (#42) in https://github.com/owner/repo" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestRender_NoTemplate(t *testing.T) {
	result, err := Render("plain string", TemplateContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain string" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestRender_MissingKey(t *testing.T) {
	ctx := TemplateContext{
		Steps: map[string]map[string]string{},
	}
	_, err := Render("{{.Steps.nonexistent.title}}", ctx)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRender_RepoPathHelper(t *testing.T) {
	ctx := TemplateContext{
		Params: map[string]string{"repo_url": "https://github.com/owner/repo.git"},
	}
	result, err := Render(`{{repoPath .Params.repo_url}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "owner/repo" {
		t.Fatalf("expected owner/repo, got: %s", result)
	}
}

func TestRender_OutputLimit(t *testing.T) {
	// Create a template that produces >1MB output
	bigValue := strings.Repeat("x", maxTemplateOutput+1)
	ctx := TemplateContext{
		Params: map[string]string{"big": bigValue},
	}
	_, err := Render("{{.Params.big}}", ctx)
	if err == nil {
		t.Fatal("expected error for output exceeding 1MB")
	}
	if !strings.Contains(err.Error(), "1MB") {
		t.Fatalf("expected 1MB error, got: %v", err)
	}
}

func TestRepoPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://gitlab.com/group/project.git", "group/project"},
		{"git@github.com:owner/repo.git", "owner/repo"},
	}

	for _, tc := range tests {
		got := repoPath(tc.input)
		if got != tc.want {
			t.Errorf("repoPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
