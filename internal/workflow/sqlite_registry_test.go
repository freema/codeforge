package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/apperror"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE workflow_definitions (
			name TEXT PRIMARY KEY,
			description TEXT NOT NULL DEFAULT '',
			builtin BOOLEAN NOT NULL DEFAULT 0,
			steps_json TEXT NOT NULL DEFAULT '[]',
			params_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
		);
		CREATE TABLE workflow_runs (
			id TEXT PRIMARY KEY,
			workflow_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			params_json TEXT NOT NULL DEFAULT '{}',
			error TEXT,
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			FOREIGN KEY (workflow_name) REFERENCES workflow_definitions(name)
		);
		CREATE TABLE workflow_run_steps (
			run_id TEXT NOT NULL,
			step_name TEXT NOT NULL,
			step_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			result_json TEXT NOT NULL DEFAULT '{}',
			task_id TEXT,
			error TEXT,
			started_at TEXT,
			finished_at TEXT,
			PRIMARY KEY (run_id, step_name),
			FOREIGN KEY (run_id) REFERENCES workflow_runs(id)
		);
	`)
	if err != nil {
		t.Fatalf("creating tables: %v", err)
	}
	return db
}

func TestSQLiteRegistry_CRUD(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	def := WorkflowDefinition{
		Name:        "test-workflow",
		Description: "A test workflow",
		Steps: []StepDefinition{
			{Name: "step1", Type: StepTypeFetch, Config: json.RawMessage(`{"url":"https://example.com"}`)},
		},
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
		},
	}

	// Create
	if err := reg.Create(ctx, def); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get
	got, err := reg.Get(ctx, "test-workflow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "test-workflow" {
		t.Fatalf("expected name test-workflow, got %s", got.Name)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got.Steps))
	}
	if len(got.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(got.Parameters))
	}

	// List
	list, err := reg.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(list))
	}

	// Delete
	if err := reg.Delete(ctx, "test-workflow"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	_, err = reg.Get(ctx, "test-workflow")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
}

func TestSQLiteRegistry_DuplicateConflict(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	def := WorkflowDefinition{Name: "dup", Description: "test"}
	if err := reg.Create(ctx, def); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err := reg.Create(ctx, def)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrConflict) {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
}

func TestSQLiteRegistry_DeleteBuiltinConflict(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	def := WorkflowDefinition{Name: "builtin-wf", Description: "test", Builtin: true}
	if err := reg.Create(ctx, def); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := reg.Delete(ctx, "builtin-wf")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrConflict) {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
}

func TestSQLiteRegistry_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	_, err := reg.Get(ctx, "nonexistent")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
}

func TestSQLiteRegistry_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	err := reg.Delete(ctx, "nonexistent")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
}
