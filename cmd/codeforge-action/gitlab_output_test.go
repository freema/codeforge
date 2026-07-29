package main

import (
	"os"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/review"
)

func TestWriteGitLabDotenv(t *testing.T) {
	tests := []struct {
		name         string
		reviewResult *review.ReviewResult
		inputTokens  int
		outputTokens int
		reviewURL    string
		wantLines    []string
		absentPrefix []string
	}{
		{
			name: "review with URL writes full parity set",
			reviewResult: &review.ReviewResult{
				Verdict: review.VerdictApprove,
				Score:   9,
				Issues:  []review.ReviewIssue{{Severity: "minor", File: "a.go", Line: 3}},
			},
			inputTokens:  1200,
			outputTokens: 340,
			reviewURL:    "https://gitlab.example.com/group/repo/-/merge_requests/5#note_1",
			wantLines: []string{
				"CODEFORGE_VERDICT=approve",
				"CODEFORGE_SCORE=9",
				"CODEFORGE_ISSUES_COUNT=1",
				"CODEFORGE_INPUT_TOKENS=1200",
				"CODEFORGE_OUTPUT_TOKENS=340",
				"CODEFORGE_REVIEW_URL=https://gitlab.example.com/group/repo/-/merge_requests/5#note_1",
			},
		},
		{
			name:         "nil review still writes token usage",
			reviewResult: nil,
			inputTokens:  10,
			outputTokens: 20,
			wantLines: []string{
				"CODEFORGE_INPUT_TOKENS=10",
				"CODEFORGE_OUTPUT_TOKENS=20",
			},
			absentPrefix: []string{
				"CODEFORGE_VERDICT=",
				"CODEFORGE_SCORE=",
				"CODEFORGE_ISSUES_COUNT=",
				"CODEFORGE_REVIEW_URL=",
			},
		},
		{
			name: "empty review URL omits the variable",
			reviewResult: &review.ReviewResult{
				Verdict: review.VerdictRequestChanges,
				Score:   4,
			},
			wantLines: []string{
				"CODEFORGE_VERDICT=request_changes",
				"CODEFORGE_SCORE=4",
				"CODEFORGE_ISSUES_COUNT=0",
			},
			absentPrefix: []string{"CODEFORGE_REVIEW_URL="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			writeGitLabDotenv(tt.reviewResult, tt.inputTokens, tt.outputTokens, tt.reviewURL)

			data, err := os.ReadFile("codeforge.env")
			if err != nil {
				t.Fatalf("reading codeforge.env: %v", err)
			}
			content := string(data)
			lines := strings.Split(strings.TrimSpace(content), "\n")

			for _, want := range tt.wantLines {
				found := false
				for _, line := range lines {
					if line == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("dotenv missing line %q, got:\n%s", want, content)
				}
			}

			for _, prefix := range tt.absentPrefix {
				for _, line := range lines {
					if strings.HasPrefix(line, prefix) {
						t.Errorf("dotenv should not contain %q line, got %q", prefix, line)
					}
				}
			}
		})
	}
}
