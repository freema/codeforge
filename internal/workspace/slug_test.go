package workspace

import (
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	// Verify delegation to slug package works correctly.
	tests := []struct {
		name   string
		prompt string
		taskID string
		want   string
	}{
		{
			name:   "basic prompt",
			prompt: "Fix the failing auth tests",
			taskID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "fix-the-failing-auth-tests-550e8400",
		},
		{
			name:   "empty prompt",
			prompt: "",
			taskID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "task-550e8400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSlug(tt.prompt, tt.taskID)
			if got != tt.want {
				t.Errorf("GenerateSlug(%q, %q) = %q, want %q", tt.prompt, tt.taskID, got, tt.want)
			}
		})
	}
}
