package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/workflow"
)

// PresetSessionCreator creates sessions. Satisfied by *session.Service.
type PresetSessionCreator interface {
	Create(ctx context.Context, req session.CreateSessionRequest) (*session.Session, error)
}

// WorkflowConfigHandler handles workflow config CRUD and run endpoints.
type WorkflowConfigHandler struct {
	store    workflow.ConfigStore
	registry workflow.Registry
	sessions PresetSessionCreator
	keys     keys.Registry
}

// NewWorkflowConfigHandler creates a new workflow config handler.
func NewWorkflowConfigHandler(store workflow.ConfigStore, registry workflow.Registry, sessions PresetSessionCreator, keys keys.Registry) *WorkflowConfigHandler {
	return &WorkflowConfigHandler{
		store:    store,
		registry: registry,
		sessions: sessions,
		keys:     keys,
	}
}

// Create handles POST /api/v1/workflow-configs.
func (h *WorkflowConfigHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string            `json:"name"`
		Workflow       string            `json:"workflow"`
		Params         map[string]string `json:"params"`
		TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" || req.Workflow == "" {
		writeError(w, http.StatusBadRequest, "name and workflow are required")
		return
	}
	if !validName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must contain only alphanumeric characters, hyphens, and underscores")
		return
	}
	if req.Params == nil {
		req.Params = make(map[string]string)
	}

	cfg := workflow.WorkflowConfig{
		Name:           req.Name,
		Workflow:       req.Workflow,
		Params:         req.Params,
		TimeoutSeconds: req.TimeoutSeconds,
	}

	id, err := h.store.Create(r.Context(), cfg)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"name":    req.Name,
		"message": "workflow config created",
	})
}

// List handles GET /api/v1/workflow-configs.
func (h *WorkflowConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	configs, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflow configs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configs": configs,
	})
}

// Get handles GET /api/v1/workflow-configs/{id}.
func (h *WorkflowConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid config ID")
		return
	}

	cfg, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Delete handles DELETE /api/v1/workflow-configs/{id}.
func (h *WorkflowConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid config ID")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "workflow config deleted",
	})
}

// Run handles POST /api/v1/workflow-configs/{id}/run.
// It looks up the saved config, builds a session request from the workflow
// definition, and creates a session directly.
func (h *WorkflowConfigHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid config ID")
		return
	}

	cfg, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}

	// Lookup workflow definition (template)
	def, err := h.registry.Get(r.Context(), cfg.Workflow)
	if err != nil {
		writeAppError(w, err)
		return
	}

	// Inject timeout into params if configured
	params := cfg.Params
	if cfg.TimeoutSeconds > 0 {
		if params == nil {
			params = make(map[string]string)
		}
		params["_timeout_seconds"] = strconv.Itoa(cfg.TimeoutSeconds)
	}

	// Build session request from workflow definition + params
	req, err := workflow.BuildSessionRequest(r.Context(), *def, params, h.keys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create session directly
	sess, err := h.sessions.Create(r.Context(), *req)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":  sess.ID,
		"config_id":   cfg.ID,
		"config_name": cfg.Name,
	})
}
