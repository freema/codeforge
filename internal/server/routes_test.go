package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/server/handlers"
	"github.com/freema/codeforge/internal/server/middleware"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tenant"
)

// recordingSessionRoutes answers every route with 200 and remembers that a
// handler was reached.
type recordingSessionRoutes struct{ reached bool }

func (f *recordingSessionRoutes) hit(w http.ResponseWriter, _ *http.Request) {
	f.reached = true
	w.WriteHeader(http.StatusOK)
}

func (f *recordingSessionRoutes) List(w http.ResponseWriter, r *http.Request)     { f.hit(w, r) }
func (f *recordingSessionRoutes) Create(w http.ResponseWriter, r *http.Request)   { f.hit(w, r) }
func (f *recordingSessionRoutes) Get(w http.ResponseWriter, r *http.Request)      { f.hit(w, r) }
func (f *recordingSessionRoutes) Instruct(w http.ResponseWriter, r *http.Request) { f.hit(w, r) }
func (f *recordingSessionRoutes) Cancel(w http.ResponseWriter, r *http.Request)   { f.hit(w, r) }
func (f *recordingSessionRoutes) Review(w http.ResponseWriter, r *http.Request)   { f.hit(w, r) }
func (f *recordingSessionRoutes) PostReviewComments(w http.ResponseWriter, r *http.Request) {
	f.hit(w, r)
}
func (f *recordingSessionRoutes) CreatePR(w http.ResponseWriter, r *http.Request)    { f.hit(w, r) }
func (f *recordingSessionRoutes) PushToPR(w http.ResponseWriter, r *http.Request)    { f.hit(w, r) }
func (f *recordingSessionRoutes) GetPRStatus(w http.ResponseWriter, r *http.Request) { f.hit(w, r) }
func (f *recordingSessionRoutes) Diff(w http.ResponseWriter, r *http.Request)        { f.hit(w, r) }

var sessionIDRoutes = []struct{ method, path string }{
	{http.MethodGet, "/sessions/sess-a"},
	{http.MethodPost, "/sessions/sess-a/instruct"},
	{http.MethodPost, "/sessions/sess-a/cancel"},
	{http.MethodPost, "/sessions/sess-a/review"},
	{http.MethodPost, "/sessions/sess-a/post-review"},
	{http.MethodPost, "/sessions/sess-a/create-pr"},
	{http.MethodPost, "/sessions/sess-a/push"},
	{http.MethodGet, "/sessions/sess-a/pr-status"},
	{http.MethodGet, "/sessions/sess-a/diff"},
}

// newSessionRouter mounts the session routes the way the server does, with the
// real ownership check over an in-memory lookup that knows one session owned by
// tenant-a. caller is put in the request context as the authenticated tenant
// (nil means operator).
func newSessionRouter(caller *tenant.Tenant, h sessionRoutes) http.Handler {
	lookup := func(_ context.Context, id string) (*session.Session, error) {
		if id == "sess-a" {
			return &session.Session{ID: id, TenantID: "tenant-a"}, nil
		}
		return nil, apperror.NotFound("session %s not found", id)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if caller != nil {
				req = req.WithContext(middleware.ContextWithTenant(req.Context(), caller))
			}
			next.ServeHTTP(w, req)
		})
	})
	mountSessionRoutes(r, h, handlers.SessionOwnership(lookup), nil)
	return r
}

func TestSessionRoutes_TenantCannotReachForeignSession(t *testing.T) {
	for _, rt := range sessionIDRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			h := &recordingSessionRoutes{}
			rec := httptest.NewRecorder()
			newSessionRouter(&tenant.Tenant{ID: "tenant-b"}, h).ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if h.reached {
				t.Error("handler ran for another tenant's session")
			}
		})
	}
}

func TestSessionRoutes_OwnerAndOperatorReachSession(t *testing.T) {
	callers := map[string]*tenant.Tenant{
		"owner":    {ID: "tenant-a"},
		"operator": nil,
	}
	for name, caller := range callers {
		for _, rt := range sessionIDRoutes {
			t.Run(name+" "+rt.method+" "+rt.path, func(t *testing.T) {
				h := &recordingSessionRoutes{}
				rec := httptest.NewRecorder()
				newSessionRouter(caller, h).ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))

				if rec.Code != http.StatusOK || !h.reached {
					t.Errorf("status = %d, reached = %v; want 200 and handler reached", rec.Code, h.reached)
				}
			})
		}
	}
}

func TestSessionRoutes_ListAndCreateSkipOwnership(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		h := &recordingSessionRoutes{}
		rec := httptest.NewRecorder()
		newSessionRouter(&tenant.Tenant{ID: "tenant-b"}, h).ServeHTTP(rec, httptest.NewRequest(method, "/sessions", nil))

		if rec.Code != http.StatusOK || !h.reached {
			t.Errorf("%s /sessions: status = %d, reached = %v", method, rec.Code, h.reached)
		}
	}
}
