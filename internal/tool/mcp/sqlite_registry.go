package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/apperror"
)

// SQLiteRegistry implements Registry backed by SQLite.
type SQLiteRegistry struct {
	db *sql.DB
}

// NewSQLiteRegistry creates a new SQLite-backed MCP registry.
func NewSQLiteRegistry(db *sql.DB) *SQLiteRegistry {
	return &SQLiteRegistry{db: db}
}

func (r *SQLiteRegistry) CreateGlobal(ctx context.Context, srv Server) error {
	return r.create(ctx, "global", srv)
}

func (r *SQLiteRegistry) ListGlobal(ctx context.Context) ([]Server, error) {
	return r.list(ctx, "global")
}

func (r *SQLiteRegistry) DeleteGlobal(ctx context.Context, name string) error {
	return r.delete(ctx, "global", name)
}

func (r *SQLiteRegistry) ResolveGlobal(ctx context.Context, name string) (*Server, error) {
	return r.get(ctx, "global", name)
}

func (r *SQLiteRegistry) CreateProject(ctx context.Context, projectID string, srv Server) error {
	return r.create(ctx, projectID, srv)
}

func (r *SQLiteRegistry) ListProject(ctx context.Context, projectID string) ([]Server, error) {
	return r.list(ctx, projectID)
}

func (r *SQLiteRegistry) DeleteProject(ctx context.Context, projectID string, name string) error {
	return r.delete(ctx, projectID, name)
}

func (r *SQLiteRegistry) ResolveMCPServers(ctx context.Context, projectID string, taskServers []Server) ([]Server, error) {
	globalServers, err := r.ListGlobal(ctx)
	if err != nil {
		globalServers = nil
	}

	var projectServers []Server
	if projectID != "" {
		projectServers, _ = r.ListProject(ctx, projectID)
	}

	return mergeServers(globalServers, projectServers, taskServers), nil
}

func (r *SQLiteRegistry) create(ctx context.Context, scope string, srv Server) error {
	argsJSON, _ := json.Marshal(srv.Args)
	envJSON, _ := json.Marshal(srv.Env)

	_, err := r.db.ExecContext(ctx,
		"INSERT INTO mcp_servers (name, scope, package, args, env) VALUES (?, ?, ?, ?, ?)",
		srv.Name, scope, srv.Package, string(argsJSON), string(envJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return apperror.Conflict("MCP server '%s' already exists", srv.Name)
		}
		return fmt.Errorf("storing MCP server: %w", err)
	}

	return nil
}

func (r *SQLiteRegistry) get(ctx context.Context, scope, name string) (*Server, error) {
	var srv Server
	var argsJSON, envJSON, createdAt string
	err := r.db.QueryRowContext(ctx,
		"SELECT name, package, args, env, created_at FROM mcp_servers WHERE scope = ? AND name = ?",
		scope, name,
	).Scan(&srv.Name, &srv.Package, &argsJSON, &envJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("MCP server not found")
	}
	if err != nil {
		return nil, fmt.Errorf("getting MCP server: %w", err)
	}

	_ = json.Unmarshal([]byte(argsJSON), &srv.Args)
	_ = json.Unmarshal([]byte(envJSON), &srv.Env)
	srv.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)

	return &srv, nil
}

func (r *SQLiteRegistry) list(ctx context.Context, scope string) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT name, package, args, env, created_at FROM mcp_servers WHERE scope = ? ORDER BY created_at",
		scope,
	)
	if err != nil {
		return nil, fmt.Errorf("listing MCP servers: %w", err)
	}
	defer rows.Close()

	servers := make([]Server, 0)
	for rows.Next() {
		var srv Server
		var argsJSON, envJSON, createdAt string
		if err := rows.Scan(&srv.Name, &srv.Package, &argsJSON, &envJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning MCP server: %w", err)
		}
		_ = json.Unmarshal([]byte(argsJSON), &srv.Args)
		_ = json.Unmarshal([]byte(envJSON), &srv.Env)
		srv.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
		servers = append(servers, srv)
	}

	return servers, rows.Err()
}

func (r *SQLiteRegistry) delete(ctx context.Context, scope, name string) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM mcp_servers WHERE scope = ? AND name = ?",
		scope, name,
	)
	if err != nil {
		return fmt.Errorf("deleting MCP server: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("MCP server '%s' not found", name)
	}

	return nil
}
