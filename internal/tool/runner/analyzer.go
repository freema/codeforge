package runner

import (
	"context"

	"github.com/freema/codeforge/internal/slug"
)

// AnalysisResult holds auto-generated PR metadata.
type AnalysisResult struct {
	BranchSlug  string
	PRTitle     string
	Description string
}

// Analyzer generates PR metadata from a task prompt.
type Analyzer struct{}

// NewAnalyzer creates a prompt analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze generates branch slug, PR title, and description from task prompt.
func (a *Analyzer) Analyze(_ context.Context, prompt string, taskID string) *AnalysisResult {
	title := truncateStr(prompt, 60)
	if len(prompt) > 60 {
		title += "..."
	}

	return &AnalysisResult{
		BranchSlug:  slug.Generate(prompt, taskID),
		PRTitle:     "CodeForge: " + title,
		Description: "Automated changes by CodeForge.",
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
