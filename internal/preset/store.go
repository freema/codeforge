package preset

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

// ErrNotFound is returned when a preset does not exist.
var ErrNotFound = errors.New("preset not found")

// ErrNameTaken is returned when a preset name is already in use (names are unique).
var ErrNameTaken = errors.New("preset name already exists")

// presetColumns is the shared SELECT column list for scanPreset.
const presetColumns = `id, name, description, request_json, created_at, updated_at`

// Store persists session presets in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a preset store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new preset and assigns its ID/timestamps.
func (s *Store) Create(ctx context.Context, p *Preset) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_presets (id, name, description, request_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, string(p.Request),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("inserting preset: %w", err)
	}
	return nil
}

// Get returns a preset by ID.
func (s *Store) Get(ctx context.Context, id string) (*Preset, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+presetColumns+` FROM session_presets WHERE id = ?`, id)
	return scanPreset(row.Scan)
}

// List returns all presets ordered by name.
func (s *Store) List(ctx context.Context) ([]*Preset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+presetColumns+` FROM session_presets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing presets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Preset
	for rows.Next() {
		p, err := scanPreset(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update persists mutable fields (name, description, request_json).
func (s *Store) Update(ctx context.Context, p *Preset) error {
	p.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE session_presets SET name = ?, description = ?, request_json = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.Description, string(p.Request),
		p.UpdatedAt.Format(time.RFC3339Nano), p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("updating preset: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a preset.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM session_presets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting preset: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPreset(scan func(dest ...any) error) (*Preset, error) {
	var p Preset
	var description sql.NullString
	var request, createdAt, updatedAt string

	err := scan(&p.ID, &p.Name, &description, &request, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning preset: %w", err)
	}

	p.Description = description.String
	p.Request = json.RawMessage(request)
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &p, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure
// (the name column). Matched on the driver's error text — modernc.org/sqlite
// exposes no typed constraint error.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
