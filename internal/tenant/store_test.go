package tenant

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func seedTenant(t *testing.T, s *Store) *Tenant {
	t.Helper()
	tnt := &Tenant{ID: generateID(), Name: "acme", Slug: "acme-" + generateID(), Tier: TierFree, APITokenHash: "h", AllowedCLIs: `["claude-code"]`}
	if err := s.CreateTenant(context.Background(), tnt); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tnt
}

func TestEffectiveWeight(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-5, 1}, {0, 1}, {1, 1}, {7, 7}} {
		if got := effectiveWeight(c.in); got != c.want {
			t.Errorf("effectiveWeight(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGetActiveKeyForProvider_Empty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetActiveKeyForProvider(context.Background(), "anthropic"); err == nil {
		t.Fatal("expected error for empty pool")
	}
}

func TestGetActiveKeyForProvider_ExcludesInactiveAndOtherProvider(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.AddKeyPoolEntry(ctx, &KeyPoolEntry{ID: "want", Provider: "anthropic", EncryptedToken: "e", Weight: 1, Active: true})
	_ = s.AddKeyPoolEntry(ctx, &KeyPoolEntry{ID: "other", Provider: "openai", EncryptedToken: "e", Weight: 1, Active: true})
	_ = s.AddKeyPoolEntry(ctx, &KeyPoolEntry{ID: "inactive", Provider: "anthropic", EncryptedToken: "e", Weight: 1, Active: false})

	for i := 0; i < 50; i++ {
		got, err := s.GetActiveKeyForProvider(ctx, "anthropic")
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if got.ID != "want" {
			t.Fatalf("got id %q, want only active matching-provider key 'want'", got.ID)
		}
	}
}

func TestGetActiveKeyForProvider_WeightedDistribution(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.AddKeyPoolEntry(ctx, &KeyPoolEntry{ID: "light", Provider: "anthropic", EncryptedToken: "e", Weight: 1, Active: true})
	_ = s.AddKeyPoolEntry(ctx, &KeyPoolEntry{ID: "heavy", Provider: "anthropic", EncryptedToken: "e", Weight: 9, Active: true})

	const n = 4000
	heavy := 0
	for i := 0; i < n; i++ {
		got, err := s.GetActiveKeyForProvider(ctx, "anthropic")
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if got.ID == "heavy" {
			heavy++
		}
	}
	frac := float64(heavy) / float64(n)
	// Expected ~0.9; wide tolerance to avoid flakiness.
	if frac < 0.82 || frac > 0.95 {
		t.Errorf("heavy key chosen %.3f of the time, expected ~0.9 (weight 9 vs 1)", frac)
	}
}

func TestLogUsage_AutoID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tnt := seedTenant(t, s)

	log := &UsageLog{TenantID: tnt.ID, SessionID: "s1", CLI: "claude-code"}
	if err := s.LogUsage(ctx, log); err != nil {
		t.Fatalf("LogUsage: %v", err)
	}
	if log.ID == "" {
		t.Error("expected an auto-generated ID")
	}

	// A caller-supplied ID is preserved.
	log2 := &UsageLog{ID: "fixed", TenantID: tnt.ID, SessionID: "s2", CLI: "claude-code"}
	if err := s.LogUsage(ctx, log2); err != nil {
		t.Fatalf("LogUsage: %v", err)
	}
	if log2.ID != "fixed" {
		t.Errorf("ID = %q, want preserved 'fixed'", log2.ID)
	}
}

func TestCountDailySessions_DistinctSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tnt := seedTenant(t, s)

	// Same session logged twice (multi-turn) + one other session today.
	_ = s.LogUsage(ctx, &UsageLog{TenantID: tnt.ID, SessionID: "s1", CLI: "claude-code"})
	_ = s.LogUsage(ctx, &UsageLog{TenantID: tnt.ID, SessionID: "s1", CLI: "claude-code"})
	_ = s.LogUsage(ctx, &UsageLog{TenantID: tnt.ID, SessionID: "s2", CLI: "claude-code"})

	// A row dated yesterday must be excluded.
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO usage_logs (id, tenant_id, session_id, cli, created_at) VALUES (?, ?, ?, ?, ?)`,
		"old", tnt.ID, "s3", "claude-code", "2000-01-01T00:00:00.000"); err != nil {
		t.Fatalf("seed old row: %v", err)
	}

	count, err := s.CountDailySessions(ctx, tnt.ID)
	if err != nil {
		t.Fatalf("CountDailySessions: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 distinct sessions today (multi-turn counts once, yesterday excluded)", count)
	}
}
