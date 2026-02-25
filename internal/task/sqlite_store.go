package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/freema/codeforge/internal/apperror"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
)

// SQLiteStore provides persistent task storage backed by SQLite.
// It serves as write-behind persistence and fallback reader for expired Redis keys.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed task store.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// Save inserts or updates a task in SQLite (UPSERT).
func (s *SQLiteStore) Save(ctx context.Context, t *Task) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	configJSON := marshalJSON(t.Config)
	changesJSON := marshalJSON(t.ChangesSummary)
	usageJSON := marshalJSON(t.Usage)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, status, repo_url, provider_key, prompt, callback_url, config_json,
			result, error, changes_json, usage_json,
			iteration, current_prompt,
			branch, pr_number, pr_url,
			trace_id, created_at, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			repo_url = excluded.repo_url,
			provider_key = excluded.provider_key,
			prompt = excluded.prompt,
			callback_url = excluded.callback_url,
			config_json = excluded.config_json,
			result = excluded.result,
			error = excluded.error,
			changes_json = excluded.changes_json,
			usage_json = excluded.usage_json,
			iteration = excluded.iteration,
			current_prompt = excluded.current_prompt,
			branch = excluded.branch,
			pr_number = excluded.pr_number,
			pr_url = excluded.pr_url,
			trace_id = excluded.trace_id,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at,
			updated_at = excluded.updated_at`,
		t.ID, string(t.Status), t.RepoURL, t.ProviderKey, t.Prompt, t.CallbackURL, configJSON,
		t.Result, t.Error, changesJSON, usageJSON,
		t.Iteration, t.CurrentPrompt,
		t.Branch, t.PRNumber, t.PRURL,
		t.TraceID, t.CreatedAt.Format(time.RFC3339Nano), nullableTime(t.StartedAt), nullableTime(t.FinishedAt), now,
	)
	if err != nil {
		return fmt.Errorf("saving task to sqlite: %w", err)
	}
	return nil
}

// UpdateStatus updates status and related timestamps in SQLite.
func (s *SQLiteStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, startedAt, finishedAt *time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at), updated_at = ? WHERE id = ?`,
		string(status), nullableTime(startedAt), nullableTime(finishedAt), now, taskID,
	)
	if err != nil {
		return fmt.Errorf("updating task status in sqlite: %w", err)
	}
	return nil
}

// UpdateResult stores result, changes summary, and usage info in SQLite.
func (s *SQLiteStore) UpdateResult(ctx context.Context, taskID string, result string, changes *gitpkg.ChangesSummary, usage *UsageInfo) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changesJSON := marshalJSON(changes)
	usageJSON := marshalJSON(usage)

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET result = ?, changes_json = ?, usage_json = ?, updated_at = ? WHERE id = ?`,
		result, changesJSON, usageJSON, now, taskID,
	)
	if err != nil {
		return fmt.Errorf("updating task result in sqlite: %w", err)
	}
	return nil
}

// UpdatePR stores PR metadata in SQLite.
func (s *SQLiteStore) UpdatePR(ctx context.Context, taskID string, branch, prURL string, prNumber int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET branch = ?, pr_url = ?, pr_number = ?, updated_at = ? WHERE id = ?`,
		branch, prURL, prNumber, now, taskID,
	)
	if err != nil {
		return fmt.Errorf("updating task PR in sqlite: %w", err)
	}
	return nil
}

// UpdateError stores an error message in SQLite.
func (s *SQLiteStore) UpdateError(ctx context.Context, taskID string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET error = ?, updated_at = ? WHERE id = ?`,
		errMsg, now, taskID,
	)
	if err != nil {
		return fmt.Errorf("updating task error in sqlite: %w", err)
	}
	return nil
}

// SaveIteration upserts an iteration record.
func (s *SQLiteStore) SaveIteration(ctx context.Context, taskID string, iter Iteration) error {
	changesJSON := marshalJSON(iter.Changes)
	usageJSON := marshalJSON(iter.Usage)
	var endedAt *string
	if iter.EndedAt != nil {
		s := iter.EndedAt.Format(time.RFC3339Nano)
		endedAt = &s
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO task_iterations (task_id, number, prompt, result, error, status, changes_json, usage_json, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(task_id, number) DO UPDATE SET
			prompt = excluded.prompt,
			result = excluded.result,
			error = excluded.error,
			status = excluded.status,
			changes_json = excluded.changes_json,
			usage_json = excluded.usage_json,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at`,
		taskID, iter.Number, iter.Prompt, iter.Result, iter.Error,
		string(iter.Status), changesJSON, usageJSON,
		iter.StartedAt.Format(time.RFC3339Nano), endedAt,
	)
	if err != nil {
		return fmt.Errorf("saving iteration to sqlite: %w", err)
	}
	return nil
}

// Get retrieves a task from SQLite by ID.
// Note: sensitive fields (access_token, ai_api_key) are NOT stored in SQLite.
func (s *SQLiteStore) Get(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	var statusStr, configJSON, changesJSON, usageJSON, createdAt, updatedAt string
	var startedAt, finishedAt sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, status, repo_url, provider_key, prompt, callback_url, config_json,
			result, error, changes_json, usage_json,
			iteration, current_prompt,
			branch, pr_number, pr_url,
			trace_id, created_at, started_at, finished_at, updated_at
		 FROM tasks WHERE id = ?`,
		taskID,
	).Scan(
		&t.ID, &statusStr, &t.RepoURL, &t.ProviderKey, &t.Prompt, &t.CallbackURL, &configJSON,
		&t.Result, &t.Error, &changesJSON, &usageJSON,
		&t.Iteration, &t.CurrentPrompt,
		&t.Branch, &t.PRNumber, &t.PRURL,
		&t.TraceID, &createdAt, &startedAt, &finishedAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("task %s not found", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("getting task from sqlite: %w", err)
	}

	t.Status = TaskStatus(statusStr)
	t.Config = UnmarshalConfig(configJSON)
	t.ChangesSummary = UnmarshalChangesSummary(changesJSON)
	t.Usage = UnmarshalUsageInfo(usageJSON)
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if startedAt.Valid {
		ts, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		t.StartedAt = &ts
	}
	if finishedAt.Valid {
		ts, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		t.FinishedAt = &ts
	}

	return &t, nil
}

// GetIterations loads all iterations for a task from SQLite.
func (s *SQLiteStore) GetIterations(ctx context.Context, taskID string) ([]Iteration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT number, prompt, result, error, status, changes_json, usage_json, started_at, ended_at
		 FROM task_iterations WHERE task_id = ? ORDER BY number`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting iterations from sqlite: %w", err)
	}
	defer rows.Close()

	iterations := make([]Iteration, 0)
	for rows.Next() {
		var iter Iteration
		var statusStr, changesJSON, usageJSON, startedAt string
		var endedAt sql.NullString

		if err := rows.Scan(&iter.Number, &iter.Prompt, &iter.Result, &iter.Error, &statusStr,
			&changesJSON, &usageJSON, &startedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("scanning iteration: %w", err)
		}

		iter.Status = TaskStatus(statusStr)
		iter.Changes = UnmarshalChangesSummary(changesJSON)
		iter.Usage = UnmarshalUsageInfo(usageJSON)
		iter.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
		if endedAt.Valid {
			ts, _ := time.Parse(time.RFC3339Nano, endedAt.String)
			iter.EndedAt = &ts
		}

		iterations = append(iterations, iter)
	}
	return iterations, rows.Err()
}

// List returns task summaries from SQLite with filtering and pagination.
func (s *SQLiteStore) List(ctx context.Context, opts ListOptions) ([]TaskSummary, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Count total
	var countQuery string
	var countArgs []interface{}
	if opts.Status != "" {
		countQuery = `SELECT COUNT(*) FROM tasks WHERE status = ?`
		countArgs = []interface{}{opts.Status}
	} else {
		countQuery = `SELECT COUNT(*) FROM tasks`
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting tasks: %w", err)
	}

	// Query tasks
	var query string
	var args []interface{}
	if opts.Status != "" {
		query = `SELECT id, status, repo_url, prompt, iteration, error, branch, pr_url, created_at, started_at, finished_at
			FROM tasks WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
		args = []interface{}{opts.Status, limit, opts.Offset}
	} else {
		query = `SELECT id, status, repo_url, prompt, iteration, error, branch, pr_url, created_at, started_at, finished_at
			FROM tasks ORDER BY created_at DESC LIMIT ? OFFSET ?`
		args = []interface{}{limit, opts.Offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]TaskSummary, 0)
	for rows.Next() {
		var ts TaskSummary
		var statusStr, prompt, createdAt string
		var startedAt, finishedAt sql.NullString

		if err := rows.Scan(&ts.ID, &statusStr, &ts.RepoURL, &prompt, &ts.Iteration,
			&ts.Error, &ts.Branch, &ts.PRURL, &createdAt, &startedAt, &finishedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning task: %w", err)
		}

		ts.Status = TaskStatus(statusStr)
		ts.Prompt = truncatePrompt(prompt, 200)
		ts.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
			ts.StartedAt = &t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
			ts.FinishedAt = &t
		}

		tasks = append(tasks, ts)
	}
	return tasks, total, rows.Err()
}

// marshalJSON serializes a value to JSON string, returning "{}" on nil or error.
func marshalJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// nullableTime formats a time pointer for SQLite (nil → NULL).
func nullableTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339Nano)
	return &s
}
