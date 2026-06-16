package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/freema/codeforge/internal/tenant"
)

type fakeLookup struct {
	t       *tenant.Tenant
	err     error
	gotHash string
}

func (f *fakeLookup) GetTenantByTokenHash(_ context.Context, hash string) (*tenant.Tenant, error) {
	f.gotHash = hash
	return f.t, f.err
}

func TestTenantAuth(t *testing.T) {
	const tok = "cfk_secret"

	cases := []struct {
		name       string
		header     string
		operator   string
		lookup     TenantLookup
		wantStatus int
		wantTenant string // "" = no tenant attached
	}{
		{"operator token passes, no tenant", "Bearer op", "op", nil, http.StatusOK, ""},
		{"valid tenant attaches", "Bearer " + tok, "op", &fakeLookup{t: &tenant.Tenant{ID: "t1"}}, http.StatusOK, "t1"},
		{"tenant lookup error -> 401", "Bearer " + tok, "op", &fakeLookup{err: errors.New("nope")}, http.StatusUnauthorized, ""},
		{"tenant lookup nil -> 401", "Bearer " + tok, "op", &fakeLookup{}, http.StatusUnauthorized, ""},
		{"unknown non-cfk token -> 401", "Bearer random", "op", &fakeLookup{t: &tenant.Tenant{ID: "t1"}}, http.StatusUnauthorized, ""},
		{"missing header -> 401", "", "op", nil, http.StatusUnauthorized, ""},
		{"bare token without Bearer -> 401", tok, "op", nil, http.StatusUnauthorized, ""},
		{"nil lookup with cfk -> 401", "Bearer " + tok, "op", nil, http.StatusUnauthorized, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotTenant string
			ran := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ran = true
				if tn := TenantFromContext(r.Context()); tn != nil {
					gotTenant = tn.ID
				}
				w.WriteHeader(http.StatusOK)
			})

			h := TenantAuth(c.operator, c.lookup)(next)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if c.wantStatus == http.StatusOK && !ran {
				t.Fatal("next handler was not called on success")
			}
			if c.wantStatus != http.StatusOK && ran {
				t.Fatal("next handler ran despite rejection")
			}
			if gotTenant != c.wantTenant {
				t.Errorf("attached tenant = %q, want %q", gotTenant, c.wantTenant)
			}
		})
	}
}

func TestTenantAuth_PassesHashedToken(t *testing.T) {
	const tok = "cfk_abc123"
	fl := &fakeLookup{t: &tenant.Tenant{ID: "t1"}}
	h := TenantAuth("op", fl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if fl.gotHash != tenant.HashToken(tok) {
		t.Errorf("lookup received hash %q, want hashed token %q (raw token must never be queried)", fl.gotHash, tenant.HashToken(tok))
	}
	if fl.gotHash == tok {
		t.Error("raw token was passed to lookup instead of its hash")
	}
}

func TestOperatorOnly(t *testing.T) {
	t.Run("tenant rejected with 403", func(t *testing.T) {
		ran := false
		final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { ran = true })
		// Attach a tenant via TenantAuth, then guard with OperatorOnly.
		h := TenantAuth("op", &fakeLookup{t: &tenant.Tenant{ID: "t1"}})(OperatorOnly(final))
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("Authorization", "Bearer cfk_x")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if ran {
			t.Fatal("operator-only handler ran for a tenant request")
		}
	})

	t.Run("operator allowed", func(t *testing.T) {
		ran := false
		final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { ran = true; w.WriteHeader(http.StatusOK) })
		h := TenantAuth("op", nil)(OperatorOnly(final))
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("Authorization", "Bearer op")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || !ran {
			t.Fatalf("operator should pass: code=%d ran=%v", rec.Code, ran)
		}
	})

	t.Run("no auth context (BearerAuth mode) passes", func(t *testing.T) {
		ran := false
		final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { ran = true })
		OperatorOnly(final).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin", nil))
		if !ran {
			t.Fatal("OperatorOnly must pass when no tenant is in context (plain BearerAuth mode)")
		}
	})
}
