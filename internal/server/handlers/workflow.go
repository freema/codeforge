package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/workflow"
)

// WorkflowRunCreator creates workflow runs. Satisfied by *workflow.Orchestrator.
type WorkflowRunCreator interface {
	CreateRun(ctx context.Context, workflowName string, params map[string]string) (*workflow.WorkflowRun, error)
}

// WorkflowHandler handles workflow-related HTTP endpoints.
type WorkflowHandler struct {
	registry   workflow.Registry
	runStore   workflow.RunStore
	runCreator WorkflowRunCreator
	redis      *redisclient.Client
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(registry workflow.Registry, runStore workflow.RunStore, runCreator WorkflowRunCreator, redis *redisclient.Client) *WorkflowHandler {
	return &WorkflowHandler{
		registry:   registry,
		runStore:   runStore,
		runCreator: runCreator,
		redis:      redis,
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
func (h *WorkflowHandler) RunWorkflow(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}

	var req struct {
		Params map[string]string `json:"params"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Params == nil {
		req.Params = make(map[string]string)
	}

	run, err := h.runCreator.CreateRun(r.Context(), name, req.Params)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"run_id":        run.ID,
		"workflow_name": run.WorkflowName,
		"status":        run.Status,
		"created_at":    run.CreatedAt,
	})
}

// GetRun handles GET /api/v1/workflow-runs/{runID}.
func (h *WorkflowHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	run, err := h.runStore.GetRun(r.Context(), runID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	// Load steps
	steps, err := h.runStore.GetSteps(r.Context(), runID)
	if err == nil {
		run.Steps = steps
	}

	writeJSON(w, http.StatusOK, run)
}

// StreamRun handles GET /api/v1/workflow-runs/{runID}/stream.
// Streams workflow events via Server-Sent Events (SSE).
func (h *WorkflowHandler) StreamRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	run, err := h.runStore.GetRun(r.Context(), runID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	flush := func() { flusher.Flush() }

	isTerminal := run.Status == workflow.RunStatusCompleted || run.Status == workflow.RunStatusFailed

	streamKey := h.redis.Key("workflow", runID, "stream")
	doneKey := h.redis.Key("workflow", runID, "done")

	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()

	var msgCh <-chan *redis.Message
	if !isTerminal {
		pubsub := h.redis.Unwrap().Subscribe(subCtx, streamKey, doneKey)
		defer pubsub.Close()
		msgCh = pubsub.Channel()
	}

	// Send connected event
	writeSSE(w, "connected", map[string]interface{}{
		"run_id": run.ID,
		"status": run.Status,
	})
	flush()

	// Replay history
	historyKey := h.redis.Key("workflow", runID, "history")
	history, err := h.redis.Unwrap().LRange(r.Context(), historyKey, 0, -1).Result()
	if err == nil && len(history) > 0 {
		for _, msg := range history {
			fmt.Fprintf(w, "data: %s\n\n", msg)
		}
		flush()
	}

	// For terminal runs, send done and close
	if isTerminal {
		writeSSE(w, "done", map[string]interface{}{
			"run_id": run.ID,
			"status": run.Status,
		})
		flush()
		return
	}

	// Stream live events
	maxDuration := 30 * time.Minute
	deadline := time.After(maxDuration)
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))

		select {
		case <-r.Context().Done():
			return

		case <-deadline:
			writeSSE(w, "timeout", map[string]string{
				"message": "stream closed after 30 minutes",
			})
			flush()
			return

		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flush()

		case msg, ok := <-msgCh:
			if !ok {
				return
			}

			if msg.Channel == doneKey {
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", msg.Payload)
				flush()
				return
			}

			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flush()
		}
	}
}

// ListRuns handles GET /api/v1/workflow-runs.
func (h *WorkflowHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	workflowName := r.URL.Query().Get("workflow")

	runs, err := h.runStore.ListRuns(r.Context(), workflowName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs": runs,
	})
}

// OtelBypassPath checks if a path needs SSE bypass.
func OtelBypassWorkflowStream(path string) bool {
	return strings.HasSuffix(path, "/stream") && strings.Contains(path, "/workflow-runs/")
}
