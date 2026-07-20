package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/schedule"
	"github.com/freema/codeforge/internal/session"
)

// ScheduleHandler manages recurring (cron) sessions. Operator-only.
type ScheduleHandler struct {
	store     *schedule.Store
	scheduler *schedule.Scheduler
}

// NewScheduleHandler creates a schedule handler.
func NewScheduleHandler(store *schedule.Store, scheduler *schedule.Scheduler) *ScheduleHandler {
	return &ScheduleHandler{store: store, scheduler: scheduler}
}

type scheduleRequest struct {
	Name           string          `json:"name"`
	Cron           string          `json:"cron"`
	Enabled        *bool           `json:"enabled"`
	SessionRequest json.RawMessage `json:"session_request"`
}

// validateSessionRequest ensures the stored template will actually produce a
// runnable session when the schedule fires.
func validateSessionRequest(raw json.RawMessage) error {
	var req session.CreateSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errors.New("session_request is not a valid session request object")
	}
	if req.RepoURL == "" {
		return errors.New("session_request.repo_url is required")
	}
	if req.Prompt == "" {
		return errors.New("session_request.prompt is required")
	}
	return nil
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
	if err := validateSessionRequest(req.SessionRequest); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sch := &schedule.Schedule{
		Name:           req.Name,
		Cron:           req.Cron,
		Enabled:        req.Enabled == nil || *req.Enabled,
		SessionRequest: req.SessionRequest,
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
	}
	if len(req.SessionRequest) > 0 {
		if err := validateSessionRequest(req.SessionRequest); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sch.SessionRequest = req.SessionRequest
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
func (h *ScheduleHandler) Run(w http.ResponseWriter, r *http.Request) {
	sch, err := h.store.Get(r.Context(), chi.URLParam(r, "scheduleID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	t, err := h.scheduler.Fire(r.Context(), sch, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"schedule_id": sch.ID,
		"session_id":  t.ID,
	})
}

func (h *ScheduleHandler) writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, schedule.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
