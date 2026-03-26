package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/workflow"
)

// WorkflowHandler handles workflow-related HTTP endpoints.
type WorkflowHandler struct {
	registry workflow.Registry
	sessions PresetSessionCreator
	keys     keys.Registry
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(
	registry workflow.Registry,
	sessions PresetSessionCreator,
	keys keys.Registry,
) *WorkflowHandler {
	return &WorkflowHandler{
		registry: registry,
		sessions: sessions,
		keys:     keys,
	}
}

// CreateWorkflow handles POST /api/v1/workflows.
func (h *WorkflowHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var def workflow.WorkflowDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if def.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validName.MatchString(def.Name) {
		writeError(w, http.StatusBadRequest, "name must contain only alphanumeric characters, hyphens, and underscores")
		return
	}

	// User-created workflows are never builtin
	def.Builtin = false

	if err := h.registry.Create(r.Context(), def); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":    def.Name,
		"message": "workflow created",
	})
}

// ListWorkflows handles GET /api/v1/workflows.
func (h *WorkflowHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	defs, err := h.registry.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflows": defs,
	})
}

// GetWorkflow handles GET /api/v1/workflows/{name}.
func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}

	def, err := h.registry.Get(r.Context(), name)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, def)
}

// DeleteWorkflow handles DELETE /api/v1/workflows/{name}.
func (h *WorkflowHandler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}

	if err := h.registry.Delete(r.Context(), name); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "workflow deleted",
	})
}

// RunWorkflow handles POST /api/v1/workflows/{name}/run.
// Creates a session directly from the workflow definition.
func (h *WorkflowHandler) RunWorkflow(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}

	var body struct {
		Params map[string]string `json:"params"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if body.Params == nil {
		body.Params = make(map[string]string)
	}

	def, err := h.registry.Get(r.Context(), name)
	if err != nil {
		writeAppError(w, err)
		return
	}

	req, err := workflow.BuildSessionRequest(r.Context(), *def, body.Params, h.keys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess, err := h.sessions.Create(r.Context(), *req)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":    sess.ID,
		"workflow_name": name,
	})
}
