package prompt

import (
	"strings"
	"testing"
)

func TestRender_CodeReview(t *testing.T) {
	data := CodeReviewData{
		OriginalPrompt: "Fix the login bug in auth.go",
	}

	result, err := Render("code_review", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain the rendered original prompt
	if !strings.Contains(result, "Fix the login bug in auth.go") {
		t.Error("result should contain the original prompt")
	}

	// Should contain key review instructions
	for _, want := range []string{
		"git diff HEAD~1",
		"Correctness",
		"Security",
		"Performance",
		`"verdict"`,
		`"approve"`,
		`"request_changes"`,
		"Do NOT modify any files",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("result should contain %q", want)
		}
	}
}

func TestRender_NotFound(t *testing.T) {
	_, err := Render("nonexistent_template", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestRender_EmptyData(t *testing.T) {
	result, err := Render("code_review", CodeReviewData{})
	if err != nil {
		t.Fatalf("unexpected error with zero-value struct: %v", err)
	}
	if result == "" {
		t.Error("result should not be empty")
	}
	// The template variable should render as empty string
	if !strings.Contains(result, "You are a code reviewer") {
		t.Error("result should contain template content")
	}
}
