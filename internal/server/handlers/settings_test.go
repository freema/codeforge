package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/settings"
	"github.com/freema/codeforge/internal/tool/runner"
)

func newTestSettingsHandler(cfg config.CodeReviewConfig) *SettingsHandler {
	registry := runner.NewRegistry("claude-code")
	registry.Register("claude-code", runner.NewClaudeRunner("claude"), runner.RunnerMeta{AIProvider: "anthropic"})
	registry.Register("codex", runner.NewCodexRunner("codex"), runner.RunnerMeta{AIProvider: "openai"})
	// store is nil-redis-free only for validation paths; UpdateReview rejects
	// invalid input before touching the store.
	return NewSettingsHandler(nil, registry, cfg)
}

func TestSettingsHandler_UpdateReview_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "unknown CLI rejected",
			body:       `{"default_cli":"not-a-cli","default_model":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON rejected",
			body:       `{not-json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestSettingsHandler(config.CodeReviewConfig{})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/settings/review", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.UpdateReview(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestSettingsHandler_ReviewResponse_FallbackChain(t *testing.T) {
	tests := []struct {
		name          string
		cfgDefaultCLI string
		stored        settings.ReviewSettings
		wantCLI       string
		wantModel     string
	}{
		{
			name:          "settings override wins",
			cfgDefaultCLI: "claude-code",
			stored:        settings.ReviewSettings{DefaultCLI: "codex", DefaultModel: "gpt-5.2-codex"},
			wantCLI:       "codex",
			wantModel:     "gpt-5.2-codex",
		},
		{
			name:          "empty settings fall back to config",
			cfgDefaultCLI: "codex",
			stored:        settings.ReviewSettings{},
			wantCLI:       "codex",
			wantModel:     "",
		},
		{
			name:          "empty settings and config fall back to built-in default",
			cfgDefaultCLI: "",
			stored:        settings.ReviewSettings{},
			wantCLI:       defaultReviewCLI,
			wantModel:     "",
		},
		{
			name:          "model comes from settings only",
			cfgDefaultCLI: "claude-code",
			stored:        settings.ReviewSettings{DefaultModel: "claude-sonnet-4-6"},
			wantCLI:       "claude-code",
			wantModel:     "claude-sonnet-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestSettingsHandler(config.CodeReviewConfig{DefaultCLI: tt.cfgDefaultCLI})

			resp := h.reviewResponse(tt.stored)
			if resp.EffectiveCLI != tt.wantCLI {
				t.Errorf("EffectiveCLI = %q, want %q", resp.EffectiveCLI, tt.wantCLI)
			}
			if resp.EffectiveModel != tt.wantModel {
				t.Errorf("EffectiveModel = %q, want %q", resp.EffectiveModel, tt.wantModel)
			}
			if resp.DefaultCLI != tt.stored.DefaultCLI || resp.DefaultModel != tt.stored.DefaultModel {
				t.Errorf("stored values not echoed back: got %+v", resp)
			}
		})
	}
}
