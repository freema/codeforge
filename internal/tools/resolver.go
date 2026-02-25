package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/freema/codeforge/internal/apperror"
)

// Resolver resolves TaskTool requests into ToolInstances by looking up
// definitions in the registry (with built-in fallback) and validating config.
type Resolver struct {
	registry Registry
}

// NewResolver creates a new tool resolver.
func NewResolver(registry Registry) *Resolver {
	return &Resolver{registry: registry}
}

// Resolve converts a list of per-task tool requests into fully resolved ToolInstances.
func (r *Resolver) Resolve(ctx context.Context, projectID string, taskTools []TaskTool) ([]ToolInstance, error) {
	if len(taskTools) == 0 {
		return nil, nil
	}

	instances := make([]ToolInstance, 0, len(taskTools))

	for _, tt := range taskTools {
		def, err := r.findDefinition(ctx, projectID, tt.Name)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", tt.Name, err)
		}

		if err := ValidateConfig(def, tt.Config); err != nil {
			return nil, fmt.Errorf("tool %q: %w", tt.Name, err)
		}

		instances = append(instances, ToolInstance{
			Definition: def,
			Config:     tt.Config,
		})
	}

	return instances, nil
}

// findDefinition looks up a tool definition, trying:
// 1. Project scope in registry
// 2. Global scope in registry
// 3. Built-in catalog
func (r *Resolver) findDefinition(ctx context.Context, projectID, name string) (*ToolDefinition, error) {
	// Try project scope first
	if projectID != "" {
		def, err := r.registry.Get(ctx, projectID, name)
		if err == nil {
			return def, nil
		}
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
			return nil, err
		}
	}

	// Try global scope
	def, err := r.registry.Get(ctx, "global", name)
	if err == nil {
		return def, nil
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || !errors.Is(appErr, apperror.ErrNotFound) {
		return nil, err
	}

	// Fallback to built-in catalog
	if builtin := BuiltinByName(name); builtin != nil {
		return builtin, nil
	}

	return nil, fmt.Errorf("unknown tool %q", name)
}
