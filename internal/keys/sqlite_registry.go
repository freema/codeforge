package keys

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/crypto"
)

// SQLiteRegistry implements Registry backed by SQLite.
type SQLiteRegistry struct {
	db     *sql.DB
	crypto *crypto.Service
}

// NewSQLiteRegistry creates a new SQLite-backed key registry.
func NewSQLiteRegistry(db *sql.DB, cryptoSvc *crypto.Service) *SQLiteRegistry {
	return &SQLiteRegistry{db: db, crypto: cryptoSvc}
}

func (r *SQLiteRegistry) Create(ctx context.Context, key Key) error {
	if key.Provider != "github" && key.Provider != "gitlab" && key.Provider != "sentry" {
		return apperror.Validation("provider must be 'github', 'gitlab', or 'sentry'")
	}

	encrypted, err := r.crypto.Encrypt(key.Token)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		"INSERT INTO keys (name, provider, encrypted_token, scope) VALUES (?, ?, ?, ?)",
		key.Name, key.Provider, encrypted, key.Scope,
	)
	if err != nil {
		// SQLite UNIQUE constraint violation
		if isUniqueViolation(err) {
			return apperror.Conflict("key '%s' already exists for provider '%s'", key.Name, key.Provider)
		}
		return fmt.Errorf("storing key: %w", err)
	}

	return nil
}

func (r *SQLiteRegistry) List(ctx context.Context) ([]Key, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT name, provider, scope, created_at FROM keys ORDER BY created_at",
	)
	if err != nil {
		return nil, fmt.Errorf("listing keys: %w", err)
	}
	defer rows.Close()

	keys := make([]Key, 0)
	for rows.Next() {
		var k Key
		var createdAt string
		if err := rows.Scan(&k.Name, &k.Provider, &k.Scope, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning key: %w", err)
		}
		k.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
		keys = append(keys, k)
	}

	return keys, rows.Err()
}

func (r *SQLiteRegistry) Delete(ctx context.Context, name string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM keys WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("deleting key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("key '%s' not found", name)
	}

	return nil
}

func (r *SQLiteRegistry) Resolve(ctx context.Context, provider, name string) (string, error) {
	var encrypted string
	err := r.db.QueryRowContext(ctx,
		"SELECT encrypted_token FROM keys WHERE provider = ? AND name = ?",
		provider, name,
	).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return "", apperror.NotFound("key '%s' not found for provider '%s'", name, provider)
	}
	if err != nil {
		return "", fmt.Errorf("reading key: %w", err)
	}

	return r.crypto.Decrypt(encrypted)
}

func (r *SQLiteRegistry) Verify(ctx context.Context, name string) (*VerifyResult, string, error) {
	var provider, encrypted string
	err := r.db.QueryRowContext(ctx,
		"SELECT provider, encrypted_token FROM keys WHERE name = ?",
		name,
	).Scan(&provider, &encrypted)
	if err == sql.ErrNoRows {
		return nil, "", apperror.NotFound("key '%s' not found", name)
	}
	if err != nil {
		return nil, "", fmt.Errorf("reading key: %w", err)
	}

	token, err := r.crypto.Decrypt(encrypted)
	if err != nil {
		return nil, "", fmt.Errorf("decrypting token: %w", err)
	}

	result := verifyToken(ctx, provider, token)
	return result, provider, nil
}

func (r *SQLiteRegistry) ResolveByName(ctx context.Context, name string) (string, string, error) {
	var provider, encrypted string
	err := r.db.QueryRowContext(ctx,
		"SELECT provider, encrypted_token FROM keys WHERE name = ?",
		name,
	).Scan(&provider, &encrypted)
	if err == sql.ErrNoRows {
		return "", "", apperror.NotFound("key '%s' not found", name)
	}
	if err != nil {
		return "", "", fmt.Errorf("reading key: %w", err)
	}

	token, err := r.crypto.Decrypt(encrypted)
	if err != nil {
		return "", "", fmt.Errorf("decrypting token: %w", err)
	}

	return token, provider, nil
}

// isUniqueViolation checks if a SQLite error is a UNIQUE constraint violation.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
