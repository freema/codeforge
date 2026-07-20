package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/server/middleware"
	"github.com/freema/codeforge/internal/tenant"
)

func newMeTestHandler(t *testing.T) (*TenantHandler, *tenant.Store) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := tenant.NewStore(db)
	return NewTenantHandler(tenant.NewService(store, nil), nil), store
}

func seedMeTenant(t *testing.T, store *tenant.Store) *tenant.Tenant {
	t.Helper()
	tnt := &tenant.Tenant{
		ID:                    "t-me",
		Name:                  "Acme",
		Slug:                  "acme",
		Tier:                  tenant.TierPro,
		APITokenHash:          "h",
		MaxSessionsPerDay:     50,
		MaxConcurrentSessions: 2,
		AllowedCLIs:           `["claude-code"]`,
	}
	if err := store.CreateTenant(context.Background(), tnt); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tnt
}

func TestMe_Operator(t *testing.T) {
	h, _ := newMeTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["role"] != "operator" {
		t.Errorf("role = %v, want operator", body["role"])
	}
	if _, ok := body["tenant"]; ok {
		t.Error("operator response must not contain a tenant object")
	}
}

func TestMe_Tenant(t *testing.T) {
	h, store := newMeTestHandler(t)
	tnt := seedMeTenant(t, store)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req = req.WithContext(middleware.ContextWithTenant(req.Context(), tnt))
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Role   string         `json:"role"`
		Tenant *tenant.Tenant `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Role != "tenant" || body.Tenant == nil || body.Tenant.Name != "Acme" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestMeUsage_OperatorGets404(t *testing.T) {
	h, _ := newMeTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/me/usage", nil)
	rec := httptest.NewRecorder()
	h.MeUsage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestMeUsage_Tenant(t *testing.T) {
	h, store := newMeTestHandler(t)
	tnt := seedMeTenant(t, store)

	if err := store.LogUsage(context.Background(), &tenant.UsageLog{
		TenantID:     tnt.ID,
		SessionID:    "sess-1",
		CLI:          "claude-code",
		InputTokens:  100,
		OutputTokens: 50,
	}); err != nil {
		t.Fatalf("log usage: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/me/usage?period=24h", nil)
	req = req.WithContext(middleware.ContextWithTenant(req.Context(), tnt))
	rec := httptest.NewRecorder()
	h.MeUsage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Period        string               `json:"period"`
		SessionsToday int                  `json:"sessions_today"`
		Summary       *tenant.UsageSummary `json:"summary"`
		Limits        map[string]any       `json:"limits"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Period != "24h" {
		t.Errorf("period = %q, want 24h", body.Period)
	}
	if body.SessionsToday != 1 {
		t.Errorf("sessions_today = %d, want 1", body.SessionsToday)
	}
	if body.Summary == nil || body.Summary.TotalSessions != 1 || body.Summary.TotalInputTokens != 100 {
		t.Errorf("unexpected summary: %+v", body.Summary)
	}
	if body.Limits["tier"] != tenant.TierPro {
		t.Errorf("limits.tier = %v, want pro", body.Limits["tier"])
	}
}

func TestMeUsage_DefaultPeriod(t *testing.T) {
	h, store := newMeTestHandler(t)
	tnt := seedMeTenant(t, store)

	req := httptest.NewRequest(http.MethodGet, "/me/usage", nil)
	req = req.WithContext(middleware.ContextWithTenant(req.Context(), tnt))
	rec := httptest.NewRecorder()
	h.MeUsage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["period"] != "7d" {
		t.Errorf("default period = %v, want 7d", body["period"])
	}
}
