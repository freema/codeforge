package review

import (
	"testing"
)

func TestParseReviewOutput(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantVerdict     Verdict
		wantScore       int
		wantIssuesCount int
	}{
		{
			name: "CleanJSON",
			input: `{
				"verdict": "approve",
				"score": 9,
				"summary": "Clean code",
				"issues": [],
				"auto_fixable": false
			}`,
			wantVerdict:     VerdictApprove,
			wantScore:       9,
			wantIssuesCount: 0,
		},
		{
			name: "JSONInMarkdown",
			input: "Here is my review:\n\n```json\n" + `{
				"verdict": "request_changes",
				"score": 4,
				"summary": "Several issues found",
				"issues": [
					{"severity": "critical", "file": "main.go", "line": 10, "description": "SQL injection"}
				],
				"auto_fixable": false
			}` + "\n```\n\nPlease fix these.",
			wantVerdict:     VerdictRequestChanges,
			wantScore:       4,
			wantIssuesCount: 1,
		},
		{
			name: "JSONWithPreamble",
			input: `I reviewed the code carefully. Here are my findings:

			{"verdict": "comment", "score": 6, "summary": "Some minor issues", "issues": [{"severity": "minor", "file": "util.go", "line": 5, "description": "naming"}], "auto_fixable": false}

			End of review.`,
			wantVerdict:     VerdictComment,
			wantScore:       6,
			wantIssuesCount: 1,
		},
		{
			name:            "InvalidJSON",
			input:           "This is not JSON at all, just a plain text review. The code looks fine.",
			wantVerdict:     VerdictComment,
			wantScore:       0,
			wantIssuesCount: 0,
		},
		{
			name: "ScoreOutOfRangeHigh",
			input: `{
				"verdict": "approve",
				"score": 15,
				"summary": "Perfect",
				"issues": [],
				"auto_fixable": false
			}`,
			wantVerdict: VerdictApprove,
			wantScore:   10,
		},
		{
			name: "ScoreOutOfRangeLow",
			input: `{
				"verdict": "request_changes",
				"score": -5,
				"summary": "Terrible",
				"issues": [],
				"auto_fixable": false
			}`,
			wantVerdict: VerdictRequestChanges,
			wantScore:   1,
		},
		{
			name: "UnknownVerdict",
			input: `{
				"verdict": "maybe",
				"score": 5,
				"summary": "Uncertain",
				"issues": [],
				"auto_fixable": false
			}`,
			wantVerdict: VerdictComment,
			wantScore:   5,
		},
		{
			name: "UnknownSeverity",
			input: `{
				"verdict": "comment",
				"score": 7,
				"summary": "Some findings",
				"issues": [
					{"severity": "blocker", "file": "main.go", "description": "Bad pattern"},
					{"severity": "critical", "file": "db.go", "description": "SQL issue"}
				],
				"auto_fixable": false
			}`,
			wantVerdict:     VerdictComment,
			wantScore:       7,
			wantIssuesCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseReviewOutput(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("result is nil")
			}

			if result.Verdict != tt.wantVerdict {
				t.Errorf("verdict: got %q, want %q", result.Verdict, tt.wantVerdict)
			}
			if result.Score != tt.wantScore {
				t.Errorf("score: got %d, want %d", result.Score, tt.wantScore)
			}
			if tt.wantIssuesCount > 0 && len(result.Issues) != tt.wantIssuesCount {
				t.Errorf("issues count: got %d, want %d", len(result.Issues), tt.wantIssuesCount)
			}

			// Verify unknown severity was normalized
			if tt.name == "UnknownSeverity" {
				if result.Issues[0].Severity != "suggestion" {
					t.Errorf("expected unknown severity to be normalized to 'suggestion', got %q", result.Issues[0].Severity)
				}
				if result.Issues[1].Severity != "critical" {
					t.Errorf("expected valid severity to be preserved, got %q", result.Issues[1].Severity)
				}
			}
		})
	}
}

func TestParseReviewOutput_EmptyInput(t *testing.T) {
	result, err := ParseReviewOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Verdict != VerdictComment {
		t.Errorf("expected VerdictComment fallback, got %q", result.Verdict)
	}
}
