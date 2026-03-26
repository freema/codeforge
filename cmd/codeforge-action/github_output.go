package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/freema/codeforge/internal/review"
)

// writeGitHubOutput writes results to $GITHUB_OUTPUT and $GITHUB_STEP_SUMMARY.
func writeGitHubOutput(ciCtx *CIContext, reviewResult *review.ReviewResult, rawOutput string, outputFormat string, inputTokens, outputTokens int) {
	// Write to $GITHUB_OUTPUT (key=value pairs for downstream steps)
	writeGitHubOutputVars(reviewResult, rawOutput, outputFormat, inputTokens, outputTokens)

	// Write to $GITHUB_STEP_SUMMARY (markdown rendered in Actions UI)
	writeGitHubStepSummary(ciCtx, reviewResult, rawOutput, inputTokens, outputTokens)

	// Write human-readable summary to stdout (visible in CI log)
	writeTerminalSummary(reviewResult, rawOutput, inputTokens, outputTokens)
}

func writeGitHubOutputVars(reviewResult *review.ReviewResult, rawOutput string, outputFormat string, inputTokens, outputTokens int) {
	outputPath := os.Getenv("GITHUB_OUTPUT")
	if outputPath == "" {
		return
	}

	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		slog.Warn("failed to open GITHUB_OUTPUT", "error", err)
		return
	}
	defer f.Close()

	if reviewResult != nil {
		fmt.Fprintf(f, "verdict=%s\n", reviewResult.Verdict)
		fmt.Fprintf(f, "score=%d\n", reviewResult.Score)
		fmt.Fprintf(f, "issues_count=%d\n", len(reviewResult.Issues))

		if outputFormat == "json" {
			// Use delimiter for multiline values
			data, _ := json.Marshal(reviewResult)
			fmt.Fprintf(f, "review<<EOF\n%s\nEOF\n", string(data))
		}
	}

	// Token usage
	fmt.Fprintf(f, "input_tokens=%d\n", inputTokens)
	fmt.Fprintf(f, "output_tokens=%d\n", outputTokens)

	// Raw output (truncated for GitHub Actions limits)
	output := rawOutput
	if len(output) > 50000 {
		output = output[:50000] + "\n... (truncated)"
	}
	fmt.Fprintf(f, "output<<EOF\n%s\nEOF\n", output)
}

func writeGitHubStepSummary(ciCtx *CIContext, reviewResult *review.ReviewResult, rawOutput string, inputTokens, outputTokens int) {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return
	}

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		slog.Warn("failed to open GITHUB_STEP_SUMMARY", "error", err)
		return
	}
	defer f.Close()

	if reviewResult != nil {
		writeReviewSummaryMarkdown(f, ciCtx, reviewResult, inputTokens, outputTokens)
	} else {
		// Non-review output
		fmt.Fprintf(f, "## CodeForge Result\n\n")
		if len(rawOutput) > 10000 {
			rawOutput = rawOutput[:10000] + "\n... (truncated)"
		}
		fmt.Fprintf(f, "```\n%s\n```\n", rawOutput)
	}
}

func writeTerminalSummary(r *review.ReviewResult, rawOutput string, inputTokens, outputTokens int) {
	if r == nil {
		// Non-review session — print truncated output
		output := rawOutput
		if len(output) > 2000 {
			output = output[:2000] + "\n... (truncated, full output in $GITHUB_OUTPUT)"
		}
		fmt.Println(output)
		return
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────┐")
	fmt.Printf("│  CodeForge Review: %-8s  Score: %d/10    │\n", strings.ToUpper(string(r.Verdict)), r.Score)
	fmt.Println("└─────────────────────────────────────────────┘")
	fmt.Println()

	if r.Summary != "" {
		fmt.Println(r.Summary)
		fmt.Println()
	}

	if len(r.Issues) > 0 {
		counts := make(map[string]int)
		for _, issue := range r.Issues {
			counts[issue.Severity]++
		}

		fmt.Println("Issues:")
		for _, sev := range []string{"critical", "major", "minor", "suggestion"} {
			if c, ok := counts[sev]; ok {
				fmt.Printf("  %-12s %d\n", sev, c)
			}
		}
		fmt.Println()

		// Print top 5 issues with file:line
		limit := 5
		if len(r.Issues) < limit {
			limit = len(r.Issues)
		}
		for i := 0; i < limit; i++ {
			issue := r.Issues[i]
			loc := issue.File
			if issue.Line > 0 {
				loc = fmt.Sprintf("%s:%d", issue.File, issue.Line)
			}
			desc := issue.Description
			if len(desc) > 120 {
				desc = desc[:120] + "..."
			}
			fmt.Printf("  [%s] %s\n", issue.Severity, loc)
			fmt.Printf("    %s\n", desc)
		}
		if len(r.Issues) > limit {
			fmt.Printf("  ... and %d more (see PR comments)\n", len(r.Issues)-limit)
		}
		fmt.Println()
	}

	fmt.Printf("Tokens: %d input / %d output\n", inputTokens, outputTokens)
	if r.ReviewedBy != "" {
		fmt.Printf("Model:  %s\n", r.ReviewedBy)
	}
	fmt.Println()
}

func writeReviewSummaryMarkdown(f *os.File, _ *CIContext, r *review.ReviewResult, inputTokens, outputTokens int) {
	fmt.Fprintf(f, "## CodeForge Review\n\n")
	fmt.Fprintf(f, "**Verdict:** %s | **Score:** %d/10\n\n", r.Verdict, r.Score)

	if r.Summary != "" {
		fmt.Fprintf(f, "%s\n\n", r.Summary)
	}

	// Issue severity breakdown
	if len(r.Issues) > 0 {
		counts := make(map[string]int)
		for _, issue := range r.Issues {
			counts[issue.Severity]++
		}

		fmt.Fprintf(f, "| Severity | Count |\n|----------|-------|\n")
		for _, sev := range []string{"critical", "major", "minor", "suggestion"} {
			if c, ok := counts[sev]; ok {
				label := strings.ToUpper(sev[:1]) + sev[1:]
				fmt.Fprintf(f, "| %s | %d |\n", label, c)
			}
		}
		fmt.Fprintf(f, "\n")
	}

	if inputTokens > 0 || outputTokens > 0 {
		fmt.Fprintf(f, "\n**Tokens:** %d input, %d output\n", inputTokens, outputTokens)
	}

	fmt.Fprintf(f, "---\n*Reviewed by CodeForge (%s)*\n", r.ReviewedBy)
}
