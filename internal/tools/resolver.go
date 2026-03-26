package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/keys"
)

// Resolver resolves SessionTool requests into ToolInstances by looking up
// definitions in the registry (with built-in fallback) and validating config.
type Resolver struct {
	registry Registry
	keys     keys.Registry
}

// NewResolver creates a new tool resolver.
func NewResolver(registry Registry, keyReg ...keys.Registry) *Resolver {
	r := &Resolver{registry: registry}
	if len(keyReg) > 0 {
		r.keys = keyReg[0]
	}
	return r
}

// Resolve converts a list of per-session tool requests into fully resolved ToolInstances.
func (r *Resolver) Resolve(ctx context.Context, projectID string, sessionTools []SessionTool) ([]ToolInstance, error) {
	if len(sessionTools) == 0 {
		return nil, nil
	}

	instances := make([]ToolInstance, 0, len(sessionTools))

	for _, tt := range sessionTools {
		def, err := r.findDefinition(ctx, projectID, tt.Name)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", tt.Name, err)
		}

		// Auto-fill missing config from Provider Keys
		config := r.autoFillConfig(ctx, def, tt.Config)

		if err := ValidateConfig(def, config); err != nil {
			return nil, fmt.Errorf("tool %q: %w", tt.Name, err)
		}

		instances = append(instances, ToolInstance{
			Definition: def,
			Config:     config,
		})
	}

	return instances, nil
}

// autoFillConfig fills missing required config fields by looking up Provider Keys.
// If a ConfigField has ProviderKey set and the user didn't supply a value,
// we try to resolve the token from the key registry.
func (r *Resolver) autoFillConfig(ctx context.Context, def *ToolDefinition, userConfig map[string]string) map[string]string {
	if r.keys == nil {
		return userConfig
	}

	config := make(map[string]string)
	for k, v := range userConfig {
		config[k] = v
	}

	for _, f := range def.RequiredConfig {
		if f.ProviderKey == "" {
			continue
		}
		// Skip if user already provided this field
		if v, ok := config[f.Name]; ok && v != "" {
			continue
		}
		// Try to resolve from key registry
		token, _, err := r.keys.ResolveByName(ctx, f.ProviderKey+"-env")
		if err != nil {
			// Try DB keys: find any key with matching provider
			allKeys, listErr := r.keys.List(ctx)
			if listErr == nil {
				for _, k := range allKeys {
					if k.Provider == f.ProviderKey {
						if resolved, _, resolveErr := r.keys.ResolveByName(ctx, k.Name); resolveErr == nil {
							token = resolved
							break
						}
					}
				}
			}
		}
		if token != "" {
			config[f.Name] = token
			slog.Info("auto-filled tool config from provider key", "tool", def.Name, "field", f.Name, "provider", f.ProviderKey)
		}
	}

	return config
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
