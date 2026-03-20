package workflow

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

// NewSQLiteRegistry creates a new SQLite-backed workflow registry.
func NewSQLiteRegistry(db *sql.DB) *SQLiteRegistry {
	return &SQLiteRegistry{db: db}
}

func (r *SQLiteRegistry) Create(ctx context.Context, def WorkflowDefinition) error {
	stepsJSON, err := json.Marshal(def.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}
	paramsJSON, err := json.Marshal(def.Parameters)
	if err != nil {
		return fmt.Errorf("marshaling parameters: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO workflow_definitions (name, description, builtin, steps_json, params_json) VALUES (?, ?, ?, ?, ?)`,
		def.Name, def.Description, def.Builtin, string(stepsJSON), string(paramsJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return apperror.Conflict("workflow '%s' already exists", def.Name)
		}
		return fmt.Errorf("creating workflow definition: %w", err)
	}

	return nil
}

func (r *SQLiteRegistry) List(ctx context.Context) ([]WorkflowDefinition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, description, builtin, steps_json, params_json, created_at FROM workflow_definitions ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing workflow definitions: %w", err)
	}
	defer rows.Close()

	defs := make([]WorkflowDefinition, 0)
	for rows.Next() {
		def, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}
	return defs, rows.Err()
}

func (r *SQLiteRegistry) Get(ctx context.Context, name string) (*WorkflowDefinition, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT name, description, builtin, steps_json, params_json, created_at FROM workflow_definitions WHERE name = ?`,
		name,
	)

	var def WorkflowDefinition
	var stepsJSON, paramsJSON, createdAt string
	err := row.Scan(&def.Name, &def.Description, &def.Builtin, &stepsJSON, &paramsJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("workflow '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow definition: %w", err)
	}

	_ = json.Unmarshal([]byte(stepsJSON), &def.Steps)
	_ = json.Unmarshal([]byte(paramsJSON), &def.Parameters)
	def.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)

	return &def, nil
}

func (r *SQLiteRegistry) Delete(ctx context.Context, name string) error {
	// Check if builtin
	var builtin bool
	err := r.db.QueryRowContext(ctx, `SELECT builtin FROM workflow_definitions WHERE name = ?`, name).Scan(&builtin)
	if err == sql.ErrNoRows {
		return apperror.NotFound("workflow '%s' not found", name)
	}
	if err != nil {
		return fmt.Errorf("checking workflow: %w", err)
	}
	if builtin {
		return apperror.Conflict("cannot delete built-in workflow '%s'", name)
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM workflow_definitions WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting workflow: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("workflow '%s' not found", name)
	}

	return nil
}

// DeleteBuiltin removes a built-in workflow definition (used for cleanup of stale builtins on startup).
func (r *SQLiteRegistry) DeleteBuiltin(ctx context.Context, name string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM workflow_definitions WHERE name = ? AND builtin = 1`, name)
	if err != nil {
		return fmt.Errorf("deleting builtin workflow: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("builtin workflow '%s' not found", name)
	}
	return nil
}

// UpdateBuiltin updates an existing built-in workflow definition's steps, params, and description.
func (r *SQLiteRegistry) UpdateBuiltin(ctx context.Context, def WorkflowDefinition) error {
	stepsJSON, err := json.Marshal(def.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}
	paramsJSON, err := json.Marshal(def.Parameters)
	if err != nil {
		return fmt.Errorf("marshaling parameters: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE workflow_definitions SET description = ?, steps_json = ?, params_json = ? WHERE name = ? AND builtin = 1`,
		def.Description, string(stepsJSON), string(paramsJSON), def.Name,
	)
	if err != nil {
		return fmt.Errorf("updating builtin workflow: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("builtin workflow '%s' not found", def.Name)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func (r *SQLiteRegistry) scanRow(row rowScanner) (*WorkflowDefinition, error) {
	var def WorkflowDefinition
	var stepsJSON, paramsJSON, createdAt string
	if err := row.Scan(&def.Name, &def.Description, &def.Builtin, &stepsJSON, &paramsJSON, &createdAt); err != nil {
		return nil, fmt.Errorf("scanning workflow definition: %w", err)
	}
	_ = json.Unmarshal([]byte(stepsJSON), &def.Steps)
	_ = json.Unmarshal([]byte(paramsJSON), &def.Parameters)
	def.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
	return &def, nil
}
