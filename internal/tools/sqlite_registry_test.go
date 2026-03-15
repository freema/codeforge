package tools

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/apperror"
)

const createToolsTable = `
CREATE TABLE IF NOT EXISTS tools (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	scope TEXT NOT NULL DEFAULT 'global',
	type TEXT NOT NULL DEFAULT 'mcp',
	description TEXT,
	version TEXT,
	mcp_transport TEXT DEFAULT 'stdio',
	mcp_url TEXT,
	mcp_package TEXT,
	mcp_command TEXT,
	mcp_args TEXT DEFAULT '[]',
	required_config TEXT DEFAULT '[]',
	optional_config TEXT DEFAULT '[]',
	capabilities TEXT DEFAULT '[]',
	builtin INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(scope, name)
);`

func setupToolsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(context.Background(), createToolsTable); err != nil {
		t.Fatalf("creating tools table: %v", err)
	}
	return db
}

func TestSQLiteRegistryDelete_UserTool(t *testing.T) {
	db := setupToolsDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Insert a non-builtin (user) tool.
	def := ToolDefinition{
		Name:        "my-tool",
		Type:        ToolTypeMCP,
		Description: "a user-defined tool",
		MCPCommand:  "npx",
		MCPArgs:     []string{"--stdio"},
		Builtin:     false,
	}
	if err := reg.Create(ctx, "global", def); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete should succeed.
	if err := reg.Delete(ctx, "global", "my-tool"); err != nil {
		t.Fatalf("Delete user tool: expected no error, got %v", err)
	}

	// Verify the tool is gone.
	_, err := reg.Get(ctx, "global", "my-tool")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected NotFound after deletion, got: %v", err)
	}
}

func TestSQLiteRegistryDelete_BuiltinTool(t *testing.T) {
	db := setupToolsDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Insert a built-in tool.
	def := ToolDefinition{
		Name:        "sentry",
		Type:        ToolTypeMCP,
		Description: "Sentry error tracking",
		MCPPackage:  "@sentry/mcp-server",
		MCPCommand:  "npx",
		MCPArgs:     []string{"--stdio"},
		Builtin:     true,
	}
	if err := reg.Create(ctx, "global", def); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attempt to delete a built-in tool should return a validation error.
	err := reg.Delete(ctx, "global", "sentry")
	if err == nil {
		t.Fatal("expected error when deleting built-in tool, got nil")
	}

	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if !errors.Is(appErr, apperror.ErrValidation) {
		t.Fatalf("expected ErrValidation, got: %v", err)
	}

	// Verify the tool still exists.
	got, err := reg.Get(ctx, "global", "sentry")
	if err != nil {
		t.Fatalf("Get after failed delete: %v", err)
	}
	if got.Name != "sentry" {
		t.Fatalf("expected tool name 'sentry', got %q", got.Name)
	}
}

func TestSQLiteRegistryDelete_NotFound(t *testing.T) {
	db := setupToolsDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Attempt to delete a tool that does not exist.
	err := reg.Delete(ctx, "global", "nonexistent")
	if err == nil {
		t.Fatal("expected error when deleting nonexistent tool, got nil")
	}

	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if !errors.Is(appErr, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}
