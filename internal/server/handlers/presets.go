package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/preset"
	"github.com/freema/codeforge/internal/session"
)

// PresetHandler manages saved, reusable session configurations. Operator-only.
type PresetHandler struct {
	service *preset.Service
	// sessions runs presets through the normal create-session path so preset
	// runs get identical validation to POST /sessions.
	sessions *SessionHandler
}

// NewPresetHandler creates a preset handler.
func NewPresetHandler(service *preset.Service, sessions *SessionHandler) *PresetHandler {
	return &PresetHandler{service: service, sessions: sessions}
}

type presetRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Request     json.RawMessage `json:"request"`
}

// validatePresetTemplate ensures the stored template is a session request that
// can produce a runnable session. Prompt is intentionally NOT required — the
// run endpoint can supply it as an override.
func validatePresetTemplate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("request is required")
	}
	var req session.CreateSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errors.New("request is not a valid session request object")
	}
	if req.RepoURL == "" {
		return errors.New("request.repo_url is required")
	}
	return nil
}

// Create handles POST /presets.
func (h *PresetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req presetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validatePresetTemplate(req.Request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p := &preset.Preset{
		Name:        req.Name,
		Description: req.Description,
		Request:     req.Request,
	}
	if err := h.service.Create(r.Context(), p); err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// List handles GET /presets.
func (h *PresetHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*preset.Preset{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"presets": items})
}

// Get handles GET /presets/{presetID}.
func (h *PresetHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.Get(r.Context(), chi.URLParam(r, "presetID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Update handles PUT /presets/{presetID} — full replacement.
func (h *PresetHandler) Update(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.Get(r.Context(), chi.URLParam(r, "presetID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	var req presetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validatePresetTemplate(req.Request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p.Name = req.Name
	p.Description = req.Description
	p.Request = req.Request
	if err := h.service.Update(r.Context(), p); err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Delete handles DELETE /presets/{presetID}.
func (h *PresetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "presetID")); err != nil {
		h.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run handles POST /presets/{presetID}/run — create a session from the preset,
// with an optional {"prompt": "..."} override merged into the template. The
// merged request goes through the same validation as POST /sessions.
func (h *PresetHandler) Run(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.Get(r.Context(), chi.URLParam(r, "presetID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	var overrides struct {
		Prompt string `json:"prompt"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&overrides); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	req, err := mergePresetRequest(p.Request, overrides.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.sessions.createSession(w, r, req)
}

// mergePresetRequest unmarshals the stored template and applies run-time
// overrides (a non-empty prompt replaces the template's prompt).
func mergePresetRequest(template json.RawMessage, prompt string) (session.CreateSessionRequest, error) {
	var req session.CreateSessionRequest
	if err := json.Unmarshal(template, &req); err != nil {
		return req, fmt.Errorf("stored preset template is invalid: %w", err)
	}
	if prompt != "" {
		req.Prompt = prompt
	}
	return req, nil
}

func (h *PresetHandler) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, preset.ErrNotFound):
		writeError(w, http.StatusNotFound, "preset not found")
	case errors.Is(err, preset.ErrNameTaken):
		writeError(w, http.StatusConflict, "preset name already exists")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
