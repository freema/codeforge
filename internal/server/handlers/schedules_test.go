package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/blueprint"
	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/schedule"
)

// newScheduleEnv builds a ScheduleHandler and its blueprint store over one
// in-memory SQLite database. The scheduler is nil — tests here never reach
// the Run/Fire path.
func newScheduleEnv(t *testing.T) (*ScheduleHandler, *blueprint.Store) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	bpStore := blueprint.NewStore(db)
	return NewScheduleHandler(schedule.NewStore(db), nil, bpStore), bpStore
}

// newScheduleHandler builds a ScheduleHandler over an in-memory SQLite store.
func newScheduleHandler(t *testing.T) *ScheduleHandler {
	t.Helper()
	h, _ := newScheduleEnv(t)
	return h
}

// seedScheduleBlueprint inserts a blueprint directly through the store.
func seedScheduleBlueprint(t *testing.T, store *blueprint.Store, name, request string, params []blueprint.ParameterDefinition) *blueprint.Blueprint {
	t.Helper()
	b := &blueprint.Blueprint{
		Name:       name,
		Request:    json.RawMessage(request),
		Parameters: params,
	}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatalf("seed blueprint: %v", err)
	}
	return b
}

func scheduleRouter(h *ScheduleHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/schedules", h.Create)
	r.Get("/api/v1/schedules", h.List)
	r.Get("/api/v1/schedules/{scheduleID}", h.Get)
	r.Patch("/api/v1/schedules/{scheduleID}", h.Update)
	r.Delete("/api/v1/schedules/{scheduleID}", h.Delete)
	return r
}

func TestValidateSessionRequest(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string // substring the error must contain, "" = no error
	}{
		{
			name: "code with prompt",
			raw:  `{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}`,
		},
		{
			name:    "not an object",
			raw:     `"not-an-object"`,
			wantErr: "not a valid session request",
		},
		{
			name:    "missing repo_url",
			raw:     `{"prompt":"update deps"}`,
			wantErr: "repo_url is required",
		},
		{
			name:    "missing prompt with empty type (code)",
			raw:     `{"repo_url":"https://example.com/org/repo.git"}`,
			wantErr: "prompt is required",
		},
		{
			name:    "missing prompt with explicit type code",
			raw:     `{"repo_url":"https://example.com/org/repo.git","session_type":"code"}`,
			wantErr: "prompt is required",
		},
		{
			name:    "missing prompt with type plan",
			raw:     `{"repo_url":"https://example.com/org/repo.git","session_type":"plan"}`,
			wantErr: "prompt is required",
		},
		{
			name: "missing prompt with type review",
			raw:  `{"repo_url":"https://example.com/org/repo.git","session_type":"review"}`,
		},
		{
			name: "missing prompt with type knowledge",
			raw:  `{"repo_url":"https://example.com/org/repo.git","session_type":"knowledge"}`,
		},
		{
			name: "review with focus prompt",
			raw:  `{"repo_url":"https://example.com/org/repo.git","session_type":"review","prompt":"focus on auth"}`,
		},
		{
			name:    "unknown session type",
			raw:     `{"repo_url":"https://example.com/org/repo.git","session_type":"bogus","prompt":"x"}`,
			wantErr: "unknown session type: bogus",
		},
		{
			name:    "pr_review cannot be scheduled",
			raw:     `{"repo_url":"https://example.com/org/repo.git","session_type":"pr_review","prompt":"x"}`,
			wantErr: "pr_review sessions cannot be scheduled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionRequest(json.RawMessage(tt.raw))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSessionRequest: unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestScheduleHandler_CreateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid code schedule",
			body:       `{"name":"deps-bump","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid review schedule without prompt",
			body:       `{"name":"weekly-review","cron":"0 6 * * 1","session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"review"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid knowledge schedule without prompt",
			body:       `{"name":"knowledge-refresh","cron":"0 6 * * 1","session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"knowledge"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing repo_url",
			body:       `{"name":"deps-bump","cron":"0 6 * * *","session_request":{"prompt":"update deps"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "code without prompt",
			body:       `{"name":"deps-bump","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "plan without prompt",
			body:       `{"name":"planner","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"plan"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown session type",
			body:       `{"name":"deps-bump","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"bogus","prompt":"x"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "pr_review rejected",
			body:       `{"name":"pr-watch","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"pr_review","prompt":"x"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := scheduleRouter(newScheduleHandler(t))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/schedules", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestScheduleHandler_CreateBlueprintMode(t *testing.T) {
	const tmpl = `{"repo_url":"https://example.com/org/{{.Params.repo}}.git","prompt":"update deps"}`
	repoParam := []blueprint.ParameterDefinition{{Name: "repo", Required: true}}

	tests := []struct {
		name       string
		seed       func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint // nil = nothing seeded
		body       func(bp *blueprint.Blueprint) string
		wantStatus int
		wantErrSub string // substring the error body must contain (400 only)
	}{
		{
			name: "valid blueprint schedule",
			seed: func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint {
				return seedScheduleBlueprint(t, store, "deps-bump", tmpl, repoParam)
			},
			body: func(bp *blueprint.Blueprint) string {
				return fmt.Sprintf(`{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":%q,"blueprint_params":{"repo":"widget"}}`, bp.ID)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "blueprint referenced by name",
			seed: func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint {
				return seedScheduleBlueprint(t, store, "deps-bump", tmpl, repoParam)
			},
			body: func(_ *blueprint.Blueprint) string {
				return `{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":"deps-bump","blueprint_params":{"repo":"widget"}}`
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing required param",
			seed: func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint {
				return seedScheduleBlueprint(t, store, "deps-bump", tmpl, repoParam)
			},
			body: func(bp *blueprint.Blueprint) string {
				return fmt.Sprintf(`{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":%q}`, bp.ID)
			},
			wantStatus: http.StatusBadRequest,
			wantErrSub: "missing required parameter",
		},
		{
			name: "both session_request and blueprint_id",
			seed: func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint {
				return seedScheduleBlueprint(t, store, "deps-bump", tmpl, repoParam)
			},
			body: func(bp *blueprint.Blueprint) string {
				return fmt.Sprintf(`{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":%q,"blueprint_params":{"repo":"widget"},"session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"x"}}`, bp.ID)
			},
			wantStatus: http.StatusBadRequest,
			wantErrSub: "exactly one of session_request or blueprint_id",
		},
		{
			name: "neither session_request nor blueprint_id",
			body: func(_ *blueprint.Blueprint) string {
				return `{"name":"bp-sched","cron":"0 6 * * *"}`
			},
			wantStatus: http.StatusBadRequest,
			wantErrSub: "exactly one of session_request or blueprint_id",
		},
		{
			name: "unknown blueprint",
			body: func(_ *blueprint.Blueprint) string {
				return `{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":"ghost"}`
			},
			wantStatus: http.StatusBadRequest,
			wantErrSub: "not found",
		},
		{
			name: "blueprint rendering pr_review rejected",
			seed: func(t *testing.T, store *blueprint.Store) *blueprint.Blueprint {
				return seedScheduleBlueprint(t, store, "pr-watch",
					`{"repo_url":"https://example.com/org/repo.git","session_type":"pr_review"}`, nil)
			},
			body: func(bp *blueprint.Blueprint) string {
				return fmt.Sprintf(`{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":%q}`, bp.ID)
			},
			wantStatus: http.StatusBadRequest,
			wantErrSub: "pr_review sessions cannot be scheduled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, bpStore := newScheduleEnv(t)
			var bp *blueprint.Blueprint
			if tt.seed != nil {
				bp = tt.seed(t, bpStore)
			}
			r := scheduleRouter(h)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/schedules", strings.NewReader(tt.body(bp)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantErrSub != "" {
				if !strings.Contains(w.Body.String(), tt.wantErrSub) {
					t.Errorf("error body = %s, want substring %q", w.Body.String(), tt.wantErrSub)
				}
				return
			}

			var created schedule.Schedule
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created.BlueprintID != bp.ID {
				t.Errorf("BlueprintID = %q, want canonical id %q", created.BlueprintID, bp.ID)
			}
			if len(created.SessionRequest) != 0 {
				t.Errorf("SessionRequest = %s, want empty for blueprint-backed schedule", created.SessionRequest)
			}
		})
	}
}

func TestScheduleHandler_UpdateBlueprintMode(t *testing.T) {
	h, bpStore := newScheduleEnv(t)
	bp := seedScheduleBlueprint(t, bpStore, "deps-bump",
		`{"repo_url":"https://example.com/org/{{.Params.repo}}.git","prompt":"update deps"}`,
		[]blueprint.ParameterDefinition{{Name: "repo", Required: true}})
	r := scheduleRouter(h)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Seed an inline schedule.
	w := do(http.MethodPost, "/api/v1/schedules",
		`{"name":"deps-bump","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("seed create: got %d, body: %s", w.Code, w.Body.String())
	}
	var created schedule.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/schedules/" + created.ID

	// Both modes in one PATCH is refused.
	w = do(http.MethodPatch, path,
		fmt.Sprintf(`{"blueprint_id":%q,"blueprint_params":{"repo":"widget"},"session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"x"}}`, bp.ID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("both modes: got %d, want 400, body: %s", w.Code, w.Body.String())
	}

	// Switching to blueprint mode clears the inline request.
	w = do(http.MethodPatch, path, fmt.Sprintf(`{"blueprint_id":%q,"blueprint_params":{"repo":"widget"}}`, bp.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("switch to blueprint: got %d, body: %s", w.Code, w.Body.String())
	}
	var updated schedule.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.BlueprintID != bp.ID || len(updated.SessionRequest) != 0 {
		t.Fatalf("blueprint switch not applied: %+v", updated)
	}

	// Params-only update is dry-run validated against the stored blueprint.
	w = do(http.MethodPatch, path, `{"blueprint_params":{}}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "missing required parameter") {
		t.Fatalf("invalid params-only patch: got %d, body: %s", w.Code, w.Body.String())
	}
	w = do(http.MethodPatch, path, `{"blueprint_params":{"repo":"gadget"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("params-only patch: got %d, body: %s", w.Code, w.Body.String())
	}

	// Switching back to an inline request clears blueprint mode.
	w = do(http.MethodPatch, path, `{"session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"x"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("switch to inline: got %d, body: %s", w.Code, w.Body.String())
	}
	// Decode into a fresh struct: blueprint_id/blueprint_params are omitempty,
	// so reusing `updated` would keep stale values when the server omits them.
	var inlineAgain schedule.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &inlineAgain); err != nil {
		t.Fatal(err)
	}
	if inlineAgain.BlueprintID != "" || inlineAgain.BlueprintParams != nil || len(inlineAgain.SessionRequest) == 0 {
		t.Fatalf("inline switch not applied: %+v", inlineAgain)
	}
}

func TestBlueprintDelete_ScheduleRefGuard(t *testing.T) {
	h, bpStore := newScheduleEnv(t)
	bp := seedScheduleBlueprint(t, bpStore, "deps-bump",
		`{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}`, nil)
	sr := scheduleRouter(h)

	// Schedule referencing the blueprint.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/schedules",
		strings.NewReader(fmt.Sprintf(`{"name":"bp-sched","cron":"0 6 * * *","blueprint_id":%q}`, bp.ID)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	sr.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create schedule: got %d, body: %s", w.Code, w.Body.String())
	}
	var created schedule.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	bh := NewBlueprintHandler(bpStore, nil, nil, h.RefChecker())
	br := chi.NewRouter()
	br.Delete("/api/v1/blueprints/{blueprintID}", bh.Delete)

	// Referenced blueprint cannot be deleted.
	dreq := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/blueprints/"+bp.ID, nil)
	w = httptest.NewRecorder()
	br.ServeHTTP(w, dreq)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete referenced blueprint: got %d, want 409, body: %s", w.Code, w.Body.String())
	}

	// Removing the schedule unblocks the delete.
	sreq := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/schedules/"+created.ID, nil)
	w = httptest.NewRecorder()
	sr.ServeHTTP(w, sreq)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete schedule: got %d, body: %s", w.Code, w.Body.String())
	}

	dreq = httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/blueprints/"+bp.ID, nil)
	w = httptest.NewRecorder()
	br.ServeHTTP(w, dreq)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete unreferenced blueprint: got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestScheduleHandler_UpdateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid replacement",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"new prompt"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "omitted session_request keeps existing",
			body:       `{"name":"renamed"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "review without prompt allowed",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"review"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "knowledge without prompt allowed",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"knowledge"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing repo_url",
			body:       `{"session_request":{"prompt":"update deps"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "code without prompt",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "plan without prompt",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"plan"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown session type",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"bogus","prompt":"x"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "pr_review rejected",
			body:       `{"session_request":{"repo_url":"https://example.com/org/repo.git","session_type":"pr_review","prompt":"x"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := scheduleRouter(newScheduleHandler(t))

			// Seed a valid schedule to patch.
			create := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/schedules",
				strings.NewReader(`{"name":"deps-bump","cron":"0 6 * * *","session_request":{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}}`))
			cw := httptest.NewRecorder()
			r.ServeHTTP(cw, create)
			if cw.Code != http.StatusCreated {
				t.Fatalf("seed create: got %d, body: %s", cw.Code, cw.Body.String())
			}
			var created schedule.Schedule
			if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/api/v1/schedules/"+created.ID, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
