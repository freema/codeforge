package workflow

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const maxTemplateOutput = 1 << 20 // 1MB

// TemplateContext holds data available to Go templates in workflow steps.
type TemplateContext struct {
	Params map[string]string            // workflow input parameters
	Steps  map[string]map[string]string // step name → output key → value
}

// Render evaluates a Go text/template string against the given context.
// Returns an error on missing keys or if the output exceeds 1MB.
func Render(tmpl string, ctx TemplateContext) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	funcMap := template.FuncMap{
		"repoPath": repoPath,
	}

	t, err := template.New("workflow").
		Option("missingkey=error").
		Funcs(funcMap).
		Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	if buf.Len() > maxTemplateOutput {
		return "", fmt.Errorf("template output exceeds 1MB limit (%d bytes)", buf.Len())
	}

	return buf.String(), nil
}

// repoPath extracts "owner/repo" from a full repository URL.
// e.g. "https://github.com/owner/repo.git" → "owner/repo"
func repoPath(url string) string {
	u := strings.TrimSuffix(url, ".git")
	u = strings.TrimSuffix(u, "/")

	// Handle https://host/owner/repo or git@host:owner/repo
	if idx := strings.Index(u, "://"); idx != -1 {
		u = u[idx+3:]
	}
	if idx := strings.Index(u, "@"); idx != -1 {
		u = u[idx+1:]
		u = strings.Replace(u, ":", "/", 1)
	}

	parts := strings.Split(u, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return url
}
