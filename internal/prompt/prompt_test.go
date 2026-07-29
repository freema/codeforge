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

func TestRenderTaskPrompt_Plan(t *testing.T) {
	result, err := RenderTaskPrompt("plan", "Add user authentication")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Add user authentication") {
		t.Error("result should contain the user prompt")
	}
	for _, want := range []string{
		"software architect",
		"Do NOT modify any files",
		"implementation plan",
		"## Output Format",
		"## Summary",
		"## Files to Change",
		"## Approach",
		"## Risks",
		"## Complexity",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("result should contain %q", want)
		}
	}
}

func TestRenderTaskPrompt_Knowledge(t *testing.T) {
	tests := []struct {
		name      string
		focus     string
		wantFocus bool
	}{
		{"with focus", "authentication and session handling", true},
		{"without focus", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTaskPrompt("knowledge", tt.focus)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if strings.Contains(result, "{{") {
				t.Error("result contains unrendered template syntax")
			}

			if tt.wantFocus {
				if !strings.Contains(result, "Focus area: "+tt.focus) {
					t.Error("result should contain the focus area")
				}
			} else if strings.Contains(result, "Focus area:") {
				t.Error("result should not contain a focus area line when focus is empty")
			}

			for _, want := range []string{
				".codeforge/OVERVIEW.md",
				".codeforge/ARCHITECTURE.md",
				".codeforge/CONVENTIONS.md",
				"do NOT modify anything outside",
				"UPDATE them",
				"STABLE knowledge",
				"reference them rather than duplicating",
			} {
				if !strings.Contains(result, want) {
					t.Errorf("result should contain %q", want)
				}
			}
		})
	}
}

func TestReviewTemplates_IdenticalSchema(t *testing.T) {
	schema, err := LoadRaw("review_schema")
	if err != nil {
		t.Fatalf("LoadRaw review_schema: %v", err)
	}
	schema = strings.TrimRight(schema, "\n")
	if !strings.Contains(schema, `"auto_fixable"`) {
		t.Fatal("review_schema should contain auto_fixable")
	}

	renders := map[string]func() (string, error){
		"review": func() (string, error) {
			return Render("review", SessionTypeData{UserPrompt: "focus"})
		},
		"pr_review": func() (string, error) {
			return Render("pr_review", PRReviewData{PRNumber: 1, PRBranch: "feature", BaseBranch: "main"})
		},
		"code_review": func() (string, error) {
			return Render("code_review", CodeReviewData{OriginalPrompt: "task"})
		},
	}

	for name, render := range renders {
		t.Run(name, func(t *testing.T) {
			result, err := render()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result, schema) {
				t.Error("rendered template should contain the shared review schema block")
			}
			if !strings.Contains(result, `"auto_fixable"`) {
				t.Error("rendered template should contain auto_fixable")
			}
			if strings.Contains(result, "{{") {
				t.Error("result contains unrendered template syntax")
			}
		})
	}
}

func TestRenderTaskPrompt_Review(t *testing.T) {
	result, err := RenderTaskPrompt("review", "Review the auth module")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Review the auth module") {
		t.Error("result should contain the user prompt")
	}
	for _, want := range []string{
		"senior code reviewer",
		"Do NOT modify any files",
		`"verdict"`,
		`"score"`,
	} {
		if !strings.Contains(result, want) {
			t.Errorf("result should contain %q", want)
		}
	}
}

func TestRenderTaskPrompt_Code(t *testing.T) {
	result, err := RenderTaskPrompt("code", "Fix the login bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Fix the login bug" {
		t.Errorf("code type should return raw prompt, got: %s", result)
	}
}

func TestRenderTaskPrompt_Empty(t *testing.T) {
	result, err := RenderTaskPrompt("", "Fix the login bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Fix the login bug" {
		t.Errorf("empty type should return raw prompt, got: %s", result)
	}
}

func TestValidSessionType(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"code", true},
		{"plan", true},
		{"review", true},
		{"pr_review", true},
		{"knowledge", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidSessionType(tt.name); got != tt.valid {
			t.Errorf("ValidSessionType(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestSessionTypes(t *testing.T) {
	types := SessionTypes()
	if len(types) != 5 {
		t.Fatalf("expected 5 session types, got %d", len(types))
	}

	names := map[string]bool{}
	for _, tt := range types {
		names[tt.Name] = true
		if tt.Label == "" {
			t.Errorf("session type %s has empty label", tt.Name)
		}
		if tt.Description == "" {
			t.Errorf("session type %s has empty description", tt.Name)
		}
	}

	for _, expected := range []string{"code", "plan", "review", "pr_review", "knowledge"} {
		if !names[expected] {
			t.Errorf("expected session type %s in list", expected)
		}
	}
}

func TestSessionTypeTemplate(t *testing.T) {
	if tmpl := SessionTypeTemplate("code"); tmpl != "" {
		t.Errorf("code should have no template, got %q", tmpl)
	}
	if tmpl := SessionTypeTemplate("plan"); tmpl != "plan" {
		t.Errorf("plan template should be 'plan', got %q", tmpl)
	}
	if tmpl := SessionTypeTemplate("review"); tmpl != "review" {
		t.Errorf("review template should be 'review', got %q", tmpl)
	}
	if tmpl := SessionTypeTemplate("knowledge"); tmpl != "knowledge" {
		t.Errorf("knowledge template should be 'knowledge', got %q", tmpl)
	}
	if tmpl := SessionTypeTemplate("unknown"); tmpl != "" {
		t.Errorf("unknown type should have no template, got %q", tmpl)
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
