package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/task"
)

// TaskCreator creates and retrieves CodeForge tasks.
type TaskCreator interface {
	Create(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error)
	Get(ctx context.Context, taskID string) (*task.Task, error)
}

// TaskExecutor executes task steps — creates a CodeForge task and waits for completion.
type TaskExecutor struct {
	tasks TaskCreator
	redis *redisclient.Client
}

// NewTaskExecutor creates a new task step executor.
func NewTaskExecutor(tasks TaskCreator, redis *redisclient.Client) *TaskExecutor {
	return &TaskExecutor{
		tasks: tasks,
		redis: redis,
	}
}

// Execute creates a CodeForge task and waits for it to complete via Redis done channel.
// Returns the task ID and result summary as outputs.
func (e *TaskExecutor) Execute(ctx context.Context, stepDef StepDefinition, tctx TemplateContext) (map[string]string, error) {
	var cfg TaskStepConfig
	if err := json.Unmarshal(stepDef.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing task config: %w", err)
	}

	// Render templates
	repoURL, err := Render(cfg.RepoURL, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering repo_url: %w", err)
	}
	prompt, err := Render(cfg.Prompt, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering prompt: %w", err)
	}
	taskType, _ := Render(cfg.TaskType, tctx)
	providerKey, _ := Render(cfg.ProviderKey, tctx)
	accessToken, _ := Render(cfg.AccessToken, tctx)
	cli, _ := Render(cfg.CLI, tctx)
	aiModel, _ := Render(cfg.AIModel, tctx)
	sourceBranch, _ := Render(cfg.SourceBranch, tctx)

	// Resolve workspace task ID from referenced step
	var workspaceTaskID string
	if cfg.WorkspaceTaskRef != "" {
		if refStep, ok := tctx.Steps[cfg.WorkspaceTaskRef]; ok {
			workspaceTaskID = refStep["task_id"]
		}
	}

	// Build TaskConfig if any overrides are set
	var taskConfig *task.TaskConfig
	if cli != "" || aiModel != "" || workspaceTaskID != "" || sourceBranch != "" {
		taskConfig = &task.TaskConfig{
			CLI:             cli,
			AIModel:         aiModel,
			WorkspaceTaskID: workspaceTaskID,
			SourceBranch:    sourceBranch,
		}
	}

	// Create the task
	req := task.CreateTaskRequest{
		RepoURL:     repoURL,
		Prompt:      prompt,
		TaskType:    taskType,
		ProviderKey: providerKey,
		AccessToken: accessToken,
		Config:      taskConfig,
	}
	t, err := e.tasks.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}

	slog.Info("workflow task created", "task_id", t.ID, "step", stepDef.Name)

	// Subscribe to done channel and wait
	doneKey := e.redis.Key("task", t.ID, "done")
	pubsub := e.redis.Unwrap().Subscribe(ctx, doneKey)
	defer pubsub.Close()

	msgCh := pubsub.Channel()

	// Poll with timeout from context
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("task %s: context cancelled while waiting: %w", t.ID, ctx.Err())

		case msg, ok := <-msgCh:
			if !ok {
				return nil, fmt.Errorf("task %s: subscription closed", t.ID)
			}

			var done struct {
				TaskID string `json:"task_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &done); err != nil {
				slog.Warn("failed to parse done message", "error", err)
				continue
			}

			if done.Status == string(task.StatusFailed) {
				// Fetch full task to get error
				ft, _ := e.tasks.Get(ctx, t.ID)
				errMsg := "task failed"
				if ft != nil && ft.Error != "" {
					errMsg = ft.Error
				}
				return nil, fmt.Errorf("task %s failed: %s", t.ID, errMsg)
			}

			// Task completed — fetch result
			ft, err := e.tasks.Get(ctx, t.ID)
			if err != nil {
				return nil, fmt.Errorf("getting completed task: %w", err)
			}

			outputs := map[string]string{
				"task_id": ft.ID,
				"status":  string(ft.Status),
				"result":  truncateString(ft.Result, 2000),
			}
			return outputs, nil

		case <-time.After(5 * time.Second):
			// Periodic check in case we missed the pub/sub message
			ft, err := e.tasks.Get(ctx, t.ID)
			if err != nil {
				continue
			}
			if ft.Status == task.StatusCompleted || ft.Status == task.StatusFailed || ft.Status == task.StatusPRCreated {
				if ft.Status == task.StatusFailed {
					return nil, fmt.Errorf("task %s failed: %s", ft.ID, ft.Error)
				}
				outputs := map[string]string{
					"task_id": ft.ID,
					"status":  string(ft.Status),
					"result":  truncateString(ft.Result, 2000),
				}
				return outputs, nil
			}
		}
	}
}

// WaitForTask subscribes to the done channel and waits for the task to complete.
// This is used when a pubsub is not available (e.g., in tests).
func WaitForTask(ctx context.Context, rdb *redis.Client, doneKey string) (string, error) {
	pubsub := rdb.Subscribe(ctx, doneKey)
	defer pubsub.Close()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case msg := <-pubsub.Channel():
		return msg.Payload, nil
	}
}
