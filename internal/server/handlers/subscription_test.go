package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tenant"
	"github.com/freema/codeforge/internal/tool/runner"
)

func TestStringInJSONList(t *testing.T) {
	cases := []struct {
		list, target string
		want         bool
	}{
		{`["claude-code","codex"]`, "codex", true},
		{`["claude-code"]`, "codex", false},
		{"", "anything", true},   // no restriction
		{"   ", "x", true},       // whitespace = no restriction
		{"not json", "x", false}, // malformed = fail closed
		{`[]`, "x", false},       // empty allow-list denies
		{`["a"]`, "a", true},
	}
	for _, c := range cases {
		if got := stringInJSONList(c.list, c.target); got != c.want {
			t.Errorf("stringInJSONList(%q, %q) = %v, want %v", c.list, c.target, got, c.want)
		}
	}
}

func newTenantService(t *testing.T) (*tenant.Service, *tenant.Store, *crypto.Service) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cryptoSvc, err := crypto.NewService(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	store := tenant.NewStore(db)
	return tenant.NewService(store, cryptoSvc), store, cryptoSvc
}

func testCLIRegistry() *runner.Registry {
	reg := runner.NewRegistry("claude-code")
	reg.Register("claude-code", runner.NewClaudeRunner("claude"), runner.RunnerMeta{AIProvider: "anthropic"})
	reg.Register("cursor", runner.NewCursorRunner("cursor-agent"), runner.RunnerMeta{AIProvider: "cursor"})
	return reg
}

// tenantRepo is a repository URL a tenant may use (applyTenant runs after the
// request passed validation, so it always has one).
const tenantRepo = "https://github.com/acme/repo.git"

type fakeCounter struct{ active int }

func (f fakeCounter) CountActiveByTenant(_ context.Context, _ string) (int, error) {
	return f.active, nil
}

func TestApplyTenant_ConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := newTenantService(t)
	res, _ := svc.CreateTenant(ctx, "c", "c", tenant.TierFree) // free: MaxConcurrentSessions = 2
	tnt, _ := store.GetTenant(ctx, res.Tenant.ID)

	h := NewSessionHandler(nil, nil, nil, testCLIRegistry(), nil, nil, svc, nil)

	h.sessionCounter = fakeCounter{active: tnt.MaxConcurrentSessions}
	if status, _ := h.applyTenant(ctx, &session.CreateSessionRequest{RepoURL: tenantRepo}, tnt); status != 429 {
		t.Fatalf("at concurrency limit: status = %d, want 429", status)
	}

	// Under the limit, a BYOK request passes (no pool needed).
	h.sessionCounter = fakeCounter{active: tnt.MaxConcurrentSessions - 1}
	req := &session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{AIApiKey: "byok"}}
	if status, msg := h.applyTenant(ctx, req, tnt); status != 0 {
		t.Fatalf("under concurrency limit: status = %d (%s), want 0", status, msg)
	}
}

func TestApplyTenant(t *testing.T) {
	ctx := context.Background()
	svc, store, cryptoSvc := newTenantService(t)

	res, err := svc.CreateTenant(ctx, "acme", "acme", tenant.TierFree) // free: allowed_clis ["claude-code"], 10/day, $1
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tnt, err := store.GetTenant(ctx, res.Tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	enc, err := cryptoSvc.Encrypt("sk-pool-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddKeyPoolEntry(ctx, &tenant.KeyPoolEntry{ID: "k1", Provider: "anthropic", EncryptedToken: enc, Weight: 1, Active: true}); err != nil {
		t.Fatal(err)
	}

	h := NewSessionHandler(nil, nil, nil, testCLIRegistry(), nil, nil, svc, nil)

	t.Run("disallowed CLI -> 403", func(t *testing.T) {
		req := &session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{CLI: "cursor"}}
		status, _ := h.applyTenant(ctx, req, tnt)
		if status != 403 {
			t.Fatalf("status = %d, want 403", status)
		}
	})

	t.Run("allowed CLI, no BYOK -> pool key assigned + tenant_id stamped + budget capped", func(t *testing.T) {
		req := &session.CreateSessionRequest{RepoURL: tenantRepo}
		status, msg := h.applyTenant(ctx, req, tnt)
		if status != 0 {
			t.Fatalf("status = %d (%s), want 0", status, msg)
		}
		if req.Config == nil || req.Config.AIApiKey != "sk-pool-key" {
			t.Fatalf("pool key not assigned: %+v", req.Config)
		}
		if req.TenantID != tnt.ID {
			t.Errorf("TenantID = %q, want %q", req.TenantID, tnt.ID)
		}
		if req.Config.MaxBudgetUSD != 1.0 {
			t.Errorf("budget = %v, want tier cap 1.0", req.Config.MaxBudgetUSD)
		}
	})

	t.Run("BYOK key preserved, pool not consulted", func(t *testing.T) {
		req := &session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{AIApiKey: "my-own-key"}}
		status, _ := h.applyTenant(ctx, req, tnt)
		if status != 0 {
			t.Fatalf("status = %d, want 0", status)
		}
		if req.Config.AIApiKey != "my-own-key" {
			t.Errorf("BYOK key overwritten: %q", req.Config.AIApiKey)
		}
	})

	t.Run("daily limit reached -> 429", func(t *testing.T) {
		limited, _ := svc.CreateTenant(ctx, "small", "small", tenant.TierFree)
		lt, _ := store.GetTenant(ctx, limited.Tenant.ID)
		for i := 0; i < lt.MaxSessionsPerDay; i++ {
			_ = store.LogUsage(ctx, &tenant.UsageLog{TenantID: lt.ID, SessionID: string(rune('a' + i)), CLI: "claude-code"})
		}
		req := &session.CreateSessionRequest{RepoURL: tenantRepo}
		status, _ := h.applyTenant(ctx, req, lt)
		if status != 429 {
			t.Fatalf("status = %d, want 429 after hitting daily limit", status)
		}
	})
}

type fakeSessions map[string]*session.Session

func (f fakeSessions) Get(_ context.Context, id string) (*session.Session, error) {
	if t, ok := f[id]; ok {
		return t, nil
	}
	return nil, apperror.NotFound("session %s not found", id)
}

// TestApplyTenant_RejectsForeignCredentials covers request fields that would
// let a tenant session run with the operator's or another tenant's access.
func TestApplyTenant_RejectsForeignCredentials(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := newTenantService(t)
	res, err := svc.CreateTenant(ctx, "acme", "acme", tenant.TierFree)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tnt, err := store.GetTenant(ctx, res.Tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	h := NewSessionHandler(nil, nil, nil, testCLIRegistry(), nil, nil, svc, nil)
	h.sessions = fakeSessions{
		"own":      {ID: "own", TenantID: tnt.ID},
		"other":    {ID: "other", TenantID: "another-tenant"},
		"operator": {ID: "operator"},
	}

	tests := []struct {
		name       string
		req        session.CreateSessionRequest
		wantStatus int
	}{
		{"operator provider key", session.CreateSessionRequest{RepoURL: tenantRepo, ProviderKey: "operator-github"}, 403},
		{"file URL", session.CreateSessionRequest{RepoURL: "file:///data/workspaces/other/"}, 400},
		{"ssh URL", session.CreateSessionRequest{RepoURL: "ssh://git@github.com/acme/repo.git"}, 400},
		{"another tenant's workspace", session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{WorkspaceSessionID: "other"}}, 404},
		{"an operator session's workspace", session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{WorkspaceSessionID: "operator"}}, 404},
		{"unknown workspace", session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{WorkspaceSessionID: "missing"}}, 404},
		// BYOK keeps the pool out of these so only the checks above decide.
		{"own workspace", session.CreateSessionRequest{RepoURL: tenantRepo, Config: &session.Config{WorkspaceSessionID: "own", AIApiKey: "byok"}}, 0},
		{"own access token", session.CreateSessionRequest{RepoURL: tenantRepo, AccessToken: "ghp_tenant", Config: &session.Config{AIApiKey: "byok"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			if status, msg := h.applyTenant(ctx, &req, tnt); status != tt.wantStatus {
				t.Fatalf("status = %d (%s), want %d", status, msg, tt.wantStatus)
			}
		})
	}
}
