package session

import (
	"errors"
	"testing"

	"github.com/freema/codeforge/internal/apperror"
)

func TestValidTransitions(t *testing.T) {
	valid := []struct {
		from, to Status
	}{
		{StatusPending, StatusCloning},
		{StatusPending, StatusRunning},
		{StatusPending, StatusFailed},
		{StatusCloning, StatusRunning},
		{StatusCloning, StatusFailed},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusCompleted, StatusAwaitingInstruction},
		{StatusCompleted, StatusCreatingPR},
		{StatusCompleted, StatusReviewing},
		{StatusReviewing, StatusCompleted},
		{StatusReviewing, StatusFailed},
		{StatusAwaitingInstruction, StatusRunning},
		{StatusAwaitingInstruction, StatusReviewing},
		{StatusAwaitingInstruction, StatusFailed},
		{StatusCreatingPR, StatusPRCreated},
		{StatusCreatingPR, StatusFailed},
		{StatusPRCreated, StatusAwaitingInstruction},
		{StatusPRCreated, StatusReviewing},
		{StatusPRCreated, StatusCreatingPR},
		{StatusPRCreated, StatusCompleted},
	}

	for _, tt := range valid {
		if err := ValidateTransition(tt.from, tt.to); err != nil {
			t.Errorf("expected valid transition %s → %s, got error: %v", tt.from, tt.to, err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := []struct {
		from, to Status
	}{
		{StatusPending, StatusCompleted},
		{StatusCloning, StatusPending},
		{StatusCloning, StatusCompleted},
		{StatusRunning, StatusCloning},
		{StatusRunning, StatusPending},
		{StatusRunning, StatusReviewing},
		{StatusFailed, StatusPending},
		{StatusFailed, StatusRunning},
		{StatusFailed, StatusCompleted},
		{StatusCompleted, StatusPending},
		{StatusCompleted, StatusRunning},
		{StatusPRCreated, StatusRunning},
	}

	for _, tt := range invalid {
		err := ValidateTransition(tt.from, tt.to)
		if err == nil {
			t.Errorf("expected invalid transition %s → %s, got nil", tt.from, tt.to)
		}
		if !errors.Is(err, apperror.ErrInvalidTransition) {
			t.Errorf("expected ErrInvalidTransition for %s → %s, got: %v", tt.from, tt.to, err)
		}
	}
}

func TestIsFinished(t *testing.T) {
	// Only failed is truly terminal
	finished := []Status{StatusFailed}
	for _, s := range finished {
		if !IsFinished(s) {
			t.Errorf("%s should be finished", s)
		}
	}

	notFinished := []Status{StatusPending, StatusCloning, StatusRunning, StatusReviewing, StatusAwaitingInstruction, StatusCreatingPR, StatusCompleted, StatusPRCreated}
	for _, s := range notFinished {
		if IsFinished(s) {
			t.Errorf("%s should not be finished", s)
		}
	}
}

func TestIsIdle(t *testing.T) {
	idle := []Status{StatusCompleted, StatusPRCreated}
	for _, s := range idle {
		if !IsIdle(s) {
			t.Errorf("%s should be idle", s)
		}
	}

	notIdle := []Status{StatusPending, StatusCloning, StatusRunning, StatusReviewing, StatusAwaitingInstruction, StatusCreatingPR, StatusFailed}
	for _, s := range notIdle {
		if IsIdle(s) {
			t.Errorf("%s should not be idle", s)
		}
	}
}
