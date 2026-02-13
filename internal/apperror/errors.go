package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common conditions.
var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation error")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrConflict          = errors.New("conflict")
	ErrInternal          = errors.New("internal error")
	ErrInvalidTransition = errors.New("invalid state transition")
)

// AppError is a structured error with an HTTP status code and optional fields.
type AppError struct {
	Err     error
	Message string
	Status  int
	Fields  map[string]string
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Err.Error()
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NotFound creates a 404 error.
func NotFound(format string, args ...interface{}) *AppError {
	return &AppError{
		Err:     ErrNotFound,
		Message: fmt.Sprintf(format, args...),
		Status:  http.StatusNotFound,
	}
}

// Validation creates a 400 error.
func Validation(format string, args ...interface{}) *AppError {
	return &AppError{
		Err:     ErrValidation,
		Message: fmt.Sprintf(format, args...),
		Status:  http.StatusBadRequest,
	}
}

// Conflict creates a 409 error.
func Conflict(format string, args ...interface{}) *AppError {
	return &AppError{
		Err:     ErrConflict,
		Message: fmt.Sprintf(format, args...),
		Status:  http.StatusConflict,
	}
}

// Internal creates a 500 error.
func Internal(format string, args ...interface{}) *AppError {
	return &AppError{
		Err:     ErrInternal,
		Message: fmt.Sprintf(format, args...),
		Status:  http.StatusInternalServerError,
	}
}

// HTTPStatus extracts the HTTP status code from an error, defaulting to 500.
func HTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Status
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrValidation) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
