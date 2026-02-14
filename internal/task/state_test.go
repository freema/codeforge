package task

import (
	"errors"
	"testing"

	"github.com/freema/codeforge/internal/apperror"
)

func TestValidTransitions(t *testing.T) {
	valid := []struct {
		from, to TaskStatus
	}{
		{StatusPending, StatusCloning},
		{StatusPending, StatusFailed},
		{StatusCloning, StatusRunning},
		{StatusCloning, StatusFailed},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusCompleted, StatusAwaitingInstruction},
		{StatusCompleted, StatusCreatingPR},
		{StatusAwaitingInstruction, StatusRunning},
		{StatusAwaitingInstruction, StatusFailed},
		{StatusCreatingPR, StatusPRCreated},
		{StatusCreatingPR, StatusFailed},
		{StatusPRCreated, StatusAwaitingInstruction},
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
		from, to TaskStatus
	}{
		{StatusPending, StatusRunning},
		{StatusPending, StatusCompleted},
		{StatusCloning, StatusPending},
		{StatusCloning, StatusCompleted},
		{StatusRunning, StatusCloning},
		{StatusRunning, StatusPending},
		{StatusFailed, StatusPending},
		{StatusFailed, StatusRunning},
		{StatusFailed, StatusCompleted},
		{StatusCompleted, StatusPending},
		{StatusCompleted, StatusRunning},
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
	finished := []TaskStatus{StatusCompleted, StatusFailed, StatusPRCreated}
	for _, s := range finished {
		if !IsFinished(s) {
			t.Errorf("%s should be finished", s)
		}
	}

	notFinished := []TaskStatus{StatusPending, StatusCloning, StatusRunning, StatusAwaitingInstruction, StatusCreatingPR}
	for _, s := range notFinished {
		if IsFinished(s) {
			t.Errorf("%s should not be finished", s)
		}
	}
}
