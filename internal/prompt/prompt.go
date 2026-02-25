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
