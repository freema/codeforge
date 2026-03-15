package mcp

import (
	"context"
	"time"
)

// Server defines an MCP server configuration.
// Transport determines the connection type:
//   - "stdio" (default): launches a local process (command + package + args)
//   - "http": connects to a remote HTTP endpoint (url + headers)
type Server struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"` // "stdio" (default) or "http"

	// stdio fields
	Command string            `json:"command,omitempty"` // e.g. "npx", "uvx", "docker"; defaults to "npx"
	Package string            `json:"package,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// http fields
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
}

// IsHTTP returns true if the server uses HTTP transport.
func (s *Server) IsHTTP() bool {
	return s.Transport == "http"
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
