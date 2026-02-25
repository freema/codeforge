package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/freema/codeforge/internal/apperror"
)

// SQLiteRunStore implements RunStore backed by SQLite.
type SQLiteRunStore struct {
	db *sql.DB
}

// NewSQLiteRunStore creates a new SQLite-backed run store.
func NewSQLiteRunStore(db *sql.DB) *SQLiteRunStore {
	return &SQLiteRunStore{db: db}
}

func (s *SQLiteRunStore) CreateRun(ctx context.Context, run WorkflowRun) error {
	paramsJSON := MarshalMapJSON(run.Params)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_runs (id, workflow_name, status, params_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		run.ID, run.WorkflowName, string(run.Status), paramsJSON, run.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("creating workflow run: %w", err)
	}
	return nil
}

func (s *SQLiteRunStore) GetRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	var run WorkflowRun
	var paramsJSON, createdAt string
	var startedAt, finishedAt sql.NullString
	var errMsg sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, workflow_name, status, params_json, error, created_at, started_at, finished_at FROM workflow_runs WHERE id = ?`,
		runID,
	).Scan(&run.ID, &run.WorkflowName, &run.Status, &paramsJSON, &errMsg, &createdAt, &startedAt, &finishedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("workflow run '%s' not found", runID)
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow run: %w", err)
	}

	run.Params = UnmarshalMapJSON(paramsJSON)
	if errMsg.Valid {
		run.Error = errMsg.String
	}
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		run.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		run.FinishedAt = &t
	}

	return &run, nil
}

func (s *SQLiteRunStore) UpdateRunStatus(ctx context.Context, runID string, status RunStatus, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var query string
	var args []interface{}

	switch status {
	case RunStatusRunning:
		query = `UPDATE workflow_runs SET status = ?, started_at = ? WHERE id = ?`
		args = []interface{}{string(status), now, runID}
	case RunStatusCompleted, RunStatusFailed:
		query = `UPDATE workflow_runs SET status = ?, error = ?, finished_at = ? WHERE id = ?`
		args = []interface{}{string(status), errMsg, now, runID}
	default:
		query = `UPDATE workflow_runs SET status = ? WHERE id = ?`
		args = []interface{}{string(status), runID}
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating workflow run status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NotFound("workflow run '%s' not found", runID)
	}
	return nil
}

func (s *SQLiteRunStore) ListRuns(ctx context.Context, workflowName string) ([]WorkflowRun, error) {
	var rows *sql.Rows
	var err error

	if workflowName != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workflow_name, status, params_json, error, created_at, started_at, finished_at
			 FROM workflow_runs WHERE workflow_name = ? ORDER BY created_at DESC LIMIT 100`,
			workflowName,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workflow_name, status, params_json, error, created_at, started_at, finished_at
			 FROM workflow_runs ORDER BY created_at DESC LIMIT 100`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("listing workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]WorkflowRun, 0)
	for rows.Next() {
		var run WorkflowRun
		var paramsJSON string
		var errMsg sql.NullString
		var createdAt string
		var startedAt, finishedAt sql.NullString

		if err := rows.Scan(&run.ID, &run.WorkflowName, &run.Status, &paramsJSON, &errMsg, &createdAt, &startedAt, &finishedAt); err != nil {
			return nil, fmt.Errorf("scanning workflow run: %w", err)
		}

		run.Params = UnmarshalMapJSON(paramsJSON)
		if errMsg.Valid {
			run.Error = errMsg.String
		}
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
			run.StartedAt = &t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
			run.FinishedAt = &t
		}

		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *SQLiteRunStore) UpsertStep(ctx context.Context, step WorkflowRunStep) error {
	resultJSON := MarshalMapJSON(step.Result)
	var startedAt, finishedAt *string
	if step.StartedAt != nil {
		s := step.StartedAt.Format(time.RFC3339Nano)
		startedAt = &s
	}
	if step.FinishedAt != nil {
		s := step.FinishedAt.Format(time.RFC3339Nano)
		finishedAt = &s
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_run_steps (run_id, step_name, step_type, status, result_json, task_id, error, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, step_name) DO UPDATE SET
			status = excluded.status,
			result_json = excluded.result_json,
			task_id = excluded.task_id,
			error = excluded.error,
			started_at = COALESCE(excluded.started_at, workflow_run_steps.started_at),
			finished_at = excluded.finished_at`,
		step.RunID, step.StepName, string(step.StepType), string(step.Status),
		resultJSON, step.TaskID, step.Error, startedAt, finishedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting workflow run step: %w", err)
	}
	return nil
}

func (s *SQLiteRunStore) GetSteps(ctx context.Context, runID string) ([]WorkflowRunStep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT run_id, step_name, step_type, status, result_json, task_id, error, started_at, finished_at
		 FROM workflow_run_steps WHERE run_id = ? ORDER BY rowid`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting workflow run steps: %w", err)
	}
	defer rows.Close()

	steps := make([]WorkflowRunStep, 0)
	for rows.Next() {
		var step WorkflowRunStep
		var resultJSON string
		var taskID, errMsg sql.NullString
		var startedAt, finishedAt sql.NullString

		if err := rows.Scan(&step.RunID, &step.StepName, &step.StepType, &step.Status, &resultJSON, &taskID, &errMsg, &startedAt, &finishedAt); err != nil {
			return nil, fmt.Errorf("scanning workflow run step: %w", err)
		}

		step.Result = UnmarshalMapJSON(resultJSON)
		if taskID.Valid {
			step.TaskID = taskID.String
		}
		if errMsg.Valid {
			step.Error = errMsg.String
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
			step.StartedAt = &t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
			step.FinishedAt = &t
		}

		steps = append(steps, step)
	}
	return steps, rows.Err()
}
