package session

import (
	"fmt"

	"github.com/freema/codeforge/internal/apperror"
)

// validTransitions defines valid state machine transitions.
// Session is a session — completed and pr_created are NOT terminal.
// They allow review/fix/instruct loops.
var validTransitions = map[Status][]Status{
	StatusPending:             {StatusCloning, StatusRunning, StatusFailed},
	StatusCloning:             {StatusRunning, StatusFailed},
	StatusRunning:             {StatusCompleted, StatusFailed},
	StatusReviewing:           {StatusCompleted, StatusFailed},
	StatusCompleted:           {StatusAwaitingInstruction, StatusCreatingPR, StatusReviewing},
	StatusFailed:              {}, // terminal — only truly dead state
	StatusAwaitingInstruction: {StatusRunning, StatusReviewing, StatusFailed},
	StatusCreatingPR:          {StatusPRCreated, StatusFailed},
	StatusPRCreated:           {StatusAwaitingInstruction, StatusReviewing, StatusCreatingPR, StatusCompleted},
}

// ValidateTransition checks if the transition from current to next status is valid.
func ValidateTransition(current, next Status) error {
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

// IsFinished returns true if the session has reached a terminal state.
// Only failed is truly terminal — completed and pr_created allow further interaction.
func IsFinished(s Status) bool {
	return s == StatusFailed
}

// IsIdle returns true if the session is in a resting state (not actively processing)
// but can still accept new interactions (review, instruct, etc.).
func IsIdle(s Status) bool {
	return s == StatusCompleted || s == StatusPRCreated
}
