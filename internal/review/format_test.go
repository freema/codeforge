package review

import (
	"strings"
	"testing"
)

func TestFormatSummaryBody(t *testing.T) {
	tests := []struct {
		name          string
		result        *ReviewResult
		nonFileIssues []ReviewIssue
		wantContains  []string
		wantAbsent    []string
	}{
		{
			name: "ApproveNoIssues",
			result: &ReviewResult{
				Verdict: VerdictApprove,
				Score:   9,
				Summary: "Code looks great, well structured.",
			},
			nonFileIssues: nil,
			wantContains: []string{
				"## CodeForge Review",
				"**Verdict:** approve",
				"**Score:** 9/10",
				"Code looks great, well structured.",
				"Reviewed by [CodeForge]",
			},
			wantAbsent: []string{
				"### General Issues",
			},
		},
		{
			name: "RequestChangesWithNonFileIssues",
			result: &ReviewResult{
				Verdict: VerdictRequestChanges,
				Score:   3,
				Summary: "Several problems found.",
			},
			nonFileIssues: []ReviewIssue{
				{
					Severity:    "critical",
					Description: "Missing error handling in main flow",
					Suggestion:  "Add proper error wrapping with fmt.Errorf",
				},
				{
					Severity:    "minor",
					Description: "Inconsistent naming conventions",
				},
			},
			wantContains: []string{
				"**Verdict:** request_changes",
				"**Score:** 3/10",
				"Several problems found.",
				"### General Issues",
				"**[CRITICAL]** Missing error handling in main flow",
				"> Suggestion: Add proper error wrapping with fmt.Errorf",
				"**[MINOR]** Inconsistent naming conventions",
			},
		},
		{
			name: "EmptySummary",
			result: &ReviewResult{
				Verdict: VerdictComment,
				Score:   5,
				Summary: "",
			},
			nonFileIssues: nil,
			wantContains: []string{
				"## CodeForge Review",
				"**Verdict:** comment",
				"**Score:** 5/10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSummaryBody(tt.result, tt.nonFileIssues)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing expected substring %q\ngot:\n%s", want, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

func TestFormatIssueComment(t *testing.T) {
	tests := []struct {
		name         string
		issue        ReviewIssue
		wantContains []string
		wantAbsent   []string
	}{
		{
			name: "CriticalSeverity",
			issue: ReviewIssue{
				Severity:    "critical",
				File:        "main.go",
				Line:        42,
				Description: "SQL injection vulnerability",
			},
			wantContains: []string{
				"**[CRITICAL]**",
				"SQL injection vulnerability",
			},
			wantAbsent: []string{
				"Suggestion:",
			},
		},
		{
			name: "MajorSeverity",
			issue: ReviewIssue{
				Severity:    "major",
				File:        "handler.go",
				Line:        15,
				Description: "Unhandled error return",
			},
			wantContains: []string{
				"**[MAJOR]**",
				"Unhandled error return",
			},
		},
		{
			name: "WithSuggestion",
			issue: ReviewIssue{
				Severity:    "minor",
				File:        "util.go",
				Line:        8,
				Description: "Variable name too short",
				Suggestion:  "Rename 'x' to 'count' for clarity",
			},
			wantContains: []string{
				"**[MINOR]**",
				"Variable name too short",
				"> Suggestion: Rename 'x' to 'count' for clarity",
			},
		},
		{
			name: "UnknownSeverityUppercased",
			issue: ReviewIssue{
				Severity:    "blocker",
				File:        "db.go",
				Line:        99,
				Description: "Connection pool exhaustion risk",
			},
			wantContains: []string{
				"**[BLOCKER]**",
				"Connection pool exhaustion risk",
			},
		},
		{
			name: "WithoutSuggestion",
			issue: ReviewIssue{
				Severity:    "suggestion",
				File:        "config.go",
				Description: "Consider using constants for magic numbers",
			},
			wantContains: []string{
				"**[SUGGESTION]**",
				"Consider using constants for magic numbers",
			},
			wantAbsent: []string{
				"Suggestion:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatIssueComment(tt.issue)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing expected substring %q\ngot:\n%s", want, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}
