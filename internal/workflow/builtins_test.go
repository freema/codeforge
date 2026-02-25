package workflow

import (
	"context"
	"testing"
)

func TestSeedBuiltins_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// First call
	if err := SeedBuiltins(ctx, reg); err != nil {
		t.Fatalf("first SeedBuiltins: %v", err)
	}

	// Second call — should not error
	if err := SeedBuiltins(ctx, reg); err != nil {
		t.Fatalf("second SeedBuiltins: %v", err)
	}

	// Verify workflows exist
	list, err := reg.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(BuiltinWorkflows) {
		t.Fatalf("expected %d workflows, got %d", len(BuiltinWorkflows), len(list))
	}
	for _, def := range list {
		if !def.Builtin {
			t.Fatalf("expected builtin=true for %s", def.Name)
		}
	}
}
