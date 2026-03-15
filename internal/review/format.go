package review

import (
	"fmt"
	"strings"
)

// severityLabel maps severity to a visual indicator for PR/MR comments.
var severityLabel = map[string]string{
	"critical":   "CRITICAL",
	"major":      "MAJOR",
	"minor":      "MINOR",
	"suggestion": "SUGGESTION",
}

// FormatSummaryBody renders the review summary as a markdown comment body.
func FormatSummaryBody(result *ReviewResult, nonFileIssues []ReviewIssue) string {
	var b strings.Builder

	b.WriteString("## CodeForge Review\n\n")

	// Verdict and score
	b.WriteString(fmt.Sprintf("**Verdict:** %s | **Score:** %d/10\n\n", result.Verdict, result.Score))

	// Summary
	if result.Summary != "" {
		b.WriteString(result.Summary)
		b.WriteString("\n")
	}

	// Non-file issues (those without file/line info)
	if len(nonFileIssues) > 0 {
		b.WriteString("\n### General Issues\n\n")
		for _, issue := range nonFileIssues {
			label := severityLabel[issue.Severity]
			if label == "" {
				label = strings.ToUpper(issue.Severity)
			}
			b.WriteString(fmt.Sprintf("- **[%s]** %s", label, issue.Description))
			if issue.Suggestion != "" {
				b.WriteString(fmt.Sprintf("\n  > Suggestion: %s", issue.Suggestion))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n---\n*Reviewed by [CodeForge](https://github.com/freema/codeforge)*\n")

	return b.String()
}

// FormatIssueComment renders a single review issue as a comment body.
func FormatIssueComment(issue ReviewIssue) string {
	label := severityLabel[issue.Severity]
	if label == "" {
		label = strings.ToUpper(issue.Severity)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("**[%s]** %s", label, issue.Description))

	if issue.Suggestion != "" {
		b.WriteString(fmt.Sprintf("\n\n> Suggestion: %s", issue.Suggestion))
	}

	return b.String()
}
