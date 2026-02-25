package mcp

import (
	"context"
	"time"
)

// Server defines an MCP server configuration.
type Server struct {
	Name      string            `json:"name"`
	Package   string            `json:"package"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
}

// Registry manages MCP server configurations.
type Registry interface {
	CreateGlobal(ctx context.Context, srv Server) error
	ListGlobal(ctx context.Context) ([]Server, error)
	DeleteGlobal(ctx context.Context, name string) error
	ResolveGlobal(ctx context.Context, name string) (*Server, error)
	CreateProject(ctx context.Context, projectID string, srv Server) error
	ListProject(ctx context.Context, projectID string) ([]Server, error)
	DeleteProject(ctx context.Context, projectID string, name string) error
	ResolveMCPServers(ctx context.Context, projectID string, taskServers []Server) ([]Server, error)
}

// mergeServers merges server lists, later entries override earlier by name.
func mergeServers(layers ...[]Server) []Server {
	byName := make(map[string]Server)
	var order []string

	for _, layer := range layers {
		for _, srv := range layer {
			if _, exists := byName[srv.Name]; !exists {
				order = append(order, srv.Name)
			}
			byName[srv.Name] = srv
		}
	}

	result := make([]Server, 0, len(byName))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}
