# Phase 5 — Workspace Management (v0.6.0)

> Persistent workspaces with lifecycle management.

---

## Task 5.1: Workspace Creation & Tracking

**Priority:** P0
**Files:** `internal/workspace/manager.go`

### Description

Create isolated workspace directories for each task, track them in Redis for lifecycle management.

### Acceptance Criteria

- [ ] Workspace created at: `{workspace_base}/{task_id}/`
- [ ] Workspace base directory from config (`tasks.workspace_base`)
- [ ] Directory created with proper permissions (0755)
- [ ] Workspace metadata stored in Redis: `workspace:{task_id}` → `{path, created_at, ttl, size_bytes}`
- [ ] `Get(taskID)` returns workspace info or nil if not exists
- [ ] `Create(taskID)` creates directory and registers in Redis
- [ ] `Delete(taskID)` removes directory and Redis entry
- [ ] **Path traversal guard:** `Delete` validates resolved path is inside `workspace_base` before `os.RemoveAll`
- [ ] Workspace TTL set from config (`tasks.workspace_ttl`)

### Implementation Notes

```go
type Workspace struct {
    TaskID    string    `json:"task_id"`
    Path      string    `json:"path"`
    CreatedAt time.Time `json:"created_at"`
    TTL       int64     `json:"ttl"`       // seconds
    SizeBytes int64     `json:"size_bytes"`
}

type Manager struct {
    basePath string
    redis    *redis.Client
    ttl      time.Duration
}

func (m *Manager) Create(ctx context.Context, taskID string) (*Workspace, error) {
    path := filepath.Join(m.basePath, taskID)
    if err := os.MkdirAll(path, 0755); err != nil {
        return nil, err
    }

    ws := &Workspace{
        TaskID:    taskID,
        Path:      path,
        CreatedAt: time.Now(),
        TTL:       int64(m.ttl.Seconds()),
    }

    m.redis.HSet(ctx, "workspace:"+taskID, map[string]interface{}{
        "path":       ws.Path,
        "created_at": ws.CreatedAt.Format(time.RFC3339),
        "ttl":        ws.TTL,
    })

    return ws, nil
}

func (m *Manager) Delete(ctx context.Context, taskID string) error {
    path := filepath.Join(m.basePath, taskID)

    // SECURITY: validate path is inside workspace_base before RemoveAll
    absPath, _ := filepath.Abs(path)
    absBase, _ := filepath.Abs(m.basePath)
    if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
        return fmt.Errorf("path traversal attempt: %s is outside workspace base %s", absPath, absBase)
    }

    os.RemoveAll(absPath)
    m.redis.Del(ctx, "workspace:"+taskID)
    return nil
}
```

### Dependencies

- Task 0.6 (Redis client)
- Task 0.2 (config for workspace base path and TTL)

---

## Task 5.2: TTL-Based Cleanup

**Priority:** P0
**Files:** `internal/workspace/cleanup.go`

### Description

Background goroutine that periodically scans for expired workspaces and removes them.

### Acceptance Criteria

- [ ] Runs every 10 minutes (configurable interval)
- [ ] Scans all `workspace:*` keys in Redis
- [ ] Checks `created_at + ttl` against current time
- [ ] Removes expired workspace directory (recursive delete)
- [ ] Removes expired Redis entries
- [ ] Skips workspaces for tasks currently in RUNNING status
- [ ] Logs cleanup actions (workspace ID, size reclaimed)
- [ ] Graceful shutdown: stops cleanup goroutine cleanly

### Implementation Notes

```go
func (c *Cleaner) Start(ctx context.Context) {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.cleanup(ctx)
        }
    }
}

func (c *Cleaner) cleanup(ctx context.Context) {
    // Scan workspace:* keys
    var cursor uint64
    for {
        keys, nextCursor, err := c.redis.Scan(ctx, cursor, "workspace:*", 100).Result()
        // For each key, check TTL expiry
        for _, key := range keys {
            ws := c.loadWorkspace(ctx, key)
            if ws.isExpired() && !c.isTaskRunning(ctx, ws.TaskID) {
                c.manager.Delete(ctx, ws.TaskID)
                slog.Info("workspace cleaned up", "task_id", ws.TaskID, "age", time.Since(ws.CreatedAt))
            }
        }
        cursor = nextCursor
        if cursor == 0 { break }
    }
}
```

### Dependencies

- Task 5.1 (workspace manager)
- Task 1.1 (task state to check RUNNING status)

---

## Task 5.3: Disk Usage Monitoring

**Priority:** P1
**Files:** `internal/workspace/manager.go`

### Description

Track workspace disk usage and expose metrics. Alert when total usage exceeds threshold.

### Acceptance Criteria

- [ ] Calculate workspace size on task completion
- [ ] Store size in workspace metadata (`size_bytes`)
- [ ] Total disk usage tracked (sum of all workspaces)
- [ ] Warning log when total exceeds configurable threshold
- [ ] **Config keys** (add to Task 0.2 config struct):
  - `tasks.disk_warning_threshold_gb`: warning level (default: 10)
  - `tasks.disk_critical_threshold_gb`: emergency cleanup trigger (default: 20)
- [ ] Expose via health endpoint: `"workspace_disk_usage_mb": 1234`
- [ ] Emergency cleanup: if above critical threshold, delete oldest expired first

### Implementation Notes

```go
func dirSize(path string) (int64, error) {
    var size int64
    filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if !d.IsDir() {
            info, _ := d.Info()
            size += info.Size()
        }
        return nil
    })
    return size, nil
}
```

### Dependencies

- Task 5.1 (workspace manager)
- Task 0.8 (health endpoint to expose metrics)

---

## Task 5.4: Workspace API

**Priority:** P2
**Files:** `internal/server/handlers/workspace.go`

### Description

Optional HTTP API for manual workspace management.

### Acceptance Criteria

- [ ] `GET /api/v1/workspaces` — list all workspaces (ID, size, age, task status)
- [ ] `DELETE /api/v1/workspaces/{taskID}` — manually delete a workspace
- [ ] Cannot delete workspace for a currently RUNNING task (409)
- [ ] Returns total disk usage in list response

### Implementation Notes

```go
// Response
type WorkspaceListResponse struct {
    Workspaces     []WorkspaceInfo `json:"workspaces"`
    TotalSizeMB    float64         `json:"total_size_mb"`
    TotalCount     int             `json:"total_count"`
}

type WorkspaceInfo struct {
    TaskID    string    `json:"task_id"`
    SizeMB    float64   `json:"size_mb"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    TaskStatus string   `json:"task_status"`
}
```

### Dependencies

- Task 5.1 (workspace manager)
- Task 5.3 (disk usage tracking)
