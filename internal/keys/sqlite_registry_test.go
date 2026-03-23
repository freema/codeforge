package keys

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/crypto"
	_ "modernc.org/sqlite"
)

// 32 random bytes, base64 encoded for AES-256-GCM
const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func setupTestDB(t *testing.T) (*sql.DB, *crypto.Service) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE keys (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			provider        TEXT NOT NULL CHECK(provider IN ('github', 'gitlab', 'sentry')),
			encrypted_token TEXT NOT NULL,
			scope           TEXT NOT NULL DEFAULT '',
			base_url        TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
			UNIQUE(provider, name)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	cryptoSvc, err := crypto.NewService(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}

	return db, cryptoSvc
}

func TestSQLiteRegistry_CreateAndResolve(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	err := reg.Create(ctx, Key{
		Name:     "my-github",
		Provider: "github",
		Token:    "ghp_secrettoken123",
		Scope:    "repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Resolve should return decrypted token
	token, err := reg.Resolve(ctx, "github", "my-github")
	if err != nil {
		t.Fatal(err)
	}
	if token != "ghp_secrettoken123" {
		t.Errorf("token: got %q, want %q", token, "ghp_secrettoken123")
	}
}

func TestSQLiteRegistry_CreateAndResolveByName(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	err := reg.Create(ctx, Key{
		Name:     "my-gitlab",
		Provider: "gitlab",
		Token:    "glpat-secrettoken456",
	})
	if err != nil {
		t.Fatal(err)
	}

	token, provider, err := reg.ResolveByName(ctx, "my-gitlab")
	if err != nil {
		t.Fatal(err)
	}
	if token != "glpat-secrettoken456" {
		t.Errorf("token: got %q, want %q", token, "glpat-secrettoken456")
	}
	if provider != "gitlab" {
		t.Errorf("provider: got %q, want %q", provider, "gitlab")
	}
}

func TestSQLiteRegistry_List(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	// Create two keys
	_ = reg.Create(ctx, Key{Name: "gh-key", Provider: "github", Token: "tok1"})
	_ = reg.Create(ctx, Key{Name: "gl-key", Provider: "gitlab", Token: "tok2"})

	keys, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Tokens must NOT be in list response
	for _, k := range keys {
		if k.Token != "" {
			t.Errorf("token should be empty in list, got %q for %s", k.Token, k.Name)
		}
	}
}

func TestSQLiteRegistry_Delete(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	_ = reg.Create(ctx, Key{Name: "to-delete", Provider: "github", Token: "tok"})

	err := reg.Delete(ctx, "to-delete")
	if err != nil {
		t.Fatal(err)
	}

	// Should be gone
	_, err = reg.Resolve(ctx, "github", "to-delete")
	if err == nil {
		t.Fatal("expected not found error after delete")
	}
}

func TestSQLiteRegistry_DeleteNotFound(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	err := reg.Delete(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestSQLiteRegistry_DuplicateKey(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	_ = reg.Create(ctx, Key{Name: "dup", Provider: "github", Token: "tok1"})

	err := reg.Create(ctx, Key{Name: "dup", Provider: "github", Token: "tok2"})
	if err == nil {
		t.Fatal("expected conflict error for duplicate key")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestSQLiteRegistry_SameNameDifferentProvider(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	// Same name, different providers — should be allowed (UNIQUE is on provider+name)
	err := reg.Create(ctx, Key{Name: "my-key", Provider: "github", Token: "gh-tok"})
	if err != nil {
		t.Fatal(err)
	}

	err = reg.Create(ctx, Key{Name: "my-key", Provider: "gitlab", Token: "gl-tok"})
	if err != nil {
		t.Fatal(err)
	}

	ghTok, err := reg.Resolve(ctx, "github", "my-key")
	if err != nil {
		t.Fatal(err)
	}
	if ghTok != "gh-tok" {
		t.Errorf("github token: got %q, want %q", ghTok, "gh-tok")
	}

	glTok, err := reg.Resolve(ctx, "gitlab", "my-key")
	if err != nil {
		t.Fatal(err)
	}
	if glTok != "gl-tok" {
		t.Errorf("gitlab token: got %q, want %q", glTok, "gl-tok")
	}
}

func TestSQLiteRegistry_InvalidProvider(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	err := reg.Create(ctx, Key{Name: "bad", Provider: "bitbucket", Token: "tok"})
	if err == nil {
		t.Fatal("expected validation error for unsupported provider")
	}
}

func TestSQLiteRegistry_SentryProvider(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	err := reg.Create(ctx, Key{Name: "sentry-key", Provider: "sentry", Token: "sntrys_tok"})
	if err != nil {
		t.Fatal(err)
	}

	token, err := reg.Resolve(ctx, "sentry", "sentry-key")
	if err != nil {
		t.Fatal(err)
	}
	if token != "sntrys_tok" {
		t.Errorf("token: got %q, want %q", token, "sntrys_tok")
	}
}

func TestSQLiteRegistry_ResolveNotFound(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	_, err := reg.Resolve(ctx, "github", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestSQLiteRegistry_EncryptionRoundtrip(t *testing.T) {
	db, cryptoSvc := setupTestDB(t)
	reg := NewSQLiteRegistry(db, cryptoSvc)
	ctx := context.Background()

	originalToken := "ghp_very_secret_token_with_special_chars_!@#$%"

	_ = reg.Create(ctx, Key{Name: "enc-test", Provider: "github", Token: originalToken})

	// Verify stored value is encrypted (not plaintext)
	var encrypted string
	err := db.QueryRowContext(context.Background(), "SELECT encrypted_token FROM keys WHERE name = ?", "enc-test").Scan(&encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == originalToken {
		t.Fatal("token stored in plaintext — encryption not working")
	}

	// Verify decryption returns original
	token, err := reg.Resolve(ctx, "github", "enc-test")
	if err != nil {
		t.Fatal(err)
	}
	if token != originalToken {
		t.Errorf("decrypted token: got %q, want %q", token, originalToken)
	}
}
