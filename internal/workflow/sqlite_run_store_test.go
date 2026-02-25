package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/freema/codeforge/internal/apperror"
)

func TestSQLiteRunStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	store := NewSQLiteRunStore(db)
	ctx := context.Background()

	// Create a workflow definition first (FK constraint)
	def := WorkflowDefinition{Name: "test-wf", Description: "test"}
	if err := reg.Create(ctx, def); err != nil {
		t.Fatalf("Create definition: %v", err)
	}

	// Create run
	now := time.Now().UTC()
	run := WorkflowRun{
		ID:           "run-1",
		WorkflowName: "test-wf",
		Status:       RunStatusPending,
		Params:       map[string]string{"key": "value"},
		CreatedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Get run
	got, err := store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.WorkflowName != "test-wf" {
		t.Fatalf("expected workflow_name test-wf, got %s", got.WorkflowName)
	}
	if got.Params["key"] != "value" {
		t.Fatalf("expected params key=value, got %v", got.Params)
	}

	// Update to running
	if err := store.UpdateRunStatus(ctx, "run-1", RunStatusRunning, ""); err != nil {
		t.Fatalf("UpdateRunStatus running: %v", err)
	}
	got, _ = store.GetRun(ctx, "run-1")
	if got.Status != RunStatusRunning {
		t.Fatalf("expected running, got %s", got.Status)
	}
	if got.StartedAt == nil {
		t.Fatal("expected started_at to be set")
	}

	// Update to completed
	if err := store.UpdateRunStatus(ctx, "run-1", RunStatusCompleted, ""); err != nil {
		t.Fatalf("UpdateRunStatus completed: %v", err)
	}
	got, _ = store.GetRun(ctx, "run-1")
	if got.Status != RunStatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}

	// List runs
	runs, err := store.ListRuns(ctx, "test-wf")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	// List all runs
	allRuns, err := store.ListRuns(ctx, "")
	if err != nil {
		t.Fatalf("ListRuns all: %v", err)
	}
	if len(allRuns) != 1 {
		t.Fatalf("expected 1 run, got %d", len(allRuns))
	}
}

func TestSQLiteRunStore_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteRunStore(db)
	ctx := context.Background()

	_, err := store.GetRun(ctx, "nonexistent")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
}

func TestSQLiteRunStore_UpsertStep(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	store := NewSQLiteRunStore(db)
	ctx := context.Background()

	// Setup
	def := WorkflowDefinition{Name: "test-wf", Description: "test"}
	_ = reg.Create(ctx, def)
	now := time.Now().UTC()
	_ = store.CreateRun(ctx, WorkflowRun{
		ID: "run-1", WorkflowName: "test-wf", Status: RunStatusRunning, CreatedAt: now,
	})

	// Insert step
	step := WorkflowRunStep{
		RunID:     "run-1",
		StepName:  "fetch_issue",
		StepType:  StepTypeFetch,
		Status:    StepStatusRunning,
		StartedAt: &now,
	}
	if err := store.UpsertStep(ctx, step); err != nil {
		t.Fatalf("UpsertStep insert: %v", err)
	}

	// Update step (upsert)
	finished := time.Now().UTC()
	step.Status = StepStatusCompleted
	step.Result = map[string]string{"title": "Fix bug"}
	step.FinishedAt = &finished
	if err := store.UpsertStep(ctx, step); err != nil {
		t.Fatalf("UpsertStep update: %v", err)
	}

	// Get steps
	steps, err := store.GetSteps(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Status != StepStatusCompleted {
		t.Fatalf("expected completed, got %s", steps[0].Status)
	}
	if steps[0].Result["title"] != "Fix bug" {
		t.Fatalf("expected result title 'Fix bug', got %v", steps[0].Result)
	}
}

func TestSQLiteRunStore_FailedRun(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	store := NewSQLiteRunStore(db)
	ctx := context.Background()

	_ = reg.Create(ctx, WorkflowDefinition{Name: "test-wf", Description: "test"})
	now := time.Now().UTC()
	_ = store.CreateRun(ctx, WorkflowRun{
		ID: "run-fail", WorkflowName: "test-wf", Status: RunStatusPending, CreatedAt: now,
	})

	if err := store.UpdateRunStatus(ctx, "run-fail", RunStatusFailed, "step_fetch failed: 404"); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}

	got, _ := store.GetRun(ctx, "run-fail")
	if got.Status != RunStatusFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if got.Error != "step_fetch failed: 404" {
		t.Fatalf("expected error message, got %s", got.Error)
	}
}
