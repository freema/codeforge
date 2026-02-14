package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/redisclient"
)

// Server defines an MCP server configuration.
type Server struct {
	Name      string            `json:"name"`
	Package   string            `json:"package"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
}

// Registry manages MCP server configurations in Redis.
type Registry struct {
	redis *redisclient.Client
}

// NewRegistry creates a new MCP registry.
func NewRegistry(redis *redisclient.Client) *Registry {
	return &Registry{redis: redis}
}

// --- Global MCP Servers ---

// CreateGlobal registers a global MCP server.
func (r *Registry) CreateGlobal(ctx context.Context, srv Server) error {
	return r.create(ctx, r.globalKey(srv.Name), r.globalIndexKey(), srv)
}

// ListGlobal returns all global MCP servers.
func (r *Registry) ListGlobal(ctx context.Context) ([]Server, error) {
	return r.list(ctx, r.globalIndexKey(), "mcp", "global")
}

// DeleteGlobal removes a global MCP server.
func (r *Registry) DeleteGlobal(ctx context.Context, name string) error {
	return r.delete(ctx, r.globalKey(name), r.globalIndexKey(), name)
}

// ResolveGlobal returns a specific global MCP server.
func (r *Registry) ResolveGlobal(ctx context.Context, name string) (*Server, error) {
	return r.get(ctx, r.globalKey(name))
}

// --- Per-Project MCP Servers ---

// CreateProject registers an MCP server for a specific project.
func (r *Registry) CreateProject(ctx context.Context, projectID string, srv Server) error {
	return r.create(ctx, r.projectKey(projectID, srv.Name), r.projectIndexKey(projectID), srv)
}

// ListProject returns all MCP servers for a project.
func (r *Registry) ListProject(ctx context.Context, projectID string) ([]Server, error) {
	return r.list(ctx, r.projectIndexKey(projectID), "mcp", "project", projectID)
}

// DeleteProject removes an MCP server from a project.
func (r *Registry) DeleteProject(ctx context.Context, projectID string, name string) error {
	return r.delete(ctx, r.projectKey(projectID, name), r.projectIndexKey(projectID), name)
}

// --- Resolution ---

// ResolveMCPServers returns merged MCP servers for a task.
// Merge order (later overrides earlier by name): global → project → task-level.
func (r *Registry) ResolveMCPServers(ctx context.Context, projectID string, taskServers []Server) ([]Server, error) {
	globalServers, err := r.ListGlobal(ctx)
	if err != nil {
		globalServers = nil // continue without global
	}

	var projectServers []Server
	if projectID != "" {
		projectServers, _ = r.ListProject(ctx, projectID)
	}

	return mergeServers(globalServers, projectServers, taskServers), nil
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

// --- Internal helpers ---

func (r *Registry) create(ctx context.Context, redisKey, indexKey string, srv Server) error {
	// Check uniqueness
	exists, err := r.redis.Unwrap().Exists(ctx, redisKey).Result()
	if err != nil {
		return fmt.Errorf("checking MCP server existence: %w", err)
	}
	if exists > 0 {
		return apperror.Conflict("MCP server '%s' already exists", srv.Name)
	}

	argsJSON, _ := json.Marshal(srv.Args)
	envJSON, _ := json.Marshal(srv.Env)

	fields := map[string]interface{}{
		"name":       srv.Name,
		"package":    srv.Package,
		"args":       string(argsJSON),
		"env":        string(envJSON),
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
	}

	pipe := r.redis.Unwrap().Pipeline()
	pipe.HSet(ctx, redisKey, fields)
	pipe.SAdd(ctx, indexKey, srv.Name)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("storing MCP server: %w", err)
	}

	return nil
}

func (r *Registry) get(ctx context.Context, redisKey string) (*Server, error) {
	fields, err := r.redis.Unwrap().HGetAll(ctx, redisKey).Result()
	if err == redis.Nil || len(fields) == 0 {
		return nil, apperror.NotFound("MCP server not found")
	}
	if err != nil {
		return nil, fmt.Errorf("getting MCP server: %w", err)
	}
	return hashToServer(fields), nil
}

func (r *Registry) list(ctx context.Context, indexKey string, keyParts ...string) ([]Server, error) {
	names, err := r.redis.Unwrap().SMembers(ctx, indexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("listing MCP servers: %w", err)
	}

	servers := make([]Server, 0, len(names))
	for _, name := range names {
		parts := append(keyParts, name)
		redisKey := r.redis.Key(parts...)
		fields, err := r.redis.Unwrap().HGetAll(ctx, redisKey).Result()
		if err != nil || len(fields) == 0 {
			continue
		}
		servers = append(servers, *hashToServer(fields))
	}
	return servers, nil
}

func (r *Registry) delete(ctx context.Context, redisKey, indexKey, name string) error {
	pipe := r.redis.Unwrap().Pipeline()
	pipe.Del(ctx, redisKey)
	pipe.SRem(ctx, indexKey, name)
	results, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting MCP server: %w", err)
	}

	// Check if the DEL actually deleted anything
	if deleted, _ := results[0].(*redis.IntCmd).Result(); deleted == 0 {
		return apperror.NotFound("MCP server '%s' not found", name)
	}
	return nil
}

func hashToServer(fields map[string]string) *Server {
	srv := &Server{
		Name:    fields["name"],
		Package: fields["package"],
	}

	if v := fields["args"]; v != "" {
		_ = json.Unmarshal([]byte(v), &srv.Args)
	}
	if v := fields["env"]; v != "" {
		_ = json.Unmarshal([]byte(v), &srv.Env)
	}
	if v := fields["created_at"]; v != "" {
		srv.CreatedAt, _ = time.Parse(time.RFC3339Nano, v)
	}

	return srv
}

func (r *Registry) globalKey(name string) string {
	return r.redis.Key("mcp", "global", name)
}

func (r *Registry) globalIndexKey() string {
	return r.redis.Key("mcp", "global", "_index")
}

func (r *Registry) projectKey(projectID, name string) string {
	return r.redis.Key("mcp", "project", projectID, name)
}

func (r *Registry) projectIndexKey(projectID string) string {
	return r.redis.Key("mcp", "project", projectID, "_index")
}
