package preset

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

func seedPreset(t *testing.T, store *Store, name string) *Preset {
	t.Helper()
	p := &Preset{
		Name:        name,
		Description: "nightly dependency bump",
		Request:     json.RawMessage(`{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}`),
	}
	if err := store.Create(context.Background(), p); err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return p
}

func TestStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	p := seedPreset(t, store, "deps-bump")
	if p.ID == "" || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatalf("Create did not assign id/timestamps: %+v", p)
	}

	got, err := store.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "deps-bump" || got.Description != "nightly dependency bump" {
		t.Errorf("Get = %+v, want seeded fields", got)
	}
	if string(got.Request) != string(p.Request) {
		t.Errorf("Request round-trip = %s, want %s", got.Request, p.Request)
	}
}

func TestStore_CreateDuplicateName(t *testing.T) {
	store := newTestStore(t)
	seedPreset(t, store, "deps-bump")

	err := store.Create(context.Background(), &Preset{
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

	seedPreset(t, store, "zeta")
	seedPreset(t, store, "alpha")

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
	taken := seedPreset(t, store, "taken")
	p := seedPreset(t, store, "deps-bump")

	tests := []struct {
		name    string
		mutate  func(c Preset) Preset
		wantErr error
	}{
		{
			name: "updates fields",
			mutate: func(c Preset) Preset {
				c.Name = "deps-bump-v2"
				c.Description = "weekly"
				c.Request = json.RawMessage(`{"repo_url":"https://example.com/org/repo.git","session_type":"feature"}`)
				return c
			},
		},
		{
			name: "unknown id",
			mutate: func(c Preset) Preset {
				c.ID = "missing"
				return c
			},
			wantErr: ErrNotFound,
		},
		{
			name: "rename to taken name",
			mutate: func(c Preset) Preset {
				c.Name = taken.Name
				return c
			},
			wantErr: ErrNameTaken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := tt.mutate(*p)
			err := store.Update(ctx, &upd)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			got, err := store.Get(ctx, p.ID)
			if err != nil {
				t.Fatalf("Get after update: %v", err)
			}
			if got.Name != upd.Name || got.Description != upd.Description || string(got.Request) != string(upd.Request) {
				t.Errorf("Get after update = %+v, want updated fields %+v", got, upd)
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
	p := seedPreset(t, store, "deps-bump")

	if err := store.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing error = %v, want ErrNotFound", err)
	}
}
