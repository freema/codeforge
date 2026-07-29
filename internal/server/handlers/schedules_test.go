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
	"github.com/freema/codeforge/internal/schedule"
)

// newScheduleHandler builds a ScheduleHandler over an in-memory SQLite store.
// The scheduler is nil — tests here never reach the Run/Fire path.
func newScheduleHandler(t *testing.T) *ScheduleHandler {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewScheduleHandler(schedule.NewStore(db), nil)
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
