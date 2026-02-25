package tools

import "context"

// Registry manages tool definitions scoped by project or global.
type Registry interface {
	Create(ctx context.Context, scope string, def ToolDefinition) error
	Get(ctx context.Context, scope string, name string) (*ToolDefinition, error)
	List(ctx context.Context, scope string) ([]ToolDefinition, error)
	Delete(ctx context.Context, scope string, name string) error
}
