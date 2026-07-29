package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/blueprint"
	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/schedule"
	"github.com/freema/codeforge/internal/session"
)

// ScheduleHandler manages recurring (cron) sessions. Operator-only.
type ScheduleHandler struct {
	store      *schedule.Store
	scheduler  *schedule.Scheduler
	blueprints *blueprint.Store // optional — nil rejects blueprint-backed schedules
}

// NewScheduleHandler creates a schedule handler. blueprints may be nil, which
// rejects blueprint-backed schedules at validation time.
func NewScheduleHandler(store *schedule.Store, scheduler *schedule.Scheduler, blueprints *blueprint.Store) *ScheduleHandler {
	return &ScheduleHandler{store: store, scheduler: scheduler, blueprints: blueprints}
}

// RefChecker exposes the schedule store as the blueprint handler's
// ScheduleRefChecker (409 guard on deleting a referenced blueprint).
// Nil-safe so a nil handler yields a nil checker.
func (h *ScheduleHandler) RefChecker() ScheduleRefChecker {
	if h == nil {
		return nil
	}
	return h.store
}

type scheduleRequest struct {
	Name            string            `json:"name"`
	Cron            string            `json:"cron"`
	Enabled         *bool             `json:"enabled"`
	Timezone        *string           `json:"timezone"` // IANA name; pointer so PATCH can clear it
	SessionRequest  json.RawMessage   `json:"session_request"`
	BlueprintID     string            `json:"blueprint_id"`
	BlueprintParams map[string]string `json:"blueprint_params"`
}

// validateSessionRequest ensures the stored template will actually produce a
// runnable session when the schedule fires. This is the only validation gate:
// Scheduler.Fire calls session.Service.Create directly, bypassing the HTTP
// session handler checks.
func validateSessionRequest(raw json.RawMessage) error {
	var req session.CreateSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errors.New("session_request is not a valid session request object")
	}
	return validateSessionRequestFields(&req)
}

// validateSessionRequestFields applies the schedulability rules to a decoded
// session request — shared by the inline path and the rendered-blueprint
// dry run.
func validateSessionRequestFields(req *session.CreateSessionRequest) error {
	if req.RepoURL == "" {
		return errors.New("session_request.repo_url is required")
	}
	if req.SessionType != "" && !prompt.ValidSessionType(req.SessionType) {
		return fmt.Errorf("unknown session type: %s", req.SessionType)
	}
	if req.SessionType == "pr_review" {
		// A schedule cannot target a fixed PR number.
		return errors.New("pr_review sessions cannot be scheduled")
	}
	// Prompt drives code (the default) and plan sessions; for review and
	// knowledge it is an optional focus/instruction on top of the template.
	switch req.SessionType {
	case "", "code", "plan":
		if req.Prompt == "" {
			return errors.New("session_request.prompt is required")
		}
	}
	return nil
}

// resolveBlueprintMode validates a blueprint-backed schedule: the blueprint
// must exist (ref is an ID, name fallback for curl users), a dry-run
// blueprint.Build must succeed with the given params (missing-required and
// template errors surface here), and the RENDERED request must pass the same
// schedulability checks as an inline session_request.
func (h *ScheduleHandler) resolveBlueprintMode(ctx context.Context, ref string, params map[string]string) (*blueprint.Blueprint, error) {
	if h.blueprints == nil {
		return nil, errors.New("blueprint-backed schedules are not available")
	}
	b, err := h.blueprints.Get(ctx, ref)
	if errors.Is(err, blueprint.ErrNotFound) {
		b, err = h.blueprints.GetByName(ctx, ref)
	}
	if errors.Is(err, blueprint.ErrNotFound) {
		return nil, fmt.Errorf("blueprint %q not found", ref)
	}
	if err != nil {
		return nil, err
	}

	// Dry run: key resolution is a fire-time concern, so no key registry here.
	req, err := blueprint.Build(ctx, b, params, nil)
	if err != nil {
		return nil, err
	}
	if err := validateSessionRequestFields(req); err != nil {
		return nil, err
	}
	return b, nil
}

// Create handles POST /schedules.
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := schedule.ParseCron(req.Cron); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	timezone := ""
	if req.Timezone != nil {
		timezone = *req.Timezone
	}
	if err := schedule.ValidateTimezone(timezone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hasInline := len(req.SessionRequest) > 0
	hasBlueprint := req.BlueprintID != ""
	if hasInline == hasBlueprint {
		writeError(w, http.StatusBadRequest, "exactly one of session_request or blueprint_id is required")
		return
	}

	sch := &schedule.Schedule{
		Name:     req.Name,
		Cron:     req.Cron,
		Enabled:  req.Enabled == nil || *req.Enabled,
		Timezone: timezone,
	}
	if hasBlueprint {
		b, err := h.resolveBlueprintMode(r.Context(), req.BlueprintID, req.BlueprintParams)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.BlueprintID = b.ID
		sch.BlueprintParams = req.BlueprintParams
	} else {
		if err := validateSessionRequest(req.SessionRequest); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.SessionRequest = req.SessionRequest
	}

	if err := h.store.Create(r.Context(), sch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sch.FillNextRun(time.Now())
	writeJSON(w, http.StatusCreated, sch)
}

// List handles GET /schedules.
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	for _, sch := range items {
		sch.FillNextRun(now)
	}
	if items == nil {
		items = []*schedule.Schedule{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schedules": items})
}

// Get handles GET /schedules/{scheduleID}.
func (h *ScheduleHandler) Get(w http.ResponseWriter, r *http.Request) {
	sch, err := h.store.Get(r.Context(), chi.URLParam(r, "scheduleID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	sch.FillNextRun(time.Now())
	writeJSON(w, http.StatusOK, sch)
}

// Update handles PATCH /schedules/{scheduleID} — partial update.
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	sch, err := h.store.Get(r.Context(), chi.URLParam(r, "scheduleID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	var req scheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		sch.Name = req.Name
	}
	if req.Cron != "" {
		if _, err := schedule.ParseCron(req.Cron); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.Cron = req.Cron
	}
	if req.Enabled != nil {
		sch.Enabled = *req.Enabled
		if sch.Enabled {
			// A human re-enabling an auto-disabled schedule gets a clean slate.
			sch.DisabledReason = ""
			sch.ConsecutiveFailures = 0
		}
	}
	if req.Timezone != nil {
		if err := schedule.ValidateTimezone(*req.Timezone); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.Timezone = *req.Timezone
	}
	hasInline := len(req.SessionRequest) > 0
	hasBlueprint := req.BlueprintID != ""
	switch {
	case hasInline && hasBlueprint:
		writeError(w, http.StatusBadRequest, "only one of session_request or blueprint_id may be set")
		return
	case hasInline:
		if err := validateSessionRequest(req.SessionRequest); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Setting an inline request clears blueprint mode.
		sch.SessionRequest = req.SessionRequest
		sch.BlueprintID = ""
		sch.BlueprintParams = nil
	case hasBlueprint:
		// Setting a blueprint clears the inline request. Params are taken
		// from this request only (absent = none), never carried over.
		b, err := h.resolveBlueprintMode(r.Context(), req.BlueprintID, req.BlueprintParams)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.BlueprintID = b.ID
		sch.BlueprintParams = req.BlueprintParams
		sch.SessionRequest = nil
	case req.BlueprintParams != nil:
		// Params-only update for an already blueprint-backed schedule.
		if sch.BlueprintID == "" {
			writeError(w, http.StatusBadRequest, "blueprint_params requires a blueprint-backed schedule")
			return
		}
		if _, err := h.resolveBlueprintMode(r.Context(), sch.BlueprintID, req.BlueprintParams); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.BlueprintParams = req.BlueprintParams
	}

	if err := h.store.Update(r.Context(), sch); err != nil {
		h.writeStoreError(w, err)
		return
	}
	sch.FillNextRun(time.Now())
	writeJSON(w, http.StatusOK, sch)
}

// Delete handles DELETE /schedules/{scheduleID}.
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.Context(), chi.URLParam(r, "scheduleID")); err != nil {
		h.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run handles POST /schedules/{scheduleID}/run — fire immediately.
// Manual runs record run history and last_session_id but never shift the
// cron base (last_run_at), so the regular cadence is unaffected.
func (h *ScheduleHandler) Run(w http.ResponseWriter, r *http.Request) {
	sch, err := h.store.Get(r.Context(), chi.URLParam(r, "scheduleID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	t, err := h.scheduler.Fire(r.Context(), sch, time.Now(), schedule.TriggerManual)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"schedule_id": sch.ID,
		"session_id":  t.ID,
	})
}

// Runs handles GET /schedules/{scheduleID}/runs — run history, newest first.
func (h *ScheduleHandler) Runs(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleID")
	if _, err := h.store.Get(r.Context(), scheduleID); err != nil {
		h.writeStoreError(w, err)
		return
	}
	runs, err := h.store.ListRuns(r.Context(), scheduleID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []*schedule.Run{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

func (h *ScheduleHandler) writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, schedule.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
