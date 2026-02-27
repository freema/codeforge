package task

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	gitpkg "github.com/freema/codeforge/internal/tool/git"
)

// openTestDB opens an in-memory SQLite database and runs the task schema migration.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE tasks (
			id              TEXT PRIMARY KEY,
			status          TEXT NOT NULL DEFAULT 'pending',
			repo_url        TEXT NOT NULL,
			provider_key    TEXT NOT NULL DEFAULT '',
			prompt          TEXT NOT NULL,
			task_type       TEXT NOT NULL DEFAULT 'code',
			callback_url    TEXT NOT NULL DEFAULT '',
			config_json     TEXT NOT NULL DEFAULT '{}',
			result          TEXT NOT NULL DEFAULT '',
			error           TEXT NOT NULL DEFAULT '',
			changes_json    TEXT NOT NULL DEFAULT '{}',
			usage_json      TEXT NOT NULL DEFAULT '{}',
			iteration       INTEGER NOT NULL DEFAULT 1,
			current_prompt  TEXT NOT NULL DEFAULT '',
			branch          TEXT NOT NULL DEFAULT '',
			pr_number       INTEGER NOT NULL DEFAULT 0,
			pr_url          TEXT NOT NULL DEFAULT '',
			trace_id        TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			started_at      TEXT,
			finished_at     TEXT,
			updated_at      TEXT NOT NULL,
			review_result_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE TABLE task_iterations (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     TEXT NOT NULL,
			number      INTEGER NOT NULL,
			prompt      TEXT NOT NULL,
			result      TEXT NOT NULL DEFAULT '',
			error       TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL,
			changes_json TEXT NOT NULL DEFAULT '{}',
			usage_json  TEXT NOT NULL DEFAULT '{}',
			started_at  TEXT NOT NULL,
			ended_at    TEXT,
			FOREIGN KEY (task_id) REFERENCES tasks(id),
			UNIQUE(task_id, number)
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func makeTask(id string) *Task {
	now := time.Now().UTC()
	return &Task{
		ID:          id,
		Status:      StatusPending,
		RepoURL:     "https://github.com/user/repo.git",
		ProviderKey: "gh-key",
		Prompt:      "fix the bug",
		CallbackURL: "https://example.com/callback",
		Iteration:   1,
		TraceID:     "trace-123",
		CreatedAt:   now,
	}
}

func TestSQLiteStore_SaveAndGet(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	task := makeTask("task-1")
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
	if got.Status != StatusPending {
		t.Errorf("Status: got %q, want pending", got.Status)
	}
	if got.RepoURL != task.RepoURL {
		t.Errorf("RepoURL: got %q, want %q", got.RepoURL, task.RepoURL)
	}
	if got.Prompt != task.Prompt {
		t.Errorf("Prompt: got %q, want %q", got.Prompt, task.Prompt)
	}
	if got.TraceID != task.TraceID {
		t.Errorf("TraceID: got %q, want %q", got.TraceID, task.TraceID)
	}
}

func TestSQLiteStore_SaveUpsert(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	task := makeTask("task-upsert")
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// Update and save again
	task.Status = StatusRunning
	task.Iteration = 2
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("second Save (upsert): %v", err)
	}

	got, err := store.Get(ctx, "task-upsert")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("Status after upsert: got %q, want running", got.Status)
	}
	if got.Iteration != 2 {
		t.Errorf("Iteration after upsert: got %d, want 2", got.Iteration)
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task, got nil")
	}
}

func TestSQLiteStore_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	task := makeTask("task-status")
	store.Save(ctx, task)

	now := time.Now().UTC()
	if err := store.UpdateStatus(ctx, "task-status", StatusCompleted, &now, &now); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, _ := store.Get(ctx, "task-status")
	if got.Status != StatusCompleted {
		t.Errorf("Status: got %q, want completed", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
}

func TestSQLiteStore_UpdateResult(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	store.Save(ctx, makeTask("task-result"))

	changes := &gitpkg.ChangesSummary{FilesModified: 3, FilesCreated: 1}
	usage := &UsageInfo{InputTokens: 100, OutputTokens: 200, DurationSeconds: 42}

	if err := store.UpdateResult(ctx, "task-result", "some output text", changes, usage); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}

	got, _ := store.Get(ctx, "task-result")
	if got.Result != "some output text" {
		t.Errorf("Result: got %q, want 'some output text'", got.Result)
	}
	if got.ChangesSummary == nil || got.ChangesSummary.FilesModified != 3 {
		t.Errorf("ChangesSummary: got %+v", got.ChangesSummary)
	}
	if got.Usage == nil || got.Usage.InputTokens != 100 {
		t.Errorf("Usage: got %+v", got.Usage)
	}
}

func TestSQLiteStore_UpdatePR(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	store.Save(ctx, makeTask("task-pr"))

	if err := store.UpdatePR(ctx, "task-pr", "feature/fix-bug-abc12345", "https://github.com/user/repo/pull/42", 42); err != nil {
		t.Fatalf("UpdatePR: %v", err)
	}

	got, _ := store.Get(ctx, "task-pr")
	if got.Branch != "feature/fix-bug-abc12345" {
		t.Errorf("Branch: got %q", got.Branch)
	}
	if got.PRURL != "https://github.com/user/repo/pull/42" {
		t.Errorf("PRURL: got %q", got.PRURL)
	}
	if got.PRNumber != 42 {
		t.Errorf("PRNumber: got %d", got.PRNumber)
	}
}

func TestSQLiteStore_UpdateError(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	store.Save(ctx, makeTask("task-error"))

	if err := store.UpdateError(ctx, "task-error", "something went wrong"); err != nil {
		t.Fatalf("UpdateError: %v", err)
	}

	got, _ := store.Get(ctx, "task-error")
	if got.Error != "something went wrong" {
		t.Errorf("Error: got %q", got.Error)
	}
}

func TestSQLiteStore_SaveAndGetIterations(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	store.Save(ctx, makeTask("task-iters"))

	now := time.Now().UTC()
	ended := now.Add(30 * time.Second)

	iter1 := Iteration{
		Number:    1,
		Prompt:    "first prompt",
		Result:    "first result",
		Status:    StatusCompleted,
		StartedAt: now,
		EndedAt:   &ended,
	}
	iter2 := Iteration{
		Number:    2,
		Prompt:    "second prompt",
		Error:     "execution failed",
		Status:    StatusFailed,
		StartedAt: ended,
	}

	if err := store.SaveIteration(ctx, "task-iters", iter1); err != nil {
		t.Fatalf("SaveIteration 1: %v", err)
	}
	if err := store.SaveIteration(ctx, "task-iters", iter2); err != nil {
		t.Fatalf("SaveIteration 2: %v", err)
	}

	iters, err := store.GetIterations(ctx, "task-iters")
	if err != nil {
		t.Fatalf("GetIterations: %v", err)
	}
	if len(iters) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(iters))
	}
	if iters[0].Number != 1 || iters[0].Result != "first result" {
		t.Errorf("iter[0]: %+v", iters[0])
	}
	if iters[1].Number != 2 || iters[1].Error != "execution failed" {
		t.Errorf("iter[1]: %+v", iters[1])
	}
	if iters[0].EndedAt == nil {
		t.Error("iter[0].EndedAt should be set")
	}
}

func TestSQLiteStore_SaveIterationUpsert(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	store.Save(ctx, makeTask("task-iter-upsert"))
	now := time.Now().UTC()

	iter := Iteration{Number: 1, Prompt: "original", Status: StatusRunning, StartedAt: now}
	store.SaveIteration(ctx, "task-iter-upsert", iter)

	// Update same iteration
	iter.Status = StatusCompleted
	iter.Result = "done"
	store.SaveIteration(ctx, "task-iter-upsert", iter)

	iters, _ := store.GetIterations(ctx, "task-iter-upsert")
	if len(iters) != 1 {
		t.Fatalf("expected 1 iteration after upsert, got %d", len(iters))
	}
	if iters[0].Status != StatusCompleted || iters[0].Result != "done" {
		t.Errorf("upserted iter: %+v", iters[0])
	}
}

func TestSQLiteStore_List(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	// Insert multiple tasks
	for i := 0; i < 5; i++ {
		task := makeTask("list-task-" + string(rune('a'+i)))
		if i%2 == 0 {
			task.Status = StatusCompleted
		}
		store.Save(ctx, task)
	}

	// List all
	tasks, total, err := store.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(tasks) != 5 {
		t.Errorf("len(tasks): got %d, want 5", len(tasks))
	}

	// Filter by status
	completed, total2, err := store.List(ctx, ListOptions{Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("List (filtered): %v", err)
	}
	if total2 != 3 {
		t.Errorf("filtered total: got %d, want 3", total2)
	}
	if len(completed) != 3 {
		t.Errorf("filtered len: got %d, want 3", len(completed))
	}
}

func TestSQLiteStore_ListPagination(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Save(ctx, makeTask("page-task-"+string(rune('a'+i))))
	}

	page1, total, err := store.List(ctx, ListOptions{Limit: 4, Offset: 0})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if total != 10 {
		t.Errorf("total: got %d, want 10", total)
	}
	if len(page1) != 4 {
		t.Errorf("page1 len: got %d, want 4", len(page1))
	}

	page2, _, err := store.List(ctx, ListOptions{Limit: 4, Offset: 4})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 4 {
		t.Errorf("page2 len: got %d, want 4", len(page2))
	}

	// IDs should not overlap
	ids1 := map[string]bool{}
	for _, ts := range page1 {
		ids1[ts.ID] = true
	}
	for _, ts := range page2 {
		if ids1[ts.ID] {
			t.Errorf("duplicate task %s on both pages", ts.ID)
		}
	}
}

func TestSQLiteStore_PromptTruncatedInList(t *testing.T) {
	db := openTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	longPrompt := string(make([]byte, 300))
	for i := range longPrompt {
		longPrompt = longPrompt[:i] + "x" + longPrompt[i+1:]
	}
	task := makeTask("task-long-prompt")
	task.Prompt = longPrompt
	store.Save(ctx, task)

	tasks, _, _ := store.List(ctx, ListOptions{Limit: 10})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task")
	}
	if len(tasks[0].Prompt) > 203 { // 200 + "..."
		t.Errorf("prompt not truncated: len=%d", len(tasks[0].Prompt))
	}
}
