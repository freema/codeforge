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

// WorkflowConfig is a saved workflow configuration that can be run later.
type WorkflowConfig struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	Workflow       string            `json:"workflow"` // template name e.g. "sentry-fixer"
	Params         map[string]string `json:"params"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"` // 0 = use default
	CreatedAt      time.Time         `json:"created_at"`
}

// ConfigStore persists workflow configurations.
type ConfigStore interface {
	Create(ctx context.Context, cfg WorkflowConfig) (int64, error)
	List(ctx context.Context) ([]WorkflowConfig, error)
	Get(ctx context.Context, id int) (*WorkflowConfig, error)
	Delete(ctx context.Context, id int) error
}

// SQLiteConfigStore implements ConfigStore backed by SQLite.
type SQLiteConfigStore struct {
	db *sql.DB
}

// NewSQLiteConfigStore creates a new SQLite-backed workflow config store.
func NewSQLiteConfigStore(db *sql.DB) *SQLiteConfigStore {
	return &SQLiteConfigStore{db: db}
}

// Create inserts a new workflow config and returns the auto-generated ID.
func (s *SQLiteConfigStore) Create(ctx context.Context, cfg WorkflowConfig) (int64, error) {
	paramsJSON, err := json.Marshal(cfg.Params)
	if err != nil {
		return 0, fmt.Errorf("marshaling params: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_configs (name, workflow, params, timeout_seconds) VALUES (?, ?, ?, ?)`,
		cfg.Name, cfg.Workflow, string(paramsJSON), cfg.TimeoutSeconds,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, apperror.Conflict("workflow config '%s' already exists", cfg.Name)
		}
		return 0, fmt.Errorf("creating workflow config: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return id, nil
}

// List returns all workflow configs ordered by creation time.
func (s *SQLiteConfigStore) List(ctx context.Context) ([]WorkflowConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, workflow, params, timeout_seconds, created_at FROM workflow_configs ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing workflow configs: %w", err)
	}
	defer rows.Close()

	configs := make([]WorkflowConfig, 0)
	for rows.Next() {
		cfg, err := scanConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, *cfg)
	}
	return configs, rows.Err()
}

// Get returns a single workflow config by ID.
func (s *SQLiteConfigStore) Get(ctx context.Context, id int) (*WorkflowConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, workflow, params, timeout_seconds, created_at FROM workflow_configs WHERE id = ?`,
		id,
	)

	var cfg WorkflowConfig
	var paramsJSON, createdAt string
	err := row.Scan(&cfg.ID, &cfg.Name, &cfg.Workflow, &paramsJSON, &cfg.TimeoutSeconds, &createdAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NotFound("workflow config %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow config: %w", err)
	}

	_ = json.Unmarshal([]byte(paramsJSON), &cfg.Params)
	cfg.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)

	return &cfg, nil
}

// Delete removes a workflow config by ID.
func (s *SQLiteConfigStore) Delete(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workflow_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting workflow config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NotFound("workflow config %d not found", id)
	}

	return nil
}

// DeleteByWorkflow removes all configs referencing a given workflow name.
func (s *SQLiteConfigStore) DeleteByWorkflow(ctx context.Context, workflowName string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workflow_configs WHERE workflow = ?`, workflowName)
	if err != nil {
		return 0, fmt.Errorf("deleting configs for workflow %q: %w", workflowName, err)
	}
	return result.RowsAffected()
}

type configRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanConfigRow(row configRowScanner) (*WorkflowConfig, error) {
	var cfg WorkflowConfig
	var paramsJSON, createdAt string
	if err := row.Scan(&cfg.ID, &cfg.Name, &cfg.Workflow, &paramsJSON, &cfg.TimeoutSeconds, &createdAt); err != nil {
		return nil, fmt.Errorf("scanning workflow config: %w", err)
	}
	_ = json.Unmarshal([]byte(paramsJSON), &cfg.Params)
	cfg.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000", createdAt)
	return &cfg, nil
}
