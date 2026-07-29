package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/blueprint"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/session"
)

// sessionRunner runs a fully-built session request through the normal
// create-session path so blueprint runs get identical validation and tenant
// enforcement to POST /sessions. Satisfied by *SessionHandler.
type sessionRunner interface {
	createSession(w http.ResponseWriter, r *http.Request, req session.CreateSessionRequest)
}

// ScheduleRefChecker reports how many schedules reference a blueprint.
// Wired in by the schedules subsystem; nil disables the referential check.
type ScheduleRefChecker interface {
	ListByBlueprint(ctx context.Context, blueprintID string) (int, error)
}

// BlueprintHandler manages blueprints: named, reusable session request
// templates with optional parameters. Operator-only. Also serves the
// deprecated /presets alias routes.
type BlueprintHandler struct {
	store     *blueprint.Store
	keys      keys.Registry
	sessions  sessionRunner
	schedules ScheduleRefChecker
}

// NewBlueprintHandler creates a blueprint handler. schedules may be nil,
// which disables the schedule-reference check on delete.
func NewBlueprintHandler(store *blueprint.Store, keyReg keys.Registry, sessions *SessionHandler, schedules ScheduleRefChecker) *BlueprintHandler {
	return &BlueprintHandler{store: store, keys: keyReg, sessions: sessions, schedules: schedules}
}

type blueprintRequest struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Request     json.RawMessage                 `json:"request"`
	Parameters  []blueprint.ParameterDefinition `json:"parameters"`
}

// validateBlueprintTemplate ensures the stored template is a session request
// that can produce a runnable session. Prompt is intentionally NOT required —
// the run endpoint can supply it as an override. String values may contain
// {{.Params.x}} template expressions, which pass through as opaque strings.
func validateBlueprintTemplate(raw json.RawMessage) error {
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

// validateBlueprintParameters checks parameter declarations: non-empty,
// unique names.
func validateBlueprintParameters(params []blueprint.ParameterDefinition) error {
	seen := make(map[string]struct{}, len(params))
	for _, p := range params {
		if p.Name == "" {
			return errors.New("parameter name is required")
		}
		if _, dup := seen[p.Name]; dup {
			return fmt.Errorf("duplicate parameter name %q", p.Name)
		}
		seen[p.Name] = struct{}{}
	}
	return nil
}

func validateBlueprintRequest(req *blueprintRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if !validName.MatchString(req.Name) {
		return errors.New("name must contain only alphanumeric characters, hyphens, and underscores")
	}
	if err := validateBlueprintTemplate(req.Request); err != nil {
		return err
	}
	return validateBlueprintParameters(req.Parameters)
}

// Create handles POST /api/v1/blueprints.
func (h *BlueprintHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req blueprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.create(w, r, req)
}

// create is the shared create path for /blueprints and the /presets alias.
func (h *BlueprintHandler) create(w http.ResponseWriter, r *http.Request, req blueprintRequest) {
	if err := validateBlueprintRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Parameters == nil {
		req.Parameters = []blueprint.ParameterDefinition{}
	}

	b := &blueprint.Blueprint{
		Name:        req.Name,
		Description: req.Description,
		Builtin:     false, // user-created blueprints are never builtin
		Request:     req.Request,
		Parameters:  req.Parameters,
	}
	if err := h.store.Create(r.Context(), b); err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// List handles GET /api/v1/blueprints.
func (h *BlueprintHandler) List(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "blueprints")
}

func (h *BlueprintHandler) list(w http.ResponseWriter, r *http.Request, envelopeKey string) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*blueprint.Blueprint{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{envelopeKey: items})
}

// Get handles GET /api/v1/blueprints/{blueprintID}.
func (h *BlueprintHandler) Get(w http.ResponseWriter, r *http.Request) {
	b, err := h.lookup(r.Context(), chi.URLParam(r, "blueprintID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// Update handles PUT /api/v1/blueprints/{blueprintID} — full replacement.
// Built-in blueprints cannot be modified.
func (h *BlueprintHandler) Update(w http.ResponseWriter, r *http.Request) {
	b, err := h.lookup(r.Context(), chi.URLParam(r, "blueprintID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if b.Builtin {
		writeError(w, http.StatusBadRequest, "built-in blueprints cannot be modified")
		return
	}

	var req blueprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateBlueprintRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Parameters == nil {
		req.Parameters = []blueprint.ParameterDefinition{}
	}

	b.Name = req.Name
	b.Description = req.Description
	b.Request = req.Request
	b.Parameters = req.Parameters
	if err := h.store.Update(r.Context(), b); err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// Delete handles DELETE /api/v1/blueprints/{blueprintID}. Built-in blueprints
// cannot be deleted; blueprints referenced by schedules are refused with 409.
func (h *BlueprintHandler) Delete(w http.ResponseWriter, r *http.Request) {
	b, err := h.lookup(r.Context(), chi.URLParam(r, "blueprintID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if b.Builtin {
		writeError(w, http.StatusBadRequest, "built-in blueprints cannot be deleted")
		return
	}

	if h.schedules != nil {
		n, err := h.schedules.ListByBlueprint(r.Context(), b.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n > 0 {
			writeError(w, http.StatusConflict, fmt.Sprintf("blueprint is referenced by %d schedule(s)", n))
			return
		}
	}

	if err := h.store.Delete(r.Context(), b.ID); err != nil {
		h.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run handles POST /api/v1/blueprints/{blueprintID}/run — merge params,
// render the request template, and create a session through the normal
// create path. An optional non-empty prompt overrides the rendered prompt.
func (h *BlueprintHandler) Run(w http.ResponseWriter, r *http.Request) {
	b, err := h.lookup(r.Context(), chi.URLParam(r, "blueprintID"))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	var body struct {
		Params map[string]string `json:"params"`
		Prompt string            `json:"prompt"`
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

	req, err := blueprint.Build(r.Context(), b, body.Params, h.keys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Prompt != "" {
		req.Prompt = body.Prompt
	}

	h.sessions.createSession(w, r, *req)
}

// CreatePreset handles POST /api/v1/presets — deprecated alias accepting the
// old preset shape (no parameters). Creates a blueprint with parameters=[].
func (h *BlueprintHandler) CreatePreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Request     json.RawMessage `json:"request"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.create(w, r, blueprintRequest{
		Name:        req.Name,
		Description: req.Description,
		Request:     req.Request,
	})
}

// ListPresets handles GET /api/v1/presets — deprecated alias listing the
// same blueprints under the old "presets" envelope key.
func (h *BlueprintHandler) ListPresets(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "presets")
}

// lookup resolves a blueprint by ID, falling back to a name lookup so
// curl users can address blueprints by their unique name.
func (h *BlueprintHandler) lookup(ctx context.Context, ref string) (*blueprint.Blueprint, error) {
	b, err := h.store.Get(ctx, ref)
	if errors.Is(err, blueprint.ErrNotFound) {
		return h.store.GetByName(ctx, ref)
	}
	return b, err
}

func (h *BlueprintHandler) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blueprint.ErrNotFound):
		writeError(w, http.StatusNotFound, "blueprint not found")
	case errors.Is(err, blueprint.ErrNameTaken):
		writeError(w, http.StatusConflict, "blueprint name already exists")
	case errors.Is(err, blueprint.ErrBuiltinImmutable):
		writeError(w, http.StatusBadRequest, "built-in blueprints cannot be modified")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
