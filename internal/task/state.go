package task

import (
	"fmt"

	"github.com/freema/codeforge/internal/apperror"
)

// validTransitions defines valid state machine transitions.
var validTransitions = map[TaskStatus][]TaskStatus{
	StatusPending:             {StatusCloning, StatusFailed},
	StatusCloning:             {StatusRunning, StatusFailed},
	StatusRunning:             {StatusCompleted, StatusFailed},
	StatusCompleted:           {StatusAwaitingInstruction, StatusCreatingPR},
	StatusFailed:              {}, // terminal for this iteration
	StatusAwaitingInstruction: {StatusRunning, StatusFailed},
	StatusCreatingPR:          {StatusPRCreated, StatusFailed},
	StatusPRCreated:           {StatusAwaitingInstruction, StatusCompleted},
}

// ValidateTransition checks if the transition from current to next status is valid.
func ValidateTransition(current, next TaskStatus) error {
	allowed, ok := validTransitions[current]
	if !ok {
		return &apperror.AppError{
			Err:     apperror.ErrInvalidTransition,
			Message: fmt.Sprintf("unknown status: %s", current),
			Status:  409,
		}
	}

	for _, s := range allowed {
		if s == next {
			return nil
		}
	}

	return &apperror.AppError{
		Err:     apperror.ErrInvalidTransition,
		Message: fmt.Sprintf("invalid transition: %s → %s", current, next),
		Status:  409,
	}
}

// IsTerminal returns true if the status is a terminal state (no further transitions for this iteration).
func IsTerminal(s TaskStatus) bool {
	return s == StatusFailed
}

// IsFinished returns true if the task has reached a completion state.
func IsFinished(s TaskStatus) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusPRCreated
}

// ValidStatuses returns all valid status values.
func ValidStatuses() []TaskStatus {
	return []TaskStatus{
		StatusPending,
		StatusCloning,
		StatusRunning,
		StatusCompleted,
		StatusFailed,
		StatusAwaitingInstruction,
		StatusCreatingPR,
		StatusPRCreated,
	}
}
