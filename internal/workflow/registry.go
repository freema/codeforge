package workflow

import "context"

// Registry manages workflow definitions.
type Registry interface {
	Create(ctx context.Context, def WorkflowDefinition) error
	List(ctx context.Context) ([]WorkflowDefinition, error)
	Get(ctx context.Context, name string) (*WorkflowDefinition, error)
	Delete(ctx context.Context, name string) error
	DeleteBuiltin(ctx context.Context, name string) error  // for cleanup of stale builtins
	UpdateBuiltin(ctx context.Context, def WorkflowDefinition) error // for updating existing builtins on startup
}
