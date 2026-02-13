package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/workspace"
)

// WorkspaceHandler handles workspace management endpoints.
type WorkspaceHandler struct {
	manager     *workspace.Manager
	taskService *task.Service
}

// NewWorkspaceHandler creates a new workspace handler.
func NewWorkspaceHandler(manager *workspace.Manager, taskService *task.Service) *WorkspaceHandler {
	return &WorkspaceHandler{manager: manager, taskService: taskService}
}

// List handles GET /api/v1/workspaces.
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.manager.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}

	type wsInfo struct {
		TaskID     string  `json:"task_id"`
		SizeMB     float64 `json:"size_mb"`
		CreatedAt  string  `json:"created_at"`
		ExpiresAt  string  `json:"expires_at"`
		TaskStatus string  `json:"task_status"`
	}

	var totalSize int64
	items := make([]wsInfo, 0, len(workspaces))
	for _, ws := range workspaces {
		totalSize += ws.SizeBytes

		status := "unknown"
		if t, err := h.taskService.Get(r.Context(), ws.TaskID); err == nil {
			status = string(t.Status)
		}

		items = append(items, wsInfo{
			TaskID:     ws.TaskID,
			SizeMB:     float64(ws.SizeBytes) / (1024 * 1024),
			CreatedAt:  ws.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ExpiresAt:  ws.ExpiresAt().Format("2006-01-02T15:04:05Z"),
			TaskStatus: status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workspaces":  items,
		"total_size_mb": float64(totalSize) / (1024 * 1024),
		"total_count":  len(items),
	})
}

// Delete handles DELETE /api/v1/workspaces/{taskID}.
func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	// Check if task is currently running
	if t, err := h.taskService.Get(r.Context(), taskID); err == nil {
		if t.Status == task.StatusRunning || t.Status == task.StatusCloning || t.Status == task.StatusCreatingPR {
			writeError(w, http.StatusConflict, "cannot delete workspace for a running task")
			return
		}
	}

	ws := h.manager.Get(r.Context(), taskID)
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	if err := h.manager.Delete(r.Context(), taskID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "workspace deleted",
	})
}
