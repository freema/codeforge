package review

import "testing"

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := &ReviewResult{
		Verdict: VerdictApprove,
		Score:   8,
		Summary: "Code looks good",
		Issues: []ReviewIssue{
			{Severity: "minor", File: "main.go", Line: 42, Description: "unused var"},
		},
		AutoFixable:     false,
		ReviewedBy:      "claude-code:claude-sonnet-4-6",
		DurationSeconds: 12.5,
	}

	data := MarshalReviewResult(original)
	if data == "" {
		t.Fatal("MarshalReviewResult returned empty string")
	}

	restored := UnmarshalReviewResult(data)
	if restored == nil {
		t.Fatal("UnmarshalReviewResult returned nil")
	}

	if restored.Verdict != original.Verdict {
		t.Errorf("verdict: got %s, want %s", restored.Verdict, original.Verdict)
	}
	if restored.Score != original.Score {
		t.Errorf("score: got %d, want %d", restored.Score, original.Score)
	}
	if restored.Summary != original.Summary {
		t.Errorf("summary: got %q, want %q", restored.Summary, original.Summary)
	}
	if len(restored.Issues) != len(original.Issues) {
		t.Fatalf("issues len: got %d, want %d", len(restored.Issues), len(original.Issues))
	}
	if restored.Issues[0].File != "main.go" {
		t.Errorf("issue file: got %q, want %q", restored.Issues[0].File, "main.go")
	}
	if restored.ReviewedBy != original.ReviewedBy {
		t.Errorf("reviewed_by: got %q, want %q", restored.ReviewedBy, original.ReviewedBy)
	}
	if restored.DurationSeconds != original.DurationSeconds {
		t.Errorf("duration: got %f, want %f", restored.DurationSeconds, original.DurationSeconds)
	}
}

func TestUnmarshalEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"invalid json", "not json"},
		{"empty object", "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnmarshalReviewResult(tt.input)
			if result != nil && tt.input != "{}" {
				t.Errorf("expected nil for input %q, got %+v", tt.input, result)
			}
		})
	}
}

func TestMarshalNil(t *testing.T) {
	result := MarshalReviewResult(nil)
	if result != "" {
		t.Errorf("expected empty string for nil, got %q", result)
	}
}
