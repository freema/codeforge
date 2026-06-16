package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"

	_ "modernc.org/sqlite"

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

type fakeCounter struct{ active int }

func (f fakeCounter) CountActiveByTenant(_ context.Context, _ string) (int, error) {
	return f.active, nil
}

func TestApplyTenant_ConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := newTenantService(t)
	res, _ := svc.CreateTenant(ctx, "c", "c", tenant.TierFree) // free: MaxConcurrentSessions = 2
	tnt, _ := store.GetTenant(ctx, res.Tenant.ID)

	h := NewSessionHandler(nil, nil, nil, testCLIRegistry(), nil, nil, svc)

	h.sessionCounter = fakeCounter{active: tnt.MaxConcurrentSessions}
	if status, _ := h.applyTenant(ctx, &session.CreateSessionRequest{}, tnt); status != 429 {
		t.Fatalf("at concurrency limit: status = %d, want 429", status)
	}

	// Under the limit, a BYOK request passes (no pool needed).
	h.sessionCounter = fakeCounter{active: tnt.MaxConcurrentSessions - 1}
	req := &session.CreateSessionRequest{Config: &session.Config{AIApiKey: "byok"}}
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

	h := NewSessionHandler(nil, nil, nil, testCLIRegistry(), nil, nil, svc)

	t.Run("disallowed CLI -> 403", func(t *testing.T) {
		req := &session.CreateSessionRequest{Config: &session.Config{CLI: "cursor"}}
		status, _ := h.applyTenant(ctx, req, tnt)
		if status != 403 {
			t.Fatalf("status = %d, want 403", status)
		}
	})

	t.Run("allowed CLI, no BYOK -> pool key assigned + tenant_id stamped + budget capped", func(t *testing.T) {
		req := &session.CreateSessionRequest{}
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
		req := &session.CreateSessionRequest{Config: &session.Config{AIApiKey: "my-own-key"}}
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
		req := &session.CreateSessionRequest{}
		status, _ := h.applyTenant(ctx, req, lt)
		if status != 429 {
			t.Fatalf("status = %d, want 429 after hitting daily limit", status)
		}
	})
}
