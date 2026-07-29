package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/*.md
var templateFS embed.FS

// CodeReviewData holds template variables for the code_review prompt.
type CodeReviewData struct {
	OriginalPrompt string
}

// SessionTypeData holds template variables for session type templates (plan, review).
type SessionTypeData struct {
	UserPrompt string
}

// PRReviewData holds template variables for the pr_review prompt.
type PRReviewData struct {
	UserPrompt string
	PRNumber   int
	PRBranch   string
	BaseBranch string
}

// SessionTypeInfo describes a session type for the API.
type SessionTypeInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Template    string `json:"-"` // template name, empty = no template (code)
}

var sessionTypes = []SessionTypeInfo{
	{Name: "code", Label: "Code", Description: "Write or modify code based on the prompt"},
	{Name: "plan", Label: "Plan", Description: "Analyze the codebase and create an implementation plan without modifying files", Template: "plan"},
	{Name: "review", Label: "Review", Description: "Review repository code quality, security, and architecture", Template: "review"},
	{Name: "pr_review", Label: "PR Review", Description: "Review a pull request / merge request diff and post comments", Template: "pr_review"},
	{Name: "knowledge", Label: "Knowledge", Description: "Analyze the repo and create or update .codeforge/ knowledge docs", Template: "knowledge"},
}

// SessionTypes returns all available session types.
func SessionTypes() []SessionTypeInfo {
	out := make([]SessionTypeInfo, len(sessionTypes))
	copy(out, sessionTypes)
	return out
}

// ValidSessionType checks if the given name is a known session type.
func ValidSessionType(name string) bool {
	for _, tt := range sessionTypes {
		if tt.Name == name {
			return true
		}
	}
	return false
}

// SessionTypeTemplate returns the template name for a session type, or "" for code.
func SessionTypeTemplate(name string) string {
	for _, tt := range sessionTypes {
		if tt.Name == name {
			return tt.Template
		}
	}
	return ""
}

// RenderTaskPrompt renders a session type template with the user prompt.
// For "code" (or unknown types with no template), it returns the original prompt.
// For "pr_review", use RenderPRReviewPrompt instead for full context.
func RenderTaskPrompt(taskType, userPrompt string) (string, error) {
	tmpl := SessionTypeTemplate(taskType)
	if tmpl == "" {
		return userPrompt, nil
	}
	return Render(tmpl, SessionTypeData{UserPrompt: userPrompt})
}

// RenderPRReviewPrompt renders the pr_review template with full PR context.
func RenderPRReviewPrompt(data PRReviewData) (string, error) {
	return Render("pr_review", data)
}

// LoadRaw reads a prompt template as raw text without template rendering.
// The name should not include the "templates/" prefix or ".md" suffix.
func LoadRaw(name string) (string, error) {
	path := "templates/" + name + ".md"
	raw, err := templateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("prompt template %q not found: %w", name, err)
	}
	return string(raw), nil
}

// templateFuncs are helper functions available inside prompt templates.
// reviewSchema returns the shared review JSON schema block so that all review
// templates (review, pr_review, code_review) render an identical schema.
var templateFuncs = template.FuncMap{
	"reviewSchema": reviewSchema,
}

// reviewSchema lazily loads the shared review output schema from the embedded FS.
func reviewSchema() (string, error) {
	raw, err := templateFS.ReadFile("templates/review_schema.md")
	if err != nil {
		return "", fmt.Errorf("review schema template not found: %w", err)
	}
	return strings.TrimRight(string(raw), "\n"), nil
}

// Render loads the named template from the embedded FS and executes it with data.
// The name should not include the "templates/" prefix or ".md" suffix.
func Render(name string, data any) (string, error) {
	path := "templates/" + name + ".md"
	raw, err := templateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("prompt template %q not found: %w", name, err)
	}

	t, err := template.New(name).Funcs(templateFuncs).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parsing prompt template %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing prompt template %q: %w", name, err)
	}

	return buf.String(), nil
}
