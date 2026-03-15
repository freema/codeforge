package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/freema/codeforge/internal/review"
)

// writeGitLabOutput writes results to GitLab CI dotenv artifact and stdout.
func writeGitLabOutput(reviewResult *review.ReviewResult, rawOutput string, outputFormat string) {
	// Write dotenv artifact for downstream jobs
	writeGitLabDotenv(reviewResult)

	// Write to stdout for CI log
	if reviewResult != nil && outputFormat == "json" {
		data, _ := json.MarshalIndent(reviewResult, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println(rawOutput)
	}
}

// writeGitLabDotenv writes variables to a dotenv file for GitLab CI artifacts.
// The file can be consumed via `artifacts: reports: dotenv: codeforge.env`
func writeGitLabDotenv(reviewResult *review.ReviewResult) {
	dotenvPath := "codeforge.env"

	f, err := os.Create(dotenvPath)
	if err != nil {
		slog.Warn("failed to create dotenv file", "error", err)
		return
	}
	defer f.Close()

	if reviewResult != nil {
		fmt.Fprintf(f, "CODEFORGE_VERDICT=%s\n", reviewResult.Verdict)
		fmt.Fprintf(f, "CODEFORGE_SCORE=%d\n", reviewResult.Score)
		fmt.Fprintf(f, "CODEFORGE_ISSUES_COUNT=%d\n", len(reviewResult.Issues))
	}
}
