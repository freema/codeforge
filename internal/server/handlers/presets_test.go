package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/preset"
)

// newPresetHandler builds a PresetHandler over an in-memory SQLite store.
// The session handler is nil — tests here never reach the create-session path.
func newPresetHandler(t *testing.T) *PresetHandler {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPresetHandler(preset.NewService(preset.NewStore(db)), nil)
}

func presetRouter(h *PresetHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/presets", h.Create)
	r.Get("/api/v1/presets", h.List)
	r.Get("/api/v1/presets/{presetID}", h.Get)
	r.Put("/api/v1/presets/{presetID}", h.Update)
	r.Delete("/api/v1/presets/{presetID}", h.Delete)
	r.Post("/api/v1/presets/{presetID}/run", h.Run)
	return r
}

func TestPresetHandler_CreateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid preset",
			body:       `{"name":"deps-bump","description":"weekly","request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid preset without prompt",
			body:       `{"name":"no-prompt","request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "empty name",
			body:       `{"name":"","request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing name",
			body:       `{"request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid body JSON",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing request template",
			body:       `{"name":"deps-bump"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "request template not an object",
			body:       `{"name":"deps-bump","request":"not-an-object"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "request template without repo_url",
			body:       `{"name":"deps-bump","request":{"prompt":"update deps"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := presetRouter(newPresetHandler(t))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/presets", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestPresetHandler_Create_DuplicateName(t *testing.T) {
	r := presetRouter(newPresetHandler(t))
	body := `{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git"}}`

	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/presets", strings.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("create #%d: got %d, want %d, body: %s", i+1, w.Code, want, w.Body.String())
		}
	}
}

func TestPresetHandler_CRUDRoundTrip(t *testing.T) {
	r := presetRouter(newPresetHandler(t))

	// Create
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/presets",
		strings.NewReader(`{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body: %s", w.Code, w.Body.String())
	}
	var created preset.Preset
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// Get
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/presets/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d, body: %s", w.Code, w.Body.String())
	}

	// Update (PUT, full replacement) — invalid template rejected
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/presets/"+created.ID,
		strings.NewReader(`{"name":"deps-bump","request":{"prompt":"no repo url"}}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update invalid template: got %d, want 400, body: %s", w.Code, w.Body.String())
	}

	// Update — valid
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/presets/"+created.ID,
		strings.NewReader(`{"name":"deps-bump-v2","description":"weekly","request":{"repo_url":"https://example.com/org/repo.git"}}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d, body: %s", w.Code, w.Body.String())
	}
	var updated preset.Preset
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "deps-bump-v2" || updated.Description != "weekly" {
		t.Errorf("update result = %+v, want replaced fields", updated)
	}

	// List
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/presets", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var listResp struct {
		Presets []preset.Preset `json:"presets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Presets) != 1 {
		t.Errorf("list = %d presets, want 1", len(listResp.Presets))
	}

	// Delete
	req = httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/presets/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d, body: %s", w.Code, w.Body.String())
	}

	// Get after delete
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/presets/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: got %d, want 404", w.Code)
	}
}

func TestPresetHandler_Run_Errors(t *testing.T) {
	h := newPresetHandler(t)
	r := presetRouter(h)

	// Unknown preset → 404
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/presets/missing/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("run missing preset: got %d, want 404", w.Code)
	}

	// Invalid override body → 400 (before any session creation)
	if err := h.service.Create(context.Background(), &preset.Preset{
		ID:      "p1",
		Name:    "deps-bump",
		Request: json.RawMessage(`{"repo_url":"https://example.com/org/repo.git"}`),
	}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/presets/p1/run", strings.NewReader(`{invalid}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("run invalid body: got %d, want 400, body: %s", w.Code, w.Body.String())
	}
}

func TestMergePresetRequest(t *testing.T) {
	template := `{"repo_url":"https://example.com/org/repo.git","prompt":"template prompt","session_type":"feature","config":{"cli":"claude-code"}}`

	tests := []struct {
		name       string
		template   string
		prompt     string
		wantPrompt string
		wantErr    bool
	}{
		{
			name:       "no override keeps template prompt",
			template:   template,
			prompt:     "",
			wantPrompt: "template prompt",
		},
		{
			name:       "override replaces template prompt",
			template:   template,
			prompt:     "run-time prompt",
			wantPrompt: "run-time prompt",
		},
		{
			name:       "override fills empty template prompt",
			template:   `{"repo_url":"https://example.com/org/repo.git"}`,
			prompt:     "run-time prompt",
			wantPrompt: "run-time prompt",
		},
		{
			name:     "invalid template errors",
			template: `{not json`,
			prompt:   "x",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := mergePresetRequest(json.RawMessage(tt.template), tt.prompt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("mergePresetRequest: %v", err)
			}
			if req.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tt.wantPrompt)
			}
			// Non-overridden template fields survive the merge.
			if req.RepoURL != "https://example.com/org/repo.git" {
				t.Errorf("RepoURL = %q, want template value", req.RepoURL)
			}
			if tt.template == template {
				if req.SessionType != "feature" {
					t.Errorf("SessionType = %q, want feature", req.SessionType)
				}
				if req.Config == nil || req.Config.CLI != "claude-code" {
					t.Errorf("Config = %+v, want cli claude-code", req.Config)
				}
			}
		})
	}
}
