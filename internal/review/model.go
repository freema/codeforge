package review

import "encoding/json"

// Verdict represents the review outcome.
type Verdict string

const (
	VerdictApprove        Verdict = "approve"
	VerdictRequestChanges Verdict = "request_changes"
	VerdictComment        Verdict = "comment"
)

// ReviewResult holds the structured output of a code review.
type ReviewResult struct {
	Verdict         Verdict       `json:"verdict"`
	Score           int           `json:"score"`            // 1-10
	Summary         string        `json:"summary"`
	Issues          []ReviewIssue `json:"issues"`
	AutoFixable     bool          `json:"auto_fixable"`
	ReviewedBy      string        `json:"reviewed_by"`      // "cli:model"
	DurationSeconds float64       `json:"duration_seconds"`
}

// ReviewIssue describes a single finding from the review.
type ReviewIssue struct {
	Severity    string `json:"severity"` // critical, major, minor, suggestion
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// MarshalReviewResult serializes a ReviewResult to JSON string for Redis storage.
func MarshalReviewResult(r *ReviewResult) string {
	if r == nil {
		return ""
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// UnmarshalReviewResult deserializes a ReviewResult from JSON string.
// Returns nil on empty or invalid input.
func UnmarshalReviewResult(data string) *ReviewResult {
	if data == "" {
		return nil
	}
	var r ReviewResult
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil
	}
	return &r
}
