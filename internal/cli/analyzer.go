package cli

import (
	"context"
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
func (a *Analyzer) Analyze(_ context.Context, prompt, _ string, taskID string) *AnalysisResult {
	shortID := taskID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	title := truncateStr(prompt, 60)
	if len(prompt) > 60 {
		title += "..."
	}

	return &AnalysisResult{
		BranchSlug:  "task-" + shortID,
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
