package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
)

// Workspace holds metadata about a task workspace.
type Workspace struct {
	TaskID    string    `json:"task_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	TTL       int64     `json:"ttl"`        // seconds
	SizeBytes int64     `json:"size_bytes"`
}

// IsExpired checks if the workspace TTL has elapsed.
func (w *Workspace) IsExpired() bool {
	return time.Since(w.CreatedAt) > time.Duration(w.TTL)*time.Second
}

// ExpiresAt returns the expiration time.
func (w *Workspace) ExpiresAt() time.Time {
	return w.CreatedAt.Add(time.Duration(w.TTL) * time.Second)
}

// Manager manages workspace directories and their Redis metadata.
type Manager struct {
	basePath string
	redis    *redisclient.Client
	ttl      time.Duration
}

// NewManager creates a new workspace manager.
func NewManager(basePath string, redis *redisclient.Client, ttl time.Duration) *Manager {
	return &Manager{
		basePath: basePath,
		redis:    redis,
		ttl:      ttl,
	}
}

// BasePath returns the workspace base directory.
func (m *Manager) BasePath() string {
	return m.basePath
}

// Create creates a workspace directory and registers it in Redis.
func (m *Manager) Create(ctx context.Context, taskID string) (*Workspace, error) {
	wsPath := filepath.Join(m.basePath, taskID)

	if err := os.MkdirAll(wsPath, 0755); err != nil {
		return nil, fmt.Errorf("creating workspace directory: %w", err)
	}

	ws := &Workspace{
		TaskID:    taskID,
		Path:      wsPath,
		CreatedAt: time.Now().UTC(),
		TTL:       int64(m.ttl.Seconds()),
	}

	fields := map[string]interface{}{
		"task_id":    ws.TaskID,
		"path":       ws.Path,
		"created_at": ws.CreatedAt.Format(time.RFC3339Nano),
		"ttl":        ws.TTL,
		"size_bytes": 0,
	}

	redisKey := m.redisKey(taskID)
	if err := m.redis.Unwrap().HSet(ctx, redisKey, fields).Err(); err != nil {
		return nil, fmt.Errorf("registering workspace in redis: %w", err)
	}

	return ws, nil
}

// Get retrieves workspace metadata from Redis. Returns nil if not found.
func (m *Manager) Get(ctx context.Context, taskID string) *Workspace {
	redisKey := m.redisKey(taskID)
	fields, err := m.redis.Unwrap().HGetAll(ctx, redisKey).Result()
	if err != nil || len(fields) == 0 {
		return nil
	}
	return hashToWorkspace(fields)
}

// Exists checks if the workspace directory exists on disk.
func (m *Manager) Exists(taskID string) bool {
	wsPath := filepath.Join(m.basePath, taskID)
	_, err := os.Stat(wsPath)
	return err == nil
}

// Delete removes a workspace directory and its Redis metadata.
// Validates path is inside basePath to prevent path traversal.
func (m *Manager) Delete(ctx context.Context, taskID string) error {
	wsPath := filepath.Join(m.basePath, taskID)

	// SECURITY: validate path is inside workspace_base
	absPath, err := filepath.Abs(wsPath)
	if err != nil {
		return fmt.Errorf("resolving workspace path: %w", err)
	}
	absBase, err := filepath.Abs(m.basePath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		return fmt.Errorf("path traversal attempt: %s is outside workspace base %s", absPath, absBase)
	}

	if err := os.RemoveAll(absPath); err != nil {
		slog.Warn("failed to remove workspace directory", "path", absPath, "error", err)
	}

	m.redis.Unwrap().Del(ctx, m.redisKey(taskID))
	return nil
}

// UpdateSize calculates and stores the workspace size.
func (m *Manager) UpdateSize(ctx context.Context, taskID string) (int64, error) {
	wsPath := filepath.Join(m.basePath, taskID)
	size, err := DirSize(wsPath)
	if err != nil {
		return 0, err
	}

	m.redis.Unwrap().HSet(ctx, m.redisKey(taskID), "size_bytes", size)
	return size, nil
}

// List returns all tracked workspaces.
func (m *Manager) List(ctx context.Context) ([]Workspace, error) {
	pattern := m.redis.Key("workspace", "*")
	var workspaces []Workspace

	iter := m.redis.Unwrap().Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		redisKey := iter.Val()
		// Skip index keys or other patterns
		if strings.HasSuffix(redisKey, "_index") {
			continue
		}

		fields, err := m.redis.Unwrap().HGetAll(ctx, redisKey).Result()
		if err != nil || len(fields) == 0 {
			continue
		}

		ws := hashToWorkspace(fields)
		if ws != nil {
			workspaces = append(workspaces, *ws)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scanning workspaces: %w", err)
	}

	if workspaces == nil {
		workspaces = []Workspace{}
	}
	return workspaces, nil
}

// TotalSizeBytes returns the sum of all tracked workspace sizes.
func (m *Manager) TotalSizeBytes(ctx context.Context) int64 {
	workspaces, err := m.List(ctx)
	if err != nil {
		return 0
	}
	var total int64
	for _, ws := range workspaces {
		total += ws.SizeBytes
	}
	return total
}

// DirSize calculates the total size of a directory recursively.
func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return nil // skip files we can't stat
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func (m *Manager) redisKey(taskID string) string {
	return m.redis.Key("workspace", taskID)
}

func hashToWorkspace(fields map[string]string) *Workspace {
	ws := &Workspace{
		TaskID: fields["task_id"],
		Path:   fields["path"],
	}
	if v := fields["created_at"]; v != "" {
		ws.CreatedAt, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v := fields["ttl"]; v != "" {
		ws.TTL, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := fields["size_bytes"]; v != "" {
		ws.SizeBytes, _ = strconv.ParseInt(v, 10, 64)
	}
	return ws
}
