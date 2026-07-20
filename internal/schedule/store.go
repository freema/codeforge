package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a schedule does not exist.
var ErrNotFound = errors.New("schedule not found")

// Store persists schedules in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a schedule store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new schedule and assigns its ID/timestamps.
func (s *Store) Create(ctx context.Context, sch *Schedule) error {
	if sch.ID == "" {
		sch.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	sch.CreatedAt = now
	sch.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (id, name, cron, enabled, session_request, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sch.ID, sch.Name, sch.Cron, boolToInt(sch.Enabled), string(sch.SessionRequest),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("inserting schedule: %w", err)
	}
	return nil
}

// Get returns a schedule by ID.
func (s *Store) Get(ctx context.Context, id string) (*Schedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, cron, enabled, session_request, last_run_at, last_session_id, created_at, updated_at
		 FROM schedules WHERE id = ?`, id)
	return scanSchedule(row.Scan)
}

// List returns all schedules ordered by creation time.
func (s *Store) List(ctx context.Context) ([]*Schedule, error) {
	return s.list(ctx,
		`SELECT id, name, cron, enabled, session_request, last_run_at, last_session_id, created_at, updated_at
		 FROM schedules ORDER BY created_at`)
}

// ListEnabled returns schedules the scheduler should consider.
func (s *Store) ListEnabled(ctx context.Context) ([]*Schedule, error) {
	return s.list(ctx,
		`SELECT id, name, cron, enabled, session_request, last_run_at, last_session_id, created_at, updated_at
		 FROM schedules WHERE enabled = 1 ORDER BY created_at`)
}

func (s *Store) list(ctx context.Context, query string) ([]*Schedule, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Schedule
	for rows.Next() {
		sch, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

// Update persists mutable fields (name, cron, enabled, session_request).
func (s *Store) Update(ctx context.Context, sch *Schedule) error {
	sch.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET name = ?, cron = ?, enabled = ?, session_request = ?, updated_at = ?
		 WHERE id = ?`,
		sch.Name, sch.Cron, boolToInt(sch.Enabled), string(sch.SessionRequest),
		sch.UpdatedAt.Format(time.RFC3339Nano), sch.ID,
	)
	if err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a schedule.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkRun records a firing (used by the scheduler and run-now).
func (s *Store) MarkRun(ctx context.Context, id string, at time.Time, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET last_run_at = ?, last_session_id = ?, updated_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), sessionID, at.UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("marking schedule run: %w", err)
	}
	return nil
}

func scanSchedule(scan func(dest ...any) error) (*Schedule, error) {
	var sch Schedule
	var enabled int
	var request, createdAt, updatedAt string
	var lastRunAt, lastSessionID sql.NullString

	err := scan(&sch.ID, &sch.Name, &sch.Cron, &enabled, &request, &lastRunAt, &lastSessionID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning schedule: %w", err)
	}

	sch.Enabled = enabled == 1
	sch.SessionRequest = json.RawMessage(request)
	sch.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	sch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastRunAt.Valid && lastRunAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, lastRunAt.String); err == nil {
			sch.LastRunAt = &t
		}
	}
	if lastSessionID.Valid {
		sch.LastSessionID = lastSessionID.String
	}
	return &sch, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
