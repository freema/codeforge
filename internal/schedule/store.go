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

// scheduleColumns is the shared SELECT column list for scanSchedule.
const scheduleColumns = `id, name, cron, enabled, timezone, session_request, blueprint_id, blueprint_params,
	consecutive_failures, disabled_reason, last_run_at, last_session_id, created_at, updated_at`

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

	params, err := marshalBlueprintParams(sch.BlueprintParams)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO schedules (id, name, cron, enabled, timezone, session_request, blueprint_id, blueprint_params, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sch.ID, sch.Name, sch.Cron, boolToInt(sch.Enabled), sch.Timezone, string(sch.SessionRequest),
		sch.BlueprintID, params,
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
		`SELECT `+scheduleColumns+` FROM schedules WHERE id = ?`, id)
	return scanSchedule(row.Scan)
}

// List returns all schedules ordered by creation time.
func (s *Store) List(ctx context.Context) ([]*Schedule, error) {
	return s.list(ctx,
		`SELECT `+scheduleColumns+` FROM schedules ORDER BY created_at`)
}

// ListEnabled returns schedules the scheduler should consider.
func (s *Store) ListEnabled(ctx context.Context) ([]*Schedule, error) {
	return s.list(ctx,
		`SELECT `+scheduleColumns+` FROM schedules WHERE enabled = 1 ORDER BY created_at`)
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

// Update persists mutable fields (name, cron, enabled, timezone,
// session_request, blueprint_id, blueprint_params, consecutive_failures,
// disabled_reason).
func (s *Store) Update(ctx context.Context, sch *Schedule) error {
	params, err := marshalBlueprintParams(sch.BlueprintParams)
	if err != nil {
		return err
	}

	sch.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET name = ?, cron = ?, enabled = ?, timezone = ?, session_request = ?,
		 blueprint_id = ?, blueprint_params = ?,
		 consecutive_failures = ?, disabled_reason = ?, updated_at = ?
		 WHERE id = ?`,
		sch.Name, sch.Cron, boolToInt(sch.Enabled), sch.Timezone, string(sch.SessionRequest),
		sch.BlueprintID, params,
		sch.ConsecutiveFailures, sch.DisabledReason,
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

// Delete removes a schedule and its run history.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schedule_runs WHERE schedule_id = ?`, id); err != nil {
		return fmt.Errorf("deleting schedule runs: %w", err)
	}
	return nil
}

// MarkRun records a cron firing: shifts the cron base (last_run_at) and
// remembers the created session.
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

// SetLastSession remembers the created session WITHOUT shifting the cron base.
// Used by the manual run endpoint.
func (s *Store) SetLastSession(ctx context.Context, id, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET last_session_id = ?, updated_at = ? WHERE id = ?`,
		sessionID, time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("setting schedule last session: %w", err)
	}
	return nil
}

// InsertRun appends a run-history row and assigns its ID/timestamps.
func (s *Store) InsertRun(ctx context.Context, run *Run) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedule_runs (id, schedule_id, session_id, "trigger", status, error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ScheduleID, run.SessionID, run.Trigger, run.Status, run.Error,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("inserting schedule run: %w", err)
	}
	return nil
}

// ListRuns returns the newest runs for a schedule, newest first.
func (s *Store) ListRuns(ctx context.Context, scheduleID string, limit int) ([]*Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schedule_id, session_id, "trigger", status, error, created_at, updated_at
		 FROM schedule_runs WHERE schedule_id = ?
		 ORDER BY created_at DESC, rowid DESC LIMIT ?`, scheduleID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing schedule runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Run
	for rows.Next() {
		var run Run
		var sessionID, errMsg sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&run.ID, &run.ScheduleID, &sessionID, &run.Trigger, &run.Status, &errMsg, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning schedule run: %w", err)
		}
		run.SessionID = sessionID.String
		run.Error = errMsg.String
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		run.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		out = append(out, &run)
	}
	return out, rows.Err()
}

// UpdateRunBySession stamps the run row(s) fired with this session with the
// session's terminal outcome. No-op (nil) when the session has no run row.
func (s *Store) UpdateRunBySession(ctx context.Context, sessionID, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedule_runs SET status = ?, error = ?, updated_at = ? WHERE session_id = ?`,
		status, errMsg, time.Now().UTC().Format(time.RFC3339Nano), sessionID,
	)
	if err != nil {
		return fmt.Errorf("updating schedule run by session: %w", err)
	}
	return nil
}

// IncrementFailures bumps consecutive_failures and returns the new count.
func (s *Store) IncrementFailures(ctx context.Context, id string) (int, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET consecutive_failures = consecutive_failures + 1, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return 0, fmt.Errorf("incrementing schedule failures: %w", err)
	}
	var n int
	err = s.db.QueryRowContext(ctx, `SELECT consecutive_failures FROM schedules WHERE id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("reading schedule failures: %w", err)
	}
	return n, nil
}

// ResetFailures zeroes consecutive_failures after a success.
func (s *Store) ResetFailures(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET consecutive_failures = 0, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("resetting schedule failures: %w", err)
	}
	return nil
}

// Disable turns a schedule off with a machine-set reason (auto-disable).
func (s *Store) Disable(ctx context.Context, id, reason string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET enabled = 0, disabled_reason = ?, updated_at = ? WHERE id = ?`,
		reason, time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("disabling schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSchedule(scan func(dest ...any) error) (*Schedule, error) {
	var sch Schedule
	var enabled int
	var request, blueprintParams, createdAt, updatedAt string
	var timezone, disabledReason, lastRunAt, lastSessionID sql.NullString

	err := scan(&sch.ID, &sch.Name, &sch.Cron, &enabled, &timezone, &request, &sch.BlueprintID, &blueprintParams,
		&sch.ConsecutiveFailures, &disabledReason, &lastRunAt, &lastSessionID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning schedule: %w", err)
	}

	sch.Enabled = enabled == 1
	sch.Timezone = timezone.String
	sch.DisabledReason = disabledReason.String
	if request != "" {
		sch.SessionRequest = json.RawMessage(request)
	}
	if blueprintParams != "" {
		if err := json.Unmarshal([]byte(blueprintParams), &sch.BlueprintParams); err != nil {
			return nil, fmt.Errorf("parsing schedule blueprint params: %w", err)
		}
	}
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

// ListByBlueprint counts schedules referencing a blueprint — the blueprint
// handler's delete guard (409 while referenced).
func (s *Store) ListByBlueprint(ctx context.Context, blueprintID string) (int, error) {
	if blueprintID == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schedules WHERE blueprint_id = ?`, blueprintID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting schedules by blueprint: %w", err)
	}
	return n, nil
}

// marshalBlueprintParams serializes blueprint parameter values, normalizing
// an empty map to "" (the column default).
func marshalBlueprintParams(params map[string]string) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshaling blueprint params: %w", err)
	}
	return string(b), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
