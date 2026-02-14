package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/freema/codeforge/internal/cli"
	gitpkg "github.com/freema/codeforge/internal/git"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/mcp"
	"github.com/freema/codeforge/internal/metrics"
	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/tracing"
	"github.com/freema/codeforge/internal/webhook"
	"github.com/freema/codeforge/internal/workspace"
)

const defaultMaxContextChars = 50000

// ExecutorConfig holds executor configuration.
type ExecutorConfig struct {
	WorkspaceBase  string
	DefaultTimeout int
	MaxTimeout     int
	DefaultModel   string
}

// Executor orchestrates the full task lifecycle: clone → run CLI → diff → report.
type Executor struct {
	taskService  *task.Service
	cliRegistry  *cli.Registry
	streamer     *Streamer
	webhook      *webhook.Sender
	keyResolver  *keys.Resolver
	mcpInstaller *mcp.Installer
	workspaceMgr *workspace.Manager
	cfg          ExecutorConfig
}

// NewExecutor creates a new task executor.
func NewExecutor(
	taskService *task.Service,
	cliRegistry *cli.Registry,
	streamer *Streamer,
	webhook *webhook.Sender,
	keyResolver *keys.Resolver,
	mcpInstaller *mcp.Installer,
	workspaceMgr *workspace.Manager,
	cfg ExecutorConfig,
) *Executor {
	return &Executor{
		taskService:  taskService,
		cliRegistry:  cliRegistry,
		streamer:     streamer,
		webhook:      webhook,
		keyResolver:  keyResolver,
		mcpInstaller: mcpInstaller,
		workspaceMgr: workspaceMgr,
		cfg:          cfg,
	}
}

// Execute runs the full task pipeline.
func (e *Executor) Execute(ctx context.Context, t *task.Task) {
	ctx, span := tracing.Tracer().Start(ctx, "task.execute",
		tracing.WithTaskAttributes(t.ID, t.Iteration),
	)
	defer span.End()

	// Store trace ID on task
	if traceID := tracing.TraceIDFromContext(ctx); traceID != "" {
		t.TraceID = traceID
	}

	log := slog.With("task_id", t.ID, "iteration", t.Iteration, "trace_id", t.TraceID)
	startTime := time.Now().UTC()

	metrics.TasksInProgress.Inc()
	defer func() {
		metrics.TasksInProgress.Dec()
		duration := time.Since(startTime).Seconds()
		metrics.TaskDuration.WithLabelValues(string(t.Status)).Observe(duration)
	}()

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

	// Resolve access token (task → registry → env)
	if e.keyResolver != nil && t.AccessToken == "" {
		token, err := e.keyResolver.ResolveToken(taskCtx, t.RepoURL, t.AccessToken, t.ProviderKey)
		if err != nil {
			log.Warn("token resolution failed", "error", err)
		} else {
			t.AccessToken = token
		}
	}

	// Clone or reuse workspace
	if t.Iteration <= 1 {
		if err := e.cloneStep(taskCtx, t, workDir, log); err != nil {
			e.failTask(ctx, t, fmt.Sprintf("clone failed: %v", err), startTime, log)
			return
		}
	} else {
		// Reuse workspace — check it still exists
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			log.Warn("workspace missing for iteration, re-cloning", "work_dir", workDir)
			if err := e.cloneStep(taskCtx, t, workDir, log); err != nil {
				e.failTask(ctx, t, fmt.Sprintf("re-clone failed: %v", err), startTime, log)
				return
			}
		} else {
			log.Info("reusing existing workspace", "work_dir", workDir)
			// Pull latest if a branch was pushed (PR flow)
			if t.Branch != "" {
				e.pullBranch(taskCtx, t, workDir, log)
			}
		}
	}

	// Setup MCP servers (generate .mcp.json)
	if e.mcpInstaller != nil {
		var taskMCPServers []mcp.Server
		if t.Config != nil {
			for _, s := range t.Config.MCPServers {
				taskMCPServers = append(taskMCPServers, mcp.Server{
					Name:    s.Name,
					Package: s.Command, // task model uses "command" as package
					Args:    s.Args,
					Env:     s.Env,
				})
			}
		}
		if err := e.mcpInstaller.Setup(taskCtx, workDir, t.RepoURL, taskMCPServers); err != nil {
			log.Warn("MCP setup failed (continuing without MCP)", "error", err)
		}
	}

	// Run CLI
	result, err := e.runStep(taskCtx, t, workDir, log)
	if err != nil {
		if taskCtx.Err() == context.DeadlineExceeded {
			_ = e.streamer.EmitSystem(ctx, t.ID, "task_timeout", map[string]interface{}{
				"timeout_seconds": timeout,
			})
			e.failTask(ctx, t, fmt.Sprintf("task timed out after %ds", timeout), startTime, log)
			return
		}
		if ctx.Err() == context.Canceled {
			_ = e.streamer.EmitSystem(ctx, t.ID, "task_cancelled", nil)
			e.failTask(context.Background(), t, "cancelled by user", startTime, log)
			return
		}
		e.failTask(ctx, t, fmt.Sprintf("CLI execution failed: %v", err), startTime, log)
		return
	}

	// Calculate changes
	changes, err := gitpkg.CalculateChanges(ctx, workDir)
	if err != nil {
		log.Warn("failed to calculate changes", "error", err)
	}

	// Update workspace size
	if e.workspaceMgr != nil {
		if size, err := e.workspaceMgr.UpdateSize(ctx, t.ID); err == nil {
			log.Info("workspace size updated", "size_bytes", size)
		}
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
	metrics.TasksTotal.WithLabelValues(string(task.StatusCompleted)).Inc()

	// Save iteration record
	now := time.Now().UTC()
	prompt := t.CurrentPrompt
	if prompt == "" {
		prompt = t.Prompt
	}
	_ = e.taskService.SaveIteration(ctx, t.ID, task.Iteration{
		Number:    t.Iteration,
		Prompt:    prompt,
		Result:    truncate(result.Output, 2000),
		Status:    task.StatusCompleted,
		Changes:   changes,
		Usage:     usage,
		StartedAt: startTime,
		EndedAt:   &now,
	})

	// Emit completion events
	_ = e.streamer.EmitResult(ctx, t.ID, "task_completed", map[string]interface{}{
		"result":          truncate(result.Output, 2000),
		"changes_summary": changes,
		"usage":           usage,
		"iteration":       t.Iteration,
	})
	_ = e.streamer.EmitDone(ctx, t.ID, task.StatusCompleted, changes)

	// Send webhook
	if t.CallbackURL != "" && e.webhook != nil {
		e.sendWebhook(ctx, t, result.Output, changes, usage, log)
	}

	log.Info("task completed", "duration", result.Duration)
}

func (e *Executor) cloneStep(ctx context.Context, t *task.Task, workDir string, log *slog.Logger) error {
	ctx, span := tracing.Tracer().Start(ctx, "task.clone")
	defer span.End()

	if err := e.taskService.UpdateStatus(ctx, t.ID, task.StatusCloning); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	_ = e.streamer.EmitGit(ctx, t.ID, "clone_started", map[string]string{
		"repo_url": gitpkg.SanitizeURL(t.RepoURL),
	})

	// Create workspace via manager (or fallback to raw mkdir)
	if e.workspaceMgr != nil {
		if _, err := e.workspaceMgr.Create(ctx, t.ID); err != nil {
			return fmt.Errorf("creating workspace: %w", err)
		}
	} else {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return fmt.Errorf("creating workspace: %w", err)
		}
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
		span.SetStatus(codes.Error, "clone failed")
		return err
	}

	_ = e.streamer.EmitGit(ctx, t.ID, "clone_completed", map[string]string{
		"work_dir": workDir,
	})

	// If running as root, chown workspace to "codeforge" user so the CLI
	// (which drops privileges) can write to it.
	if os.Getuid() == 0 {
		if u, uErr := user.Lookup("codeforge"); uErr == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			_ = chownRecursive(workDir, uid, gid)
		}
	}

	log.Info("repository cloned", "work_dir", workDir)
	return nil
}

func (e *Executor) pullBranch(ctx context.Context, t *task.Task, workDir string, log *slog.Logger) {
	log.Info("pulling latest changes", "branch", t.Branch)

	askPassEnv, cleanup, err := gitpkg.AskPassEnv(t.AccessToken)
	if err != nil {
		log.Warn("failed to create askpass for pull", "error", err)
		return
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "git", "pull", "origin", t.Branch)
	cmd.Dir = workDir
	if len(askPassEnv) > 0 {
		cmd.Env = append(os.Environ(), askPassEnv...)
	}

	if err := cmd.Run(); err != nil {
		log.Warn("git pull failed (continuing with existing workspace)", "error", err)
	}
}

func (e *Executor) runStep(ctx context.Context, t *task.Task, workDir string, log *slog.Logger) (*cli.RunResult, error) {
	ctx, span := tracing.Tracer().Start(ctx, "task.run")
	defer span.End()

	// Transition to RUNNING — handle both fresh tasks and follow-up iterations
	if t.Status != task.StatusRunning {
		if err := e.taskService.UpdateStatus(ctx, t.ID, task.StatusRunning); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	}

	// Resolve CLI runner from registry
	cliName := ""
	if t.Config != nil {
		cliName = t.Config.CLI
	}
	runner, err := e.cliRegistry.Get(cliName)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("resolving CLI runner: %w", err)
	}
	span.SetAttributes(attribute.String("cli.name", cliName))

	_ = e.streamer.EmitSystem(ctx, t.ID, "cli_started", map[string]string{
		"cli":       cliName,
		"iteration": fmt.Sprintf("%d", t.Iteration),
	})

	// Build prompt with conversation context for iterations > 1
	prompt := e.buildPrompt(ctx, t)

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

	result, err := runner.Run(ctx, cli.RunOptions{
		Prompt:       prompt,
		WorkDir:      workDir,
		Model:        model,
		APIKey:       apiKey,
		MaxTurns:     maxTurns,
		MaxBudgetUSD: maxBudget,
		OnEvent: func(event json.RawMessage) {
			_ = e.streamer.EmitCLIOutput(ctx, t.ID, event)
		},
	})

	if err != nil {
		return result, err
	}

	log.Info("CLI execution completed", "exit_code", result.ExitCode, "duration", result.Duration)
	return result, nil
}

// buildPrompt constructs the prompt with conversation context for multi-turn iterations.
func (e *Executor) buildPrompt(ctx context.Context, t *task.Task) string {
	currentPrompt := t.CurrentPrompt
	if currentPrompt == "" {
		currentPrompt = t.Prompt
	}

	// First iteration — no context needed
	if t.Iteration <= 1 {
		return currentPrompt
	}

	// Load previous iterations for context
	iterations, err := e.taskService.GetIterations(ctx, t.ID)
	if err != nil || len(iterations) == 0 {
		return currentPrompt
	}

	var ctx2 strings.Builder
	ctx2.WriteString("## Previous iterations on this codebase:\n\n")

	totalChars := 0
	// Build from oldest to newest, but we may need to truncate oldest first
	for _, iter := range iterations {
		entry := fmt.Sprintf("### Iteration %d\n**Prompt:** %s\n**Result summary:** %s\n**Status:** %s\n\n",
			iter.Number, iter.Prompt, iter.Result, iter.Status)

		if totalChars+len(entry) > defaultMaxContextChars {
			// Truncate — drop this and older entries
			ctx2.WriteString("(earlier iterations truncated for context limits)\n\n")
			break
		}

		ctx2.WriteString(entry)
		totalChars += len(entry)
	}

	ctx2.WriteString("## Current instruction:\n\n")
	ctx2.WriteString(currentPrompt)

	return ctx2.String()
}

func (e *Executor) failTask(ctx context.Context, t *task.Task, errMsg string, startTime time.Time, log *slog.Logger) {
	log.Error("task failed", "error", errMsg)

	_ = e.taskService.SetError(ctx, t.ID, errMsg)
	_ = e.taskService.UpdateStatus(ctx, t.ID, task.StatusFailed)
	metrics.TasksTotal.WithLabelValues(string(task.StatusFailed)).Inc()

	// Save failed iteration record
	now := time.Now().UTC()
	prompt := t.CurrentPrompt
	if prompt == "" {
		prompt = t.Prompt
	}
	_ = e.taskService.SaveIteration(ctx, t.ID, task.Iteration{
		Number:    t.Iteration,
		Prompt:    prompt,
		Error:     errMsg,
		Status:    task.StatusFailed,
		StartedAt: startTime,
		EndedAt:   &now,
	})

	_ = e.streamer.EmitSystem(ctx, t.ID, "task_failed", map[string]string{
		"error": errMsg,
	})
	_ = e.streamer.EmitDone(ctx, t.ID, task.StatusFailed, nil)

	if t.CallbackURL != "" && e.webhook != nil {
		_ = e.webhook.Send(ctx, t.CallbackURL, webhook.Payload{
			TaskID:     t.ID,
			Status:     string(task.StatusFailed),
			Error:      errMsg,
			TraceID:    t.TraceID,
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
		TraceID:        t.TraceID,
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

// chownRecursive changes ownership of a directory tree.
func chownRecursive(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}
