package worker

import (
	"testing"

	"github.com/freema/codeforge/internal/session"
)

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		status session.Status
		want   bool
	}{
		{session.StatusPending, true},
		{session.StatusAwaitingInstruction, true},
		{session.StatusReviewing, true},
		{session.StatusCompleted, false},
		{session.StatusFailed, false},
		{session.StatusRunning, false},
		{session.StatusCloning, false},
		{session.StatusCreatingPR, false},
		{session.StatusPRCreated, false},
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
