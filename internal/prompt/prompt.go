package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.md
var templateFS embed.FS

// CodeReviewData holds template variables for the code_review prompt.
type CodeReviewData struct {
	OriginalPrompt string
}

// TaskTypeData holds template variables for task type templates (plan, review).
type TaskTypeData struct {
	UserPrompt string
}

// TaskTypeInfo describes a task type for the API.
type TaskTypeInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Template    string `json:"-"` // template name, empty = no template (code)
}

var taskTypes = []TaskTypeInfo{
	{Name: "code", Label: "Code", Description: "Write or modify code based on the prompt"},
	{Name: "plan", Label: "Plan", Description: "Analyze the codebase and create an implementation plan without modifying files", Template: "plan"},
	{Name: "review", Label: "Review", Description: "Review repository code quality, security, and architecture", Template: "review"},
}

// TaskTypes returns all available task types.
func TaskTypes() []TaskTypeInfo {
	out := make([]TaskTypeInfo, len(taskTypes))
	copy(out, taskTypes)
	return out
}

// ValidTaskType checks if the given name is a known task type.
func ValidTaskType(name string) bool {
	for _, tt := range taskTypes {
		if tt.Name == name {
			return true
		}
	}
	return false
}

// TaskTypeTemplate returns the template name for a task type, or "" for code.
func TaskTypeTemplate(name string) string {
	for _, tt := range taskTypes {
		if tt.Name == name {
			return tt.Template
		}
	}
	return ""
}

// RenderTaskPrompt renders a task type template with the user prompt.
// For "code" (or unknown types with no template), it returns the original prompt.
func RenderTaskPrompt(taskType, userPrompt string) (string, error) {
	tmpl := TaskTypeTemplate(taskType)
	if tmpl == "" {
		return userPrompt, nil
	}
	return Render(tmpl, TaskTypeData{UserPrompt: userPrompt})
}

// Render loads the named template from the embedded FS and executes it with data.
// The name should not include the "templates/" prefix or ".md" suffix.
func Render(name string, data any) (string, error) {
	path := "templates/" + name + ".md"
	raw, err := templateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("prompt template %q not found: %w", name, err)
	}

	t, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parsing prompt template %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing prompt template %q: %w", name, err)
	}

	return buf.String(), nil
}
