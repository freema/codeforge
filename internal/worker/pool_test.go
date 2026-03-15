package worker

import (
	"testing"

	"github.com/freema/codeforge/internal/task"
)

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		status task.TaskStatus
		want   bool
	}{
		{task.StatusPending, true},
		{task.StatusAwaitingInstruction, true},
		{task.StatusReviewing, true},
		{task.StatusCompleted, false},
		{task.StatusFailed, false},
		{task.StatusRunning, false},
		{task.StatusCloning, false},
		{task.StatusCreatingPR, false},
		{task.StatusPRCreated, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := shouldProcess(tt.status)
			if got != tt.want {
				t.Errorf("shouldProcess(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
