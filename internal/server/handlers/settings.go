package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/settings"
	"github.com/freema/codeforge/internal/tool/runner"
)

// SettingsHandler handles runtime settings endpoints (operator-only).
type SettingsHandler struct {
	store       *settings.Store
	cliRegistry *runner.Registry
	cfg         config.CodeReviewConfig
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(store *settings.Store, cliRegistry *runner.Registry, cfg config.CodeReviewConfig) *SettingsHandler {
	return &SettingsHandler{store: store, cliRegistry: cliRegistry, cfg: cfg}
}

// reviewSettingsResponse is the stored override plus the effective values
// after the fallback chain, so the UI can show what actually applies.
type reviewSettingsResponse struct {
	DefaultCLI     string `json:"default_cli"`
	DefaultModel   string `json:"default_model"`
	EffectiveCLI   string `json:"effective_cli"`
	EffectiveModel string `json:"effective_model"`
}

// reviewResponse computes the effective values: runtime settings →
// code_review.default_cli config → built-in default. The model comes from
// runtime settings only (empty = the CLI's own default).
func (h *SettingsHandler) reviewResponse(rs settings.ReviewSettings) reviewSettingsResponse {
	effectiveCLI := rs.DefaultCLI
	if effectiveCLI == "" {
		effectiveCLI = h.cfg.DefaultCLI
	}
	if effectiveCLI == "" {
		effectiveCLI = defaultReviewCLI
	}

	return reviewSettingsResponse{
		DefaultCLI:     rs.DefaultCLI,
		DefaultModel:   rs.DefaultModel,
		EffectiveCLI:   effectiveCLI,
		EffectiveModel: rs.DefaultModel,
	}
}

// GetReview handles GET /api/v1/settings/review.
func (h *SettingsHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	rs, err := h.store.GetReview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load review settings")
		return
	}
	writeJSON(w, http.StatusOK, h.reviewResponse(rs))
}

// UpdateReview handles PUT /api/v1/settings/review.
// Empty strings clear the runtime override (fall back to config).
func (h *SettingsHandler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	var req settings.ReviewSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate CLI name against registry — same rule as POST /sessions
	// config.cli. Empty string = clear the override.
	if req.DefaultCLI != "" {
		if _, err := h.cliRegistry.Get(req.DefaultCLI); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": map[string]string{"default_cli": fmt.Sprintf("unknown CLI: %s", req.DefaultCLI)},
			})
			return
		}
	}

	if err := h.store.SetReview(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store review settings")
		return
	}

	writeJSON(w, http.StatusOK, h.reviewResponse(req))
}
