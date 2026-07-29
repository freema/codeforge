package blueprint

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a blueprint does not exist.
var ErrNotFound = errors.New("blueprint not found")

// ErrNameTaken is returned when a blueprint name is already in use (names are unique).
var ErrNameTaken = errors.New("blueprint name already exists")

// ErrBuiltinImmutable is returned when trying to modify or delete a built-in
// blueprint through the regular Update/Delete paths.
var ErrBuiltinImmutable = errors.New("built-in blueprints cannot be modified")

// blueprintColumns is the shared SELECT column list for scanBlueprint.
const blueprintColumns = `id, name, description, builtin, request_json, params_json, created_at, updated_at`

// Store persists blueprints in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a blueprint store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new blueprint and assigns its ID/timestamps.
func (s *Store) Create(ctx context.Context, b *Blueprint) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	b.CreatedAt = now
	b.UpdatedAt = now

	paramsJSON, err := marshalParams(b.Parameters)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO blueprints (id, name, description, builtin, request_json, params_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Name, b.Description, b.Builtin, string(b.Request), paramsJSON,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("inserting blueprint: %w", err)
	}
	return nil
}

// Get returns a blueprint by ID.
func (s *Store) Get(ctx context.Context, id string) (*Blueprint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+blueprintColumns+` FROM blueprints WHERE id = ?`, id)
	return scanBlueprint(row.Scan)
}

// GetByName returns a blueprint by its unique name.
func (s *Store) GetByName(ctx context.Context, name string) (*Blueprint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+blueprintColumns+` FROM blueprints WHERE name = ?`, name)
	return scanBlueprint(row.Scan)
}

// List returns all blueprints ordered by name.
func (s *Store) List(ctx context.Context) ([]*Blueprint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+blueprintColumns+` FROM blueprints ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing blueprints: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Blueprint
	for rows.Next() {
		b, err := scanBlueprint(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Update persists mutable fields (name, description, request, parameters).
// Built-in blueprints cannot be updated through this path — use
// UpdateBuiltinByName from the seeding code instead.
func (s *Store) Update(ctx context.Context, b *Blueprint) error {
	var builtin bool
	err := s.db.QueryRowContext(ctx,
		`SELECT builtin FROM blueprints WHERE id = ?`, b.ID).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("checking blueprint: %w", err)
	}
	if builtin {
		return ErrBuiltinImmutable
	}

	paramsJSON, err := marshalParams(b.Parameters)
	if err != nil {
		return err
	}

	b.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE blueprints SET name = ?, description = ?, request_json = ?, params_json = ?, updated_at = ?
		 WHERE id = ? AND builtin = 0`,
		b.Name, b.Description, string(b.Request), paramsJSON,
		b.UpdatedAt.Format(time.RFC3339Nano), b.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("updating blueprint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a blueprint. Built-in blueprints cannot be deleted through
// this path — stale builtins are garbage-collected by SeedBuiltins.
func (s *Store) Delete(ctx context.Context, id string) error {
	var builtin bool
	err := s.db.QueryRowContext(ctx,
		`SELECT builtin FROM blueprints WHERE id = ?`, id).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("checking blueprint: %w", err)
	}
	if builtin {
		return ErrBuiltinImmutable
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM blueprints WHERE id = ? AND builtin = 0`, id)
	if err != nil {
		return fmt.Errorf("deleting blueprint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateBuiltinByName updates an existing built-in blueprint's description,
// request, and parameters. Used by SeedBuiltins so code changes propagate on
// startup. Returns ErrNotFound when no builtin row with that name exists
// (e.g. the name is held by a user blueprint, which is never clobbered).
func (s *Store) UpdateBuiltinByName(ctx context.Context, b *Blueprint) error {
	paramsJSON, err := marshalParams(b.Parameters)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE blueprints SET description = ?, request_json = ?, params_json = ?, updated_at = ?
		 WHERE name = ? AND builtin = 1`,
		b.Description, string(b.Request), paramsJSON,
		time.Now().UTC().Format(time.RFC3339Nano), b.Name,
	)
	if err != nil {
		return fmt.Errorf("updating builtin blueprint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBuiltinByName removes a built-in blueprint row (used by SeedBuiltins
// to garbage-collect builtins no longer defined in code).
func (s *Store) DeleteBuiltinByName(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM blueprints WHERE name = ? AND builtin = 1`, name)
	if err != nil {
		return fmt.Errorf("deleting builtin blueprint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanBlueprint(scan func(dest ...any) error) (*Blueprint, error) {
	var b Blueprint
	var request, params, createdAt, updatedAt string

	err := scan(&b.ID, &b.Name, &b.Description, &b.Builtin, &request, &params, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning blueprint: %w", err)
	}

	b.Request = json.RawMessage(request)
	if err := json.Unmarshal([]byte(params), &b.Parameters); err != nil {
		return nil, fmt.Errorf("parsing blueprint parameters: %w", err)
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &b, nil
}

// marshalParams serializes parameter definitions, normalizing nil to "[]".
func marshalParams(params []ParameterDefinition) (string, error) {
	if params == nil {
		return "[]", nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshaling parameters: %w", err)
	}
	return string(b), nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure
// (the name column). Matched on the driver's error text — modernc.org/sqlite
// exposes no typed constraint error.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
