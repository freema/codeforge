package database

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func TestMigrations_AllApply(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Verify all migrations were recorded
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("expected 5 migrations, got %d", count)
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Run twice — should not fail
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
}

func getColumns(t *testing.T, db *sql.DB, table string) map[string]string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		columns[name] = colType
	}
	return columns
}

func TestSchema_KeysTable(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols := getColumns(t, db, "keys")
	required := []string{"id", "name", "provider", "encrypted_token", "scope", "created_at"}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Errorf("keys table missing column %q", col)
		}
	}
}

func TestSchema_SessionsTable(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols := getColumns(t, db, "sessions")
	required := []string{
		"id", "status", "repo_url", "provider_key", "prompt", "session_type",
		"callback_url", "config_json", "result", "error", "changes_json",
		"usage_json", "iteration", "current_prompt", "branch", "pr_number",
		"pr_url", "workflow_run_id", "trace_id", "created_at", "started_at",
		"finished_at", "updated_at", "review_result_json",
	}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Errorf("sessions table missing column %q", col)
		}
	}
}

func TestSchema_WorkflowDefinitionsTable(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols := getColumns(t, db, "workflow_definitions")
	required := []string{"name", "description", "builtin", "steps_json", "params_json", "created_at"}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Errorf("workflow_definitions table missing column %q", col)
		}
	}
}

func TestSchema_MCPServersTable(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols := getColumns(t, db, "mcp_servers")
	required := []string{"id", "scope", "name", "transport", "command", "url", "env", "created_at"}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Errorf("mcp_servers table missing column %q", col)
		}
	}
}

func TestSchema_ToolsTable(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols := getColumns(t, db, "tools")
	required := []string{"id", "scope", "name", "type", "description", "mcp_package", "mcp_command", "builtin", "created_at"}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Errorf("tools table missing column %q", col)
		}
	}
}

func TestSchema_KeysRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	// Insert
	_, err := db.ExecContext(context.Background(),
		"INSERT INTO keys (name, provider, encrypted_token, scope) VALUES (?, ?, ?, ?)",
		"test-key", "github", "encrypted-data", "repo",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Select + Scan — matches what sqlite_registry.go does
	var name, provider, scope, createdAt string
	err = db.QueryRowContext(context.Background(),
		"SELECT name, provider, scope, created_at FROM keys WHERE name = ?", "test-key",
	).Scan(&name, &provider, &scope, &createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if name != "test-key" || provider != "github" || scope != "repo" {
		t.Errorf("roundtrip mismatch: name=%s provider=%s scope=%s", name, provider, scope)
	}
}

func TestSchema_SessionsRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	// Insert — matches what sqlite_store.go does
	now := "2026-03-14T12:00:00.000"
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO sessions (id, status, repo_url, provider_key, prompt, session_type,
			callback_url, config_json, result, error, changes_json, usage_json,
			iteration, current_prompt, branch, pr_number, pr_url,
			workflow_run_id, trace_id, review_result_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"session-1", "pending", "https://github.com/test/repo", "my-key",
		"do something", "code", "", "{}", "", "", "{}", "{}",
		1, "", "", 0, "", "run-1", "", "", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Select + Scan — matches sqlite_store.go Get() query
	var id, status, repoURL, prompt, sessionType, workflowRunID string
	var iteration int
	err = db.QueryRowContext(context.Background(), `
		SELECT id, status, repo_url, prompt, session_type, iteration, workflow_run_id
		FROM sessions WHERE id = ?`, "session-1",
	).Scan(&id, &status, &repoURL, &prompt, &sessionType, &iteration, &workflowRunID)
	if err != nil {
		t.Fatal(err)
	}
	if workflowRunID != "run-1" {
		t.Errorf("workflow_run_id: got %q, want %q", workflowRunID, "run-1")
	}
}
