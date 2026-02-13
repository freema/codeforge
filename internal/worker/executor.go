package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/freema/codeforge/internal/cli"
	gitpkg "github.com/freema/codeforge/internal/git"
	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/webhook"
)

// ExecutorConfig holds executor configuration.
type ExecutorConfig struct {
	WorkspaceBase  string
	DefaultTimeout int
	MaxTimeout     int
	DefaultModel   string
}

// Executor orchestrates the full task lifecycle: clone → run CLI → diff → report.
type Executor struct {
	taskService *task.Service
	runner      cli.Runner
	streamer    *Streamer
	webhook     *webhook.Sender
	cfg         ExecutorConfig
}

// NewExecutor creates a new task executor.
func NewExecutor(
	taskService *task.Service,
	runner cli.Runner,
	streamer *Streamer,
	webhook *webhook.Sender,
	cfg ExecutorConfig,
) *Executor {
	return &Executor{
		taskService: taskService,
		runner:      runner,
		streamer:    streamer,
		webhook:     webhook,
		cfg:         cfg,
	}
}

// Execute runs the full task pipeline.
func (e *Executor) Execute(ctx context.Context, t *task.Task) {
	log := slog.With("task_id", t.ID, "iteration", t.Iteration)

	// Determine timeout
	timeout := e.cfg.DefaultTimeout
	if t.Config != nil && t.Config.TimeoutSeconds > 0 {
		timeout = t.Config.TimeoutSeconds
	}
	if timeout > e.cfg.MaxTimeout {
		timeout = e.cfg.MaxTimeout
	}

	taskCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	workDir := filepath.Join(e.cfg.WorkspaceBase, t.ID)

	// Skip clone for iterations > 1 (workspace reuse)
	if t.Iteration <= 1 {
		if err := e.cloneStep(taskCtx, t, workDir, log); err != nil {
			e.failTask(ctx, t, fmt.Sprintf("clone failed: %v", err), log)
			return
		}
	} else {
		log.Info("reusing existing workspace", "work_dir", workDir)
	}

	// Run CLI
	result, err := e.runStep(taskCtx, t, workDir, log)
	if err != nil {
		if taskCtx.Err() == context.DeadlineExceeded {
			e.streamer.EmitSystem(ctx, t.ID, "task_timeout", map[string]interface{}{
				"timeout_seconds": timeout,
			})
			e.failTask(ctx, t, fmt.Sprintf("task timed out after %ds", timeout), log)
			return
		}
		e.failTask(ctx, t, fmt.Sprintf("CLI execution failed: %v", err), log)
		return
	}

	// Calculate changes
	changes, err := gitpkg.CalculateChanges(ctx, workDir)
	if err != nil {
		log.Warn("failed to calculate changes", "error", err)
	}

	// Build usage info
	usage := &task.UsageInfo{
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		DurationSeconds: int(result.Duration.Seconds()),
	}

	// Store result
	if err := e.taskService.SetResult(ctx, t.ID, result.Output, changes, usage); err != nil {
		log.Error("failed to store result", "error", err)
	}

	// Transition to completed
	if err := e.taskService.UpdateStatus(ctx, t.ID, task.StatusCompleted); err != nil {
		log.Error("failed to update status to completed", "error", err)
		return
	}

	// Emit completion events
	e.streamer.EmitResult(ctx, t.ID, "task_completed", map[string]interface{}{
		"result":          truncate(result.Output, 2000),
		"changes_summary": changes,
		"usage":           usage,
	})
	e.streamer.EmitDone(ctx, t.ID, task.StatusCompleted, changes)

	// Send webhook
	if t.CallbackURL != "" && e.webhook != nil {
		e.sendWebhook(ctx, t, result.Output, changes, usage, log)
	}

	log.Info("task completed", "duration", result.Duration)
}

func (e *Executor) cloneStep(ctx context.Context, t *task.Task, workDir string, log *slog.Logger) error {
	if err := e.taskService.UpdateStatus(ctx, t.ID, task.StatusCloning); err != nil {
		return err
	}

	e.streamer.EmitGit(ctx, t.ID, "clone_started", map[string]string{
		"repo_url": gitpkg.SanitizeURL(t.RepoURL),
	})

	// Ensure workspace directory exists
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}

	branch := ""
	if t.Config != nil {
		branch = t.Config.TargetBranch
	}

	err := gitpkg.Clone(ctx, gitpkg.CloneOptions{
		RepoURL: t.RepoURL,
		DestDir: workDir,
		Token:   t.AccessToken,
		Branch:  branch,
		Shallow: true,
	})
	if err != nil {
		return err
	}

	e.streamer.EmitGit(ctx, t.ID, "clone_completed", map[string]string{
		"work_dir": workDir,
	})

	log.Info("repository cloned", "work_dir", workDir)
	return nil
}

func (e *Executor) runStep(ctx context.Context, t *task.Task, workDir string, log *slog.Logger) (*cli.RunResult, error) {
	if err := e.taskService.UpdateStatus(ctx, t.ID, task.StatusRunning); err != nil {
		return nil, err
	}

	e.streamer.EmitSystem(ctx, t.ID, "cli_started", map[string]string{
		"cli": "claude-code",
	})

	prompt := t.CurrentPrompt
	if prompt == "" {
		prompt = t.Prompt
	}

	model := e.cfg.DefaultModel
	apiKey := ""
	var maxTurns int
	var maxBudget float64

	if t.Config != nil {
		if t.Config.AIModel != "" {
			model = t.Config.AIModel
		}
		apiKey = t.Config.AIApiKey
		maxTurns = t.Config.MaxTurns
		maxBudget = t.Config.MaxBudgetUSD
	}

	result, err := e.runner.Run(ctx, cli.RunOptions{
		Prompt:       prompt,
		WorkDir:      workDir,
		Model:        model,
		APIKey:       apiKey,
		MaxTurns:     maxTurns,
		MaxBudgetUSD: maxBudget,
		OnEvent: func(event json.RawMessage) {
			e.streamer.EmitCLIOutput(ctx, t.ID, event)
		},
	})

	if err != nil {
		return result, err
	}

	log.Info("CLI execution completed", "exit_code", result.ExitCode, "duration", result.Duration)
	return result, nil
}

func (e *Executor) failTask(ctx context.Context, t *task.Task, errMsg string, log *slog.Logger) {
	log.Error("task failed", "error", errMsg)

	e.taskService.SetError(ctx, t.ID, errMsg)
	e.taskService.UpdateStatus(ctx, t.ID, task.StatusFailed)

	e.streamer.EmitSystem(ctx, t.ID, "task_failed", map[string]string{
		"error": errMsg,
	})
	e.streamer.EmitDone(ctx, t.ID, task.StatusFailed, nil)

	// Send failure webhook
	if t.CallbackURL != "" && e.webhook != nil {
		e.webhook.Send(ctx, t.CallbackURL, webhook.Payload{
			TaskID:     t.ID,
			Status:     string(task.StatusFailed),
			Error:      errMsg,
			FinishedAt: time.Now().UTC(),
		})
	}
}

func (e *Executor) sendWebhook(ctx context.Context, t *task.Task, result string, changes *gitpkg.ChangesSummary, usage *task.UsageInfo, log *slog.Logger) {
	if err := e.webhook.Send(ctx, t.CallbackURL, webhook.Payload{
		TaskID:         t.ID,
		Status:         string(task.StatusCompleted),
		Result:         result,
		ChangesSummary: changes,
		Usage:          usage,
		FinishedAt:     time.Now().UTC(),
	}); err != nil {
		log.Error("webhook delivery failed", "error", err)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
