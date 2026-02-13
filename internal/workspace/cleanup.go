package workspace

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/freema/codeforge/internal/task"
)

// CleanerConfig holds cleanup configuration.
type CleanerConfig struct {
	Interval             time.Duration
	DiskWarningThreshold int64 // bytes
	DiskCriticalThreshold int64 // bytes
}

// Cleaner periodically removes expired workspaces.
type Cleaner struct {
	manager     *Manager
	taskService *task.Service
	cfg         CleanerConfig
}

// NewCleaner creates a new workspace cleaner.
func NewCleaner(manager *Manager, taskService *task.Service, cfg CleanerConfig) *Cleaner {
	return &Cleaner{
		manager:     manager,
		taskService: taskService,
		cfg:         cfg,
	}
}

// Start runs the cleanup loop until the context is cancelled.
func (c *Cleaner) Start(ctx context.Context) {
	interval := c.cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	slog.Info("workspace cleaner started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("workspace cleaner stopped")
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Cleaner) cleanup(ctx context.Context) {
	workspaces, err := c.manager.List(ctx)
	if err != nil {
		slog.Error("workspace cleanup scan failed", "error", err)
		return
	}

	var cleaned int
	var reclaimedBytes int64

	for _, ws := range workspaces {
		if !ws.IsExpired() {
			continue
		}

		// Skip workspaces for currently running tasks
		if c.isTaskRunning(ctx, ws.TaskID) {
			continue
		}

		slog.Info("cleaning up expired workspace",
			"task_id", ws.TaskID,
			"age", time.Since(ws.CreatedAt).Round(time.Second),
			"size_bytes", ws.SizeBytes,
		)

		if err := c.manager.Delete(ctx, ws.TaskID); err != nil {
			slog.Error("failed to delete workspace", "task_id", ws.TaskID, "error", err)
			continue
		}

		cleaned++
		reclaimedBytes += ws.SizeBytes
	}

	if cleaned > 0 {
		slog.Info("workspace cleanup complete",
			"cleaned", cleaned,
			"reclaimed_mb", float64(reclaimedBytes)/(1024*1024),
		)
	}

	// Check disk thresholds
	c.checkDiskUsage(ctx)
}

func (c *Cleaner) checkDiskUsage(ctx context.Context) {
	totalBytes := c.manager.TotalSizeBytes(ctx)

	if c.cfg.DiskCriticalThreshold > 0 && totalBytes > c.cfg.DiskCriticalThreshold {
		slog.Error("workspace disk usage CRITICAL — triggering emergency cleanup",
			"total_mb", float64(totalBytes)/(1024*1024),
			"threshold_mb", float64(c.cfg.DiskCriticalThreshold)/(1024*1024),
		)
		c.emergencyCleanup(ctx)
		return
	}

	if c.cfg.DiskWarningThreshold > 0 && totalBytes > c.cfg.DiskWarningThreshold {
		slog.Warn("workspace disk usage above warning threshold",
			"total_mb", float64(totalBytes)/(1024*1024),
			"threshold_mb", float64(c.cfg.DiskWarningThreshold)/(1024*1024),
		)
	}
}

// emergencyCleanup deletes oldest expired workspaces first until below critical threshold.
func (c *Cleaner) emergencyCleanup(ctx context.Context) {
	workspaces, err := c.manager.List(ctx)
	if err != nil {
		return
	}

	// Sort by creation time (oldest first)
	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].CreatedAt.Before(workspaces[j].CreatedAt)
	})

	for _, ws := range workspaces {
		if c.isTaskRunning(ctx, ws.TaskID) {
			continue
		}

		slog.Warn("emergency cleanup: deleting workspace", "task_id", ws.TaskID)
		if err := c.manager.Delete(ctx, ws.TaskID); err != nil {
			continue
		}

		// Re-check if we're below threshold
		totalBytes := c.manager.TotalSizeBytes(ctx)
		if totalBytes < c.cfg.DiskCriticalThreshold {
			slog.Info("emergency cleanup: below critical threshold",
				"total_mb", float64(totalBytes)/(1024*1024),
			)
			return
		}
	}
}

func (c *Cleaner) isTaskRunning(ctx context.Context, taskID string) bool {
	t, err := c.taskService.Get(ctx, taskID)
	if err != nil {
		return false // task not found, safe to delete
	}
	return t.Status == task.StatusRunning || t.Status == task.StatusCloning || t.Status == task.StatusCreatingPR
}
