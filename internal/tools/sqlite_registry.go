package tools

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

// NewSQLiteRegistry creates a new SQLite-backed tool registry.
func NewSQLiteRegistry(db *sql.DB) *SQLiteRegistry {
	return &SQLiteRegistry{db: db}
}

func (r *SQLiteRegistry) Create(ctx context.Context, scope string, def ToolDefinition) error {
	argsJSON, _ := json.Marshal(def.MCPArgs)
	reqJSON, _ := json.Marshal(def.RequiredConfig)
	optJSON, _ := json.Marshal(def.OptionalConfig)
	capJSON, _ := json.Marshal(def.Capabilities)

	builtin := 0
	if def.Builtin {
		builtin = 1
	}

	transport := def.MCPTransport
	if transport == "" {
		transport = "stdio"
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tools (name, scope, type, description, version, mcp_transport, mcp_url, mcp_package, mcp_command, mcp_args, required_config, optional_config, capabilities, builtin)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		def.Name, scope, string(def.Type), def.Description, def.Version,
		transport, def.MCPURL, def.MCPPackage, def.MCPCommand,
		string(argsJSON), string(reqJSON), string(optJSON), string(capJSON),
		builtin,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return apperror.Conflict("tool '%s' already exists in scope '%s'", def.Name, scope)
		}
		return fmt.Errorf("storing tool: %w", err)
	}

	return nil
}

func (r *SQLiteRegistry) Get(ctx context.Context, scope, name string) (*ToolDefinition, error) {
	var def ToolDefinition
	var toolType, argsJSON, reqJSON, optJSON, capJSON, createdAt string
	var builtinInt int

	err := r.db.QueryRowContext(ctx,
		`SELECT name, type, description, version, mcp_transport, mcp_url, mcp_package, mcp_command, mcp_args, required_config, optional_config, capabilities, builtin, created_at
		 FROM tools WHERE scope = ? AND name = ?`,
		scope, name,
	).Scan(
		&def.Name, &toolType, &def.Description, &def.Version,
		&def.MCPTransport, &def.MCPURL, &def.MCPPackage, &def.MCPCommand,
		&argsJSON, &reqJSON, &optJSON, &capJSON,
		&builtinInt, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("tool '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("getting tool: %w", err)
	}

	def.Type = ToolType(toolType)
	def.Builtin = builtinInt == 1
	def.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)

	_ = json.Unmarshal([]byte(argsJSON), &def.MCPArgs)
	_ = json.Unmarshal([]byte(reqJSON), &def.RequiredConfig)
	_ = json.Unmarshal([]byte(optJSON), &def.OptionalConfig)
	_ = json.Unmarshal([]byte(capJSON), &def.Capabilities)

	return &def, nil
}

func (r *SQLiteRegistry) List(ctx context.Context, scope string) ([]ToolDefinition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, type, description, version, mcp_transport, mcp_url, mcp_package, mcp_command, mcp_args, required_config, optional_config, capabilities, builtin, created_at
		 FROM tools WHERE scope = ? ORDER BY created_at`,
		scope,
	)
	if err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}
	defer rows.Close()

	tools := make([]ToolDefinition, 0)
	for rows.Next() {
		var def ToolDefinition
		var toolType, argsJSON, reqJSON, optJSON, capJSON, createdAt string
		var builtinInt int

		if err := rows.Scan(
			&def.Name, &toolType, &def.Description, &def.Version,
			&def.MCPTransport, &def.MCPURL, &def.MCPPackage, &def.MCPCommand,
			&argsJSON, &reqJSON, &optJSON, &capJSON,
			&builtinInt, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tool: %w", err)
		}

		def.Type = ToolType(toolType)
		def.Builtin = builtinInt == 1
		def.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)

		_ = json.Unmarshal([]byte(argsJSON), &def.MCPArgs)
		_ = json.Unmarshal([]byte(reqJSON), &def.RequiredConfig)
		_ = json.Unmarshal([]byte(optJSON), &def.OptionalConfig)
		_ = json.Unmarshal([]byte(capJSON), &def.Capabilities)

		tools = append(tools, def)
	}

	return tools, rows.Err()
}

func (r *SQLiteRegistry) Delete(ctx context.Context, scope, name string) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM tools WHERE scope = ? AND name = ?",
		scope, name,
	)
	if err != nil {
		return fmt.Errorf("deleting tool: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("tool '%s' not found", name)
	}

	return nil
}
