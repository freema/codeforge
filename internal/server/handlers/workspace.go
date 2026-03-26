package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/workspace"
)

// WorkspaceHandler handles workspace management endpoints.
type WorkspaceHandler struct {
	manager     *workspace.Manager
	sessionService *session.Service
}

// NewWorkspaceHandler creates a new workspace handler.
func NewWorkspaceHandler(manager *workspace.Manager, sessionService *session.Service) *WorkspaceHandler {
	return &WorkspaceHandler{manager: manager, sessionService: sessionService}
}

// List handles GET /api/v1/workspaces.
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.manager.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}

	type wsInfo struct {
		SessionID     string  `json:"session_id"`
		Path          string  `json:"path"`
		SizeMB        float64 `json:"size_mb"`
		CreatedAt     string  `json:"created_at"`
		ExpiresAt     string  `json:"expires_at"`
		SessionStatus string  `json:"session_status"`
	}

	var totalSize int64
	items := make([]wsInfo, 0, len(workspaces))
	for _, ws := range workspaces {
		totalSize += ws.SizeBytes

		status := "unknown"
		if t, err := h.sessionService.Get(r.Context(), ws.TaskID); err == nil {
			status = string(t.Status)
		}

		items = append(items, wsInfo{
			SessionID:     ws.TaskID,
			Path:          ws.Path,
			SizeMB:        float64(ws.SizeBytes) / (1024 * 1024),
			CreatedAt:     ws.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ExpiresAt:     ws.ExpiresAt().Format("2006-01-02T15:04:05Z"),
			SessionStatus: status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workspaces":    items,
		"total_size_mb": float64(totalSize) / (1024 * 1024),
		"total_count":   len(items),
	})
}

// Delete handles DELETE /api/v1/workspaces/{sessionID}.
func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	// Check if session is currently running
	if t, err := h.sessionService.Get(r.Context(), sessionID); err == nil {
		if t.Status == session.StatusRunning || t.Status == session.StatusCloning || t.Status == session.StatusCreatingPR {
			writeError(w, http.StatusConflict, "cannot delete workspace for a running session")
			return
		}
	}

	ws := h.manager.Get(r.Context(), sessionID)
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	if err := h.manager.Delete(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "workspace deleted",
	})
}
