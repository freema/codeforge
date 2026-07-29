package blueprint

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func seedBlueprint(t *testing.T, store *Store, name string) *Blueprint {
	t.Helper()
	b := &Blueprint{
		Name:        name,
		Description: "nightly dependency bump",
		Request:     json.RawMessage(`{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}`),
	}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return b
}

func seedBuiltinBlueprint(t *testing.T, store *Store, name string) *Blueprint {
	t.Helper()
	b := &Blueprint{
		Name:        name,
		Description: "builtin",
		Builtin:     true,
		Request:     json.RawMessage(`{"repo_url":"{{.Params.repo_url}}"}`),
		Parameters:  []ParameterDefinition{{Name: "repo_url", Required: true}},
	}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatalf("Create builtin (%q): %v", name, err)
	}
	return b
}

func TestStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	b := seedBlueprint(t, store, "deps-bump")
	if b.ID == "" || b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
		t.Fatalf("Create did not assign id/timestamps: %+v", b)
	}

	got, err := store.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "deps-bump" || got.Description != "nightly dependency bump" || got.Builtin {
		t.Errorf("Get = %+v, want seeded fields", got)
	}
	if string(got.Request) != string(b.Request) {
		t.Errorf("Request round-trip = %s, want %s", got.Request, b.Request)
	}
	if len(got.Parameters) != 0 {
		t.Errorf("Parameters = %+v, want empty", got.Parameters)
	}
}

func TestStore_GetByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	b := seedBlueprint(t, store, "deps-bump")

	got, err := store.GetByName(ctx, "deps-bump")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.ID != b.ID {
		t.Errorf("GetByName ID = %s, want %s", got.ID, b.ID)
	}
	if _, err := store.GetByName(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByName missing error = %v, want ErrNotFound", err)
	}
}

func TestStore_ParametersRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	b := &Blueprint{
		Name:    "with-params",
		Request: json.RawMessage(`{"repo_url":"{{.Params.repo_url}}"}`),
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "focus", Default: "everything"},
		},
	}
	if err := store.Create(ctx, b); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Parameters) != 2 {
		t.Fatalf("Parameters = %d, want 2", len(got.Parameters))
	}
	if got.Parameters[0].Name != "repo_url" || !got.Parameters[0].Required {
		t.Errorf("Parameters[0] = %+v, want repo_url required", got.Parameters[0])
	}
	if got.Parameters[1].Name != "focus" || got.Parameters[1].Default != "everything" {
		t.Errorf("Parameters[1] = %+v, want focus with default", got.Parameters[1])
	}
}

func TestStore_CreateDuplicateName(t *testing.T) {
	store := newTestStore(t)
	seedBlueprint(t, store, "deps-bump")

	err := store.Create(context.Background(), &Blueprint{
		Name:    "deps-bump",
		Request: json.RawMessage(`{"repo_url":"https://example.com/org/other.git"}`),
	})
	if !errors.Is(err, ErrNameTaken) {
		t.Errorf("Create duplicate name error = %v, want ErrNameTaken", err)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing error = %v, want ErrNotFound", err)
	}
}

func TestStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	empty, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("List empty = %d items, want 0", len(empty))
	}

	seedBlueprint(t, store, "zeta")
	seedBlueprint(t, store, "alpha")

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List = %d items, want 2", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("List order = [%s, %s], want name-ordered [alpha, zeta]", got[0].Name, got[1].Name)
	}
}

func TestStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	taken := seedBlueprint(t, store, "taken")
	builtin := seedBuiltinBlueprint(t, store, "builtin-bp")
	b := seedBlueprint(t, store, "deps-bump")

	tests := []struct {
		name    string
		mutate  func(c Blueprint) Blueprint
		wantErr error
	}{
		{
			name: "updates fields",
			mutate: func(c Blueprint) Blueprint {
				c.Name = "deps-bump-v2"
				c.Description = "weekly"
				c.Request = json.RawMessage(`{"repo_url":"https://example.com/org/repo.git","session_type":"feature"}`)
				c.Parameters = []ParameterDefinition{{Name: "focus"}}
				return c
			},
		},
		{
			name: "unknown id",
			mutate: func(c Blueprint) Blueprint {
				c.ID = "missing"
				return c
			},
			wantErr: ErrNotFound,
		},
		{
			name: "rename to taken name",
			mutate: func(c Blueprint) Blueprint {
				c.Name = taken.Name
				return c
			},
			wantErr: ErrNameTaken,
		},
		{
			name: "builtin refused",
			mutate: func(c Blueprint) Blueprint {
				c.ID = builtin.ID
				c.Name = builtin.Name
				return c
			},
			wantErr: ErrBuiltinImmutable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := tt.mutate(*b)
			err := store.Update(ctx, &upd)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			got, err := store.Get(ctx, b.ID)
			if err != nil {
				t.Fatalf("Get after update: %v", err)
			}
			if got.Name != upd.Name || got.Description != upd.Description || string(got.Request) != string(upd.Request) {
				t.Errorf("Get after update = %+v, want updated fields %+v", got, upd)
			}
			if len(got.Parameters) != 1 || got.Parameters[0].Name != "focus" {
				t.Errorf("Parameters after update = %+v, want [focus]", got.Parameters)
			}
			if !got.UpdatedAt.After(got.CreatedAt) {
				t.Errorf("UpdatedAt %v not after CreatedAt %v", got.UpdatedAt, got.CreatedAt)
			}
		})
	}
}

func TestStore_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	b := seedBlueprint(t, store, "deps-bump")

	if err := store.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing error = %v, want ErrNotFound", err)
	}
}

func TestStore_Delete_BuiltinRefused(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	builtin := seedBuiltinBlueprint(t, store, "builtin-bp")

	if err := store.Delete(ctx, builtin.ID); !errors.Is(err, ErrBuiltinImmutable) {
		t.Fatalf("Delete builtin error = %v, want ErrBuiltinImmutable", err)
	}
	if _, err := store.Get(ctx, builtin.ID); err != nil {
		t.Errorf("builtin should still exist after refused delete: %v", err)
	}

	// The seeding path can remove builtin rows by name.
	if err := store.DeleteBuiltinByName(ctx, builtin.Name); err != nil {
		t.Fatalf("DeleteBuiltinByName: %v", err)
	}
	if _, err := store.Get(ctx, builtin.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after DeleteBuiltinByName error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateBuiltinByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	builtin := seedBuiltinBlueprint(t, store, "builtin-bp")
	user := seedBlueprint(t, store, "user-bp")

	upd := *builtin
	upd.Description = "updated builtin"
	upd.Request = json.RawMessage(`{"repo_url":"{{.Params.repo_url}}","session_type":"review"}`)
	if err := store.UpdateBuiltinByName(ctx, &upd); err != nil {
		t.Fatalf("UpdateBuiltinByName: %v", err)
	}
	got, err := store.Get(ctx, builtin.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "updated builtin" || string(got.Request) != string(upd.Request) {
		t.Errorf("Get after UpdateBuiltinByName = %+v, want updated fields", got)
	}

	// A non-builtin row with the name is never touched.
	userUpd := *user
	if err := store.UpdateBuiltinByName(ctx, &userUpd); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateBuiltinByName on user row error = %v, want ErrNotFound", err)
	}
}
