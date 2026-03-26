package runner

import (
	"context"
	"log/slog"

	"github.com/freema/codeforge/internal/ai"
	"github.com/freema/codeforge/internal/slug"
)

// AnalysisResult holds auto-generated PR metadata.
type AnalysisResult struct {
	BranchSlug  string
	PRTitle     string
	Description string
}

// Analyzer generates PR metadata from a session prompt.
type Analyzer struct {
	ai ai.Client // optional, nil = fallback mode
}

// NewAnalyzer creates a prompt analyzer. Pass nil for ai to use fallback mode.
func NewAnalyzer(aiClient ...ai.Client) *Analyzer {
	a := &Analyzer{}
	if len(aiClient) > 0 {
		a.ai = aiClient[0]
	}
	return a
}

// Analyze generates branch slug, PR title, and description from session prompt.
// If an AI client is available, it generates smart metadata.
// Otherwise falls back to simple truncation.
func (a *Analyzer) Analyze(ctx context.Context, prompt string, sessionID string) *AnalysisResult {
	branchSlug := slug.Generate(prompt, sessionID)

	// Try AI generation
	if a.ai != nil {
		meta := ai.GeneratePRMetadata(ctx, a.ai, "", prompt)
		if meta != nil {
			slog.Info("AI-generated PR metadata", "title", meta.Title)
			return &AnalysisResult{
				BranchSlug:  branchSlug,
				PRTitle:     meta.Title,
				Description: meta.Description,
			}
		}
	}

	// Fallback: truncate prompt
	title := truncateStr(prompt, 60)
	if len(prompt) > 60 {
		title += "..."
	}

	return &AnalysisResult{
		BranchSlug:  branchSlug,
		PRTitle:     title,
		Description: "Automated changes by CodeForge.",
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
