package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/workflow"
)

// WorkflowConfigHandler handles workflow config CRUD and run endpoints.
type WorkflowConfigHandler struct {
	store      workflow.ConfigStore
	runCreator WorkflowRunCreator
}

// NewWorkflowConfigHandler creates a new workflow config handler.
func NewWorkflowConfigHandler(store workflow.ConfigStore, runCreator WorkflowRunCreator) *WorkflowConfigHandler {
	return &WorkflowConfigHandler{
		store:      store,
		runCreator: runCreator,
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
// It looks up the saved config and creates a workflow run using the orchestrator.
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

	// Inject timeout into params if configured
	params := cfg.Params
	if cfg.TimeoutSeconds > 0 {
		if params == nil {
			params = make(map[string]string)
		}
		params["_timeout_seconds"] = strconv.Itoa(cfg.TimeoutSeconds)
	}

	run, err := h.runCreator.CreateRun(r.Context(), cfg.Workflow, params)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"run_id":        run.ID,
		"workflow_name": run.WorkflowName,
		"config_id":     cfg.ID,
		"config_name":   cfg.Name,
		"status":        run.Status,
		"created_at":    run.CreatedAt,
	})
}
