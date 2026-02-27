package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/review"
	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/workspace"
)

// reviewTaskAdapter adapts task.Service + workspace.Manager to the review.TaskProvider interface.
type reviewTaskAdapter struct {
	service       *task.Service
	workspaceMgr  *workspace.Manager
	workspaceBase string
}

func (a *reviewTaskAdapter) StartReview(ctx context.Context, taskID string) error {
	return a.service.StartReview(ctx, taskID)
}

func (a *reviewTaskAdapter) CompleteReview(ctx context.Context, taskID string, result *review.ReviewResult) error {
	return a.service.CompleteReview(ctx, taskID, result)
}

func (a *reviewTaskAdapter) GetTask(ctx context.Context, taskID string) (review.TaskInfo, error) {
	t, err := a.service.Get(ctx, taskID)
	if err != nil {
		return review.TaskInfo{}, err
	}

	// Resolve workspace path
	workDir := a.resolveWorkDir(ctx, t)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return review.TaskInfo{}, apperror.Conflict("workspace not found for task %s — it may have been cleaned up", taskID)
	}

	info := review.TaskInfo{
		ID:     t.ID,
		Prompt: t.Prompt,
	}

	if t.Config != nil {
		info.AIApiKey = t.Config.AIApiKey
		info.CLI = t.Config.CLI
	}

	info.WorkDir = workDir

	return info, nil
}

func (a *reviewTaskAdapter) resolveWorkDir(ctx context.Context, t *task.Task) string {
	if a.workspaceMgr != nil {
		if ws := a.workspaceMgr.Get(ctx, t.ID); ws != nil && ws.Path != "" {
			return ws.Path
		}
		if t.Config != nil && t.Config.WorkspaceTaskID != "" {
			if ws := a.workspaceMgr.Get(ctx, t.Config.WorkspaceTaskID); ws != nil && ws.Path != "" {
				return ws.Path
			}
		}
	}
	return filepath.Join(a.workspaceBase, t.ID)
}
