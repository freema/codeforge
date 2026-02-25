package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/freema/codeforge/internal/task"
)

// PRCreator creates pull requests for completed tasks.
type PRCreator interface {
	CreatePR(ctx context.Context, taskID string, req task.CreatePRRequest) (*task.CreatePRResponse, error)
}

// ActionExecutor executes action steps — built-in operations like create_pr.
type ActionExecutor struct {
	prCreator PRCreator
}

// NewActionExecutor creates a new action step executor.
func NewActionExecutor(prCreator PRCreator) *ActionExecutor {
	return &ActionExecutor{prCreator: prCreator}
}

// Execute runs an action step based on its kind.
func (e *ActionExecutor) Execute(ctx context.Context, stepDef StepDefinition, tctx TemplateContext) (map[string]string, error) {
	var cfg ActionConfig
	if err := json.Unmarshal(stepDef.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing action config: %w", err)
	}

	switch cfg.Kind {
	case ActionCreatePR:
		return e.executeCreatePR(ctx, cfg, tctx)
	case ActionNotify:
		return e.executeNotify(ctx, cfg, tctx)
	default:
		return nil, fmt.Errorf("unknown action kind: %s", cfg.Kind)
	}
}

func (e *ActionExecutor) executeCreatePR(ctx context.Context, cfg ActionConfig, tctx TemplateContext) (map[string]string, error) {
	// Resolve task ID from the referenced task step
	refStep, ok := tctx.Steps[cfg.TaskStepRef]
	if !ok {
		return nil, fmt.Errorf("task step ref '%s' not found in context", cfg.TaskStepRef)
	}
	taskID := refStep["task_id"]
	if taskID == "" {
		return nil, fmt.Errorf("task step '%s' has no task_id", cfg.TaskStepRef)
	}

	title, err := Render(cfg.Title, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering PR title: %w", err)
	}
	description, err := Render(cfg.Description, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering PR description: %w", err)
	}

	result, err := e.prCreator.CreatePR(ctx, taskID, task.CreatePRRequest{
		Title:       title,
		Description: description,
	})
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}

	slog.Info("workflow created PR", "task_id", taskID, "pr_url", result.PRURL)

	return map[string]string{
		"pr_url":    result.PRURL,
		"pr_number": fmt.Sprintf("%d", result.PRNumber),
		"branch":    result.Branch,
	}, nil
}

func (e *ActionExecutor) executeNotify(_ context.Context, _ ActionConfig, _ TemplateContext) (map[string]string, error) {
	slog.Info("notify action: placeholder (not implemented)")
	return map[string]string{"status": "skipped"}, nil
}
