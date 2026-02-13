package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/task"
)

var validate = validator.New()

// TaskHandler handles task-related HTTP endpoints.
type TaskHandler struct {
	service   *task.Service
	prService *task.PRService
}

// NewTaskHandler creates a new task handler.
func NewTaskHandler(service *task.Service, prService *task.PRService) *TaskHandler {
	return &TaskHandler{service: service, prService: prService}
}

// Create handles POST /api/v1/tasks.
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req task.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validate.Struct(req); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			fields := make(map[string]string)
			for _, e := range validationErrs {
				fields[e.Field()] = formatValidationError(e)
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": fields,
			})
			return
		}
		writeError(w, http.StatusBadRequest, "validation failed")
		return
	}

	t, err := h.service.Create(r.Context(), req)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         t.ID,
		"status":     t.Status,
		"created_at": t.CreatedAt,
	})
}

// Get handles GET /api/v1/tasks/{taskID}.
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	t, err := h.service.Get(r.Context(), taskID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// CreatePR handles POST /api/v1/tasks/{taskID}/create-pr.
func (h *TaskHandler) CreatePR(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	var req task.CreatePRRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	result, err := h.prService.CreatePR(r.Context(), taskID, req)
	if err != nil {
		// Determine status code from error message
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			writeError(w, http.StatusNotFound, errMsg)
		case strings.Contains(errMsg, "must be in completed status"):
			writeError(w, http.StatusConflict, errMsg)
		case strings.Contains(errMsg, "no changes"):
			writeError(w, http.StatusBadRequest, errMsg)
		case strings.Contains(errMsg, "not supported"):
			writeError(w, http.StatusBadRequest, errMsg)
		default:
			writeError(w, http.StatusInternalServerError, errMsg)
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

func writeAppError(w http.ResponseWriter, err error) {
	status := apperror.HTTPStatus(err)
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, status, map[string]interface{}{
			"error":   http.StatusText(status),
			"message": appErr.Message,
			"fields":  appErr.Fields,
		})
		return
	}
	writeError(w, status, "internal server error")
}

func formatValidationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "field is required"
	case "url":
		return "must be a valid URL"
	case "max":
		return "exceeds maximum length"
	default:
		return "invalid value"
	}
}
