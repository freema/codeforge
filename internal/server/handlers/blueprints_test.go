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

	"github.com/freema/codeforge/internal/blueprint"
	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/session"
)

// fakeSessionRunner captures the request that a blueprint run built and
// responds the way the real create path does on success.
type fakeSessionRunner struct {
	req *session.CreateSessionRequest
}

func (f *fakeSessionRunner) createSession(w http.ResponseWriter, _ *http.Request, req session.CreateSessionRequest) {
	f.req = &req
	writeJSON(w, http.StatusCreated, map[string]string{"id": "session-1"})
}

// fakeScheduleRefChecker returns a fixed schedule reference count.
type fakeScheduleRefChecker struct {
	count int
}

func (f *fakeScheduleRefChecker) ListByBlueprint(context.Context, string) (int, error) {
	return f.count, nil
}

// newBlueprintHandler builds a BlueprintHandler over an in-memory SQLite
// store with a fake session runner. The key registry is nil — blueprint.Build
// skips tool key resolution when no registry is present.
func newBlueprintHandler(t *testing.T) (*BlueprintHandler, *fakeSessionRunner) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	runner := &fakeSessionRunner{}
	return &BlueprintHandler{store: blueprint.NewStore(db), sessions: runner}, runner
}

func blueprintRouter(h *BlueprintHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/blueprints", h.Create)
	r.Get("/api/v1/blueprints", h.List)
	r.Get("/api/v1/blueprints/{blueprintID}", h.Get)
	r.Put("/api/v1/blueprints/{blueprintID}", h.Update)
	r.Delete("/api/v1/blueprints/{blueprintID}", h.Delete)
	r.Post("/api/v1/blueprints/{blueprintID}/run", h.Run)
	// Deprecated /presets alias — same handler, old envelope/create shape.
	r.Post("/api/v1/presets", h.CreatePreset)
	r.Get("/api/v1/presets", h.ListPresets)
	r.Get("/api/v1/presets/{blueprintID}", h.Get)
	r.Put("/api/v1/presets/{blueprintID}", h.Update)
	r.Delete("/api/v1/presets/{blueprintID}", h.Delete)
	r.Post("/api/v1/presets/{blueprintID}/run", h.Run)
	return r
}

func doJSON(t *testing.T, r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBlueprintHandler_CreateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid blueprint without parameters",
			body:       `{"name":"deps-bump","description":"weekly","request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid blueprint with parameters",
			body:       `{"name":"issue-fixer","request":{"repo_url":"https://example.com/org/repo.git","prompt":"fix {{.Params.issue}}"},"parameters":[{"name":"issue","required":true}]}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid blueprint without prompt",
			body:       `{"name":"no-prompt","request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "empty name",
			body:       `{"name":"","request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid name characters",
			body:       `{"name":"bad name!","request":{"repo_url":"https://example.com/org/repo.git"}}`,
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
		{
			name:       "parameter with empty name",
			body:       `{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git"},"parameters":[{"name":""}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate parameter names",
			body:       `{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git"},"parameters":[{"name":"env"},{"name":"env"}]}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newBlueprintHandler(t)
			w := doJSON(t, blueprintRouter(h), http.MethodPost, "/api/v1/blueprints", tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestBlueprintHandler_Create_DuplicateName(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	r := blueprintRouter(h)
	body := `{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git"}}`

	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		w := doJSON(t, r, http.MethodPost, "/api/v1/blueprints", body)
		if w.Code != want {
			t.Fatalf("create #%d: got %d, want %d, body: %s", i+1, w.Code, want, w.Body.String())
		}
	}
}

func TestBlueprintHandler_CRUDRoundTrip(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	r := blueprintRouter(h)

	// Create
	w := doJSON(t, r, http.MethodPost, "/api/v1/blueprints",
		`{"name":"deps-bump","request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"},"parameters":[{"name":"env","default":"prod"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body: %s", w.Code, w.Body.String())
	}
	var created blueprint.Blueprint
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("expected blueprint id in create response")
	}
	if created.Builtin {
		t.Error("user-created blueprint must not be builtin")
	}

	// Get by ID
	w = doJSON(t, r, http.MethodGet, "/api/v1/blueprints/"+created.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get by id: got %d, body: %s", w.Code, w.Body.String())
	}

	// Get by name (lookup fallback for curl UX)
	w = doJSON(t, r, http.MethodGet, "/api/v1/blueprints/deps-bump", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get by name: got %d, body: %s", w.Code, w.Body.String())
	}

	// Update (PUT, full replacement) — invalid template rejected
	w = doJSON(t, r, http.MethodPut, "/api/v1/blueprints/"+created.ID,
		`{"name":"deps-bump","request":{"prompt":"no repo url"}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update invalid template: got %d, want 400, body: %s", w.Code, w.Body.String())
	}

	// Update — valid, replaces name/description/parameters
	w = doJSON(t, r, http.MethodPut, "/api/v1/blueprints/"+created.ID,
		`{"name":"deps-bump-v2","description":"weekly","request":{"repo_url":"https://example.com/org/repo.git"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d, body: %s", w.Code, w.Body.String())
	}
	var updated blueprint.Blueprint
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "deps-bump-v2" || updated.Description != "weekly" {
		t.Errorf("update result = %+v, want replaced fields", updated)
	}
	if len(updated.Parameters) != 0 {
		t.Errorf("update result parameters = %+v, want full replacement with []", updated.Parameters)
	}

	// List — "blueprints" envelope
	w = doJSON(t, r, http.MethodGet, "/api/v1/blueprints", "")
	var listResp struct {
		Blueprints []blueprint.Blueprint `json:"blueprints"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Blueprints) != 1 {
		t.Errorf("list = %d blueprints, want 1", len(listResp.Blueprints))
	}

	// Delete
	w = doJSON(t, r, http.MethodDelete, "/api/v1/blueprints/"+created.ID, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d, body: %s", w.Code, w.Body.String())
	}

	// Gone
	w = doJSON(t, r, http.MethodGet, "/api/v1/blueprints/"+created.ID, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: got %d, want 404", w.Code)
	}
}

func TestBlueprintHandler_BuiltinProtection(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	if err := blueprint.SeedBuiltins(context.Background(), h.store); err != nil {
		t.Fatalf("seed builtins: %v", err)
	}
	r := blueprintRouter(h)

	all, err := h.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var builtin *blueprint.Blueprint
	for _, b := range all {
		if b.Builtin {
			builtin = b
			break
		}
	}
	if builtin == nil {
		t.Fatal("expected at least one seeded builtin blueprint")
	}

	// PUT on a builtin → 400
	w := doJSON(t, r, http.MethodPut, "/api/v1/blueprints/"+builtin.ID,
		`{"name":"hijack","request":{"repo_url":"https://example.com/org/repo.git"}}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("update builtin: got %d, want 400, body: %s", w.Code, w.Body.String())
	}

	// DELETE on a builtin → 400
	w = doJSON(t, r, http.MethodDelete, "/api/v1/blueprints/"+builtin.ID, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("delete builtin: got %d, want 400, body: %s", w.Code, w.Body.String())
	}

	// Builtin must survive both attempts.
	if _, err := h.store.Get(context.Background(), builtin.ID); err != nil {
		t.Errorf("builtin after update/delete attempts: %v", err)
	}
}

func TestBlueprintHandler_Delete_ScheduleReference(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	r := blueprintRouter(h)

	w := doJSON(t, r, http.MethodPost, "/api/v1/blueprints",
		`{"name":"referenced","request":{"repo_url":"https://example.com/org/repo.git"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body: %s", w.Code, w.Body.String())
	}
	var created blueprint.Blueprint
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// Referenced by schedules → 409
	h.schedules = &fakeScheduleRefChecker{count: 2}
	w = doJSON(t, r, http.MethodDelete, "/api/v1/blueprints/"+created.ID, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("delete referenced: got %d, want 409, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "2 schedule(s)") {
		t.Errorf("expected schedule count in error, got: %s", w.Body.String())
	}

	// No references → delete succeeds
	h.schedules = &fakeScheduleRefChecker{count: 0}
	w = doJSON(t, r, http.MethodDelete, "/api/v1/blueprints/"+created.ID, "")
	if w.Code != http.StatusNoContent {
		t.Errorf("delete unreferenced: got %d, want 204, body: %s", w.Code, w.Body.String())
	}
}

func TestBlueprintHandler_Run(t *testing.T) {
	const create = `{
		"name":"issue-fixer",
		"request":{"repo_url":"https://example.com/org/repo.git","prompt":"Fix {{.Params.issue}} in {{.Params.env}}","session_type":"feature"},
		"parameters":[{"name":"issue","required":true},{"name":"env","default":"prod"}]
	}`

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantPrompt string
	}{
		{
			name:       "params rendered with default applied",
			body:       `{"params":{"issue":"BUG-1"}}`,
			wantStatus: http.StatusCreated,
			wantPrompt: "Fix BUG-1 in prod",
		},
		{
			name:       "explicit param overrides default",
			body:       `{"params":{"issue":"BUG-2","env":"staging"}}`,
			wantStatus: http.StatusCreated,
			wantPrompt: "Fix BUG-2 in staging",
		},
		{
			name:       "prompt override wins over rendered prompt",
			body:       `{"params":{"issue":"BUG-3"},"prompt":"run-time prompt"}`,
			wantStatus: http.StatusCreated,
			wantPrompt: "run-time prompt",
		},
		{
			name:       "missing required param",
			body:       `{"params":{"env":"staging"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body missing required param",
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid body JSON",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, runner := newBlueprintHandler(t)
			r := blueprintRouter(h)

			w := doJSON(t, r, http.MethodPost, "/api/v1/blueprints", create)
			if w.Code != http.StatusCreated {
				t.Fatalf("create: got %d, body: %s", w.Code, w.Body.String())
			}
			var created blueprint.Blueprint
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}

			w = doJSON(t, r, http.MethodPost, "/api/v1/blueprints/"+created.ID+"/run", tt.body)
			if w.Code != tt.wantStatus {
				t.Fatalf("run: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus != http.StatusCreated {
				if runner.req != nil {
					t.Errorf("session created despite run error, req: %+v", runner.req)
				}
				return
			}
			if runner.req == nil {
				t.Fatal("expected session creation, got none")
			}
			if runner.req.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", runner.req.Prompt, tt.wantPrompt)
			}
			if runner.req.RepoURL != "https://example.com/org/repo.git" {
				t.Errorf("RepoURL = %q, want template value", runner.req.RepoURL)
			}
			if runner.req.SessionType != "feature" {
				t.Errorf("SessionType = %q, want feature", runner.req.SessionType)
			}
		})
	}
}

func TestBlueprintHandler_Run_MissingParamMessage(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	r := blueprintRouter(h)

	w := doJSON(t, r, http.MethodPost, "/api/v1/blueprints",
		`{"name":"issue-fixer","request":{"repo_url":"https://example.com/org/repo.git","prompt":"Fix {{.Params.issue}}"},"parameters":[{"name":"issue","required":true}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body: %s", w.Code, w.Body.String())
	}
	var created blueprint.Blueprint
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	w = doJSON(t, r, http.MethodPost, "/api/v1/blueprints/"+created.ID+"/run", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("run: got %d, want 400, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "issue") {
		t.Errorf("expected missing parameter name in error, got: %s", w.Body.String())
	}
}

func TestBlueprintHandler_Run_NotFound(t *testing.T) {
	h, _ := newBlueprintHandler(t)
	w := doJSON(t, blueprintRouter(h), http.MethodPost, "/api/v1/blueprints/missing/run", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("run missing blueprint: got %d, want 404", w.Code)
	}
}

// TestBlueprintHandler_PresetAlias covers the deprecated /presets routes: the
// old create shape (no parameters), the old "presets" list envelope, and run
// with the old {prompt} body — all backed by the same blueprints.
func TestBlueprintHandler_PresetAlias(t *testing.T) {
	h, runner := newBlueprintHandler(t)
	r := blueprintRouter(h)

	// Create via old preset shape → blueprint with parameters=[]
	w := doJSON(t, r, http.MethodPost, "/api/v1/presets",
		`{"name":"deps-bump","description":"weekly","request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create preset: got %d, body: %s", w.Code, w.Body.String())
	}
	var created blueprint.Blueprint
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("expected id in preset create response")
	}
	if len(created.Parameters) != 0 {
		t.Errorf("preset parameters = %+v, want empty", created.Parameters)
	}

	// List via alias → "presets" envelope
	w = doJSON(t, r, http.MethodGet, "/api/v1/presets", "")
	var presetList struct {
		Presets []blueprint.Blueprint `json:"presets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &presetList); err != nil {
		t.Fatal(err)
	}
	if len(presetList.Presets) != 1 || presetList.Presets[0].ID != created.ID {
		t.Errorf("preset list = %+v, want the created blueprint", presetList.Presets)
	}

	// The same row is visible under the canonical "blueprints" envelope.
	w = doJSON(t, r, http.MethodGet, "/api/v1/blueprints", "")
	var bpList struct {
		Blueprints []blueprint.Blueprint `json:"blueprints"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &bpList); err != nil {
		t.Fatal(err)
	}
	if len(bpList.Blueprints) != 1 || bpList.Blueprints[0].ID != created.ID {
		t.Errorf("blueprint list = %+v, want the created blueprint", bpList.Blueprints)
	}

	// Run with no body → template prompt
	w = doJSON(t, r, http.MethodPost, "/api/v1/presets/"+created.ID+"/run", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("run preset: got %d, body: %s", w.Code, w.Body.String())
	}
	if runner.req == nil || runner.req.Prompt != "update deps" {
		t.Fatalf("run preset req = %+v, want template prompt", runner.req)
	}

	// Run with old {prompt} override body
	runner.req = nil
	w = doJSON(t, r, http.MethodPost, "/api/v1/presets/"+created.ID+"/run", `{"prompt":"run-time prompt"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("run preset with prompt: got %d, body: %s", w.Code, w.Body.String())
	}
	if runner.req == nil || runner.req.Prompt != "run-time prompt" {
		t.Fatalf("run preset override req = %+v, want overridden prompt", runner.req)
	}

	// Delete via alias
	w = doJSON(t, r, http.MethodDelete, "/api/v1/presets/"+created.ID, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete preset: got %d, body: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, r, http.MethodGet, "/api/v1/presets/"+created.ID, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("get deleted preset: got %d, want 404", w.Code)
	}
}
