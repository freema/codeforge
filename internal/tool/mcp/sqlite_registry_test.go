package mcp

import (
	"context"
	"database/sql"
	"strings"
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

	// Create table matching migrations 002 + 008
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE mcp_servers (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			scope      TEXT NOT NULL DEFAULT 'global',
			transport  TEXT NOT NULL DEFAULT 'stdio',
			command    TEXT NOT NULL DEFAULT '',
			package    TEXT NOT NULL DEFAULT '',
			args       TEXT NOT NULL DEFAULT '[]',
			env        TEXT NOT NULL DEFAULT '{}',
			url        TEXT NOT NULL DEFAULT '',
			headers    TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
			UNIQUE(scope, name)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

func TestSQLiteRegistry_CreateAndResolveGlobal(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	err := reg.CreateGlobal(ctx, Server{
		Name:    "sentry",
		Package: "@sentry/mcp-server",
		Command: "npx",
		Args:    []string{"--org", "my-org"},
		Env:     map[string]string{"SENTRY_TOKEN": "tok"},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv, err := reg.ResolveGlobal(ctx, "sentry")
	if err != nil {
		t.Fatal(err)
	}
	if srv.Name != "sentry" {
		t.Errorf("name: got %q, want %q", srv.Name, "sentry")
	}
	if srv.Package != "@sentry/mcp-server" {
		t.Errorf("package: got %q, want %q", srv.Package, "@sentry/mcp-server")
	}
	if srv.Transport != "stdio" {
		t.Errorf("transport: got %q, want %q", srv.Transport, "stdio")
	}
	if len(srv.Args) != 2 || srv.Args[0] != "--org" {
		t.Errorf("args: got %v, want [--org my-org]", srv.Args)
	}
	if srv.Env["SENTRY_TOKEN"] != "tok" {
		t.Errorf("env: got %v", srv.Env)
	}
}

func TestSQLiteRegistry_CreateHTTPServer(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	err := reg.CreateGlobal(ctx, Server{
		Name:      "remote-mcp",
		Transport: "http",
		URL:       "https://mcp.example.com/sse",
		Headers:   map[string]string{"Authorization": "Bearer tok"},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv, err := reg.ResolveGlobal(ctx, "remote-mcp")
	if err != nil {
		t.Fatal(err)
	}
	if srv.Transport != "http" {
		t.Errorf("transport: got %q, want %q", srv.Transport, "http")
	}
	if srv.URL != "https://mcp.example.com/sse" {
		t.Errorf("url: got %q", srv.URL)
	}
	if srv.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("headers: got %v", srv.Headers)
	}
}

func TestSQLiteRegistry_ListGlobal(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	_ = reg.CreateGlobal(ctx, Server{Name: "srv-a", Package: "pkg-a"})
	_ = reg.CreateGlobal(ctx, Server{Name: "srv-b", Package: "pkg-b"})

	servers, err := reg.ListGlobal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestSQLiteRegistry_DeleteGlobal(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	_ = reg.CreateGlobal(ctx, Server{Name: "to-delete", Package: "pkg"})

	err := reg.DeleteGlobal(ctx, "to-delete")
	if err != nil {
		t.Fatal(err)
	}

	_, err = reg.ResolveGlobal(ctx, "to-delete")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestSQLiteRegistry_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	err := reg.DeleteGlobal(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestSQLiteRegistry_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	_ = reg.CreateGlobal(ctx, Server{Name: "dup", Package: "pkg"})
	err := reg.CreateGlobal(ctx, Server{Name: "dup", Package: "pkg2"})

	if err == nil {
		t.Fatal("expected conflict error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestSQLiteRegistry_ProjectScope(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Create in project scope
	err := reg.CreateProject(ctx, "project-1", Server{Name: "proj-srv", Package: "pkg"})
	if err != nil {
		t.Fatal(err)
	}

	// List project — should have 1
	servers, err := reg.ListProject(ctx, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 project server, got %d", len(servers))
	}

	// List global — should have 0
	global, err := reg.ListGlobal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(global) != 0 {
		t.Errorf("expected 0 global servers, got %d", len(global))
	}

	// Delete project server
	err = reg.DeleteProject(ctx, "project-1", "proj-srv")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRegistry_ResolveMCPServers_Merge(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Global server
	_ = reg.CreateGlobal(ctx, Server{Name: "sentry", Package: "global-sentry"})
	_ = reg.CreateGlobal(ctx, Server{Name: "jira", Package: "global-jira"})

	// Project server overrides sentry
	_ = reg.CreateProject(ctx, "proj-1", Server{Name: "sentry", Package: "project-sentry"})

	// Task server overrides jira
	taskServers := []Server{
		{Name: "jira", Package: "task-jira"},
	}

	merged, err := reg.ResolveMCPServers(ctx, "proj-1", taskServers)
	if err != nil {
		t.Fatal(err)
	}

	if len(merged) != 2 {
		t.Fatalf("expected 2 merged servers, got %d", len(merged))
	}

	// Find each server and verify override
	byName := make(map[string]Server)
	for _, s := range merged {
		byName[s.Name] = s
	}

	if byName["sentry"].Package != "project-sentry" {
		t.Errorf("sentry should be overridden by project: got %q", byName["sentry"].Package)
	}
	if byName["jira"].Package != "task-jira" {
		t.Errorf("jira should be overridden by task: got %q", byName["jira"].Package)
	}
}

func TestSQLiteRegistry_DefaultTransport(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	ctx := context.Background()

	// Create with empty transport — should default to "stdio"
	err := reg.CreateGlobal(ctx, Server{Name: "test", Package: "pkg"})
	if err != nil {
		t.Fatal(err)
	}

	srv, err := reg.ResolveGlobal(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if srv.Transport != "stdio" {
		t.Errorf("transport: got %q, want %q", srv.Transport, "stdio")
	}
}
