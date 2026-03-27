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

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/metrics"
	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/review"
	"github.com/freema/codeforge/internal/session"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tool/mcp"
	"github.com/freema/codeforge/internal/tool/runner"
	"github.com/freema/codeforge/internal/tools"
	"github.com/freema/codeforge/internal/tracing"
	"github.com/freema/codeforge/internal/webhook"
	"github.com/freema/codeforge/internal/workspace"
)

const (
	defaultMaxContextChars = 50000
	defaultCLI             = "claude-code"
)

// ExecutorConfig holds executor configuration.
type ExecutorConfig struct {
	WorkspaceBase   string
	DefaultTimeout  int
	MaxTimeout      int
	DefaultModels   map[string]string // CLI name → default model (e.g. "claude-code" → "claude-sonnet-4-...")
	ProviderDomains map[string]string // custom domain → provider mappings
}

// Executor orchestrates the full session lifecycle: clone → run CLI → diff → report.
type Executor struct {
	sessionService  *session.Service
	cliRegistry  *runner.Registry
	streamer     *Streamer
	webhook      *webhook.Sender
	keyResolver  *keys.Resolver
	mcpInstaller *mcp.Installer
	toolResolver *tools.Resolver
	workspaceMgr *workspace.Manager
	cfg          ExecutorConfig
}

// NewExecutor creates a new session executor.
func NewExecutor(
	sessionService *session.Service,
	cliRegistry *runner.Registry,
	streamer *Streamer,
	webhook *webhook.Sender,
	keyResolver *keys.Resolver,
	mcpInstaller *mcp.Installer,
	toolResolver *tools.Resolver,
	workspaceMgr *workspace.Manager,
	cfg ExecutorConfig,
) *Executor {
	return &Executor{
		sessionService:  sessionService,
		cliRegistry:  cliRegistry,
		streamer:     streamer,
		webhook:      webhook,
		keyResolver:  keyResolver,
		mcpInstaller: mcpInstaller,
		toolResolver: toolResolver,
		workspaceMgr: workspaceMgr,
		cfg:          cfg,
	}
}

// emitOrLog emits a stream event, logging a warning on failure.
// Streaming is best-effort — failures are non-fatal.
func (e *Executor) emitOrLog(err error, log *slog.Logger, event, sessionID string) {
	if err != nil {
		log.Warn("stream emit failed", "event", event, "session_id", sessionID, "error", err)
	}
}

// Execute runs the full session pipeline.
func (e *Executor) Execute(ctx context.Context, t *session.Session) {
	ctx, span := tracing.Tracer().Start(ctx, "task.execute",
		tracing.WithSessionAttributes(t.ID, t.Iteration),
	)
	defer span.End()

	if traceID := tracing.TraceIDFromContext(ctx); traceID != "" {
		t.TraceID = traceID
	}

	// Dispatch review tasks to dedicated flow (enqueued via StartReviewAsync)
	if t.Status == session.StatusReviewing {
		e.executeReview(ctx, t)
		return
	}

	log := slog.With("session_id", t.ID, "iteration", t.Iteration, "trace_id", t.TraceID)
	startTime := time.Now().UTC()

	// Emit user instruction for follow-up iterations so the UI shows what the user asked
	if t.Iteration > 1 && t.CurrentPrompt != "" {
		e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "user_instruction", map[string]string{
			"prompt":    t.CurrentPrompt,
			"iteration": fmt.Sprintf("%d", t.Iteration),
		}), log, "user_instruction", t.ID)
	}

	metrics.TasksInProgress.Inc()
	defer func() {
		metrics.TasksInProgress.Dec()
		metrics.TaskDuration.WithLabelValues(string(t.Status)).Observe(time.Since(startTime).Seconds())
	}()

	timeout := e.resolveTimeout(t)
	sessionCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Phase 1: resolve token + prepare workspace
	e.resolveToken(sessionCtx, t, log)
	workDir, err := e.setupWorkspace(sessionCtx, ctx, t, startTime, log)
	if err != nil {
		return // failSession already called inside setupWorkspace
	}

	// Phase 2: resolve tools + MCP config
	mcpConfigPath, mcpErr := e.setupMCP(sessionCtx, t, workDir, log)
	if mcpErr != nil {
		e.failSession(ctx, t, fmt.Sprintf("tool/MCP setup failed: %v", mcpErr), startTime, log)
		return
	}

	// Phase 3: run CLI
	result, err := e.runStep(sessionCtx, t, workDir, mcpConfigPath, log)
	if err != nil {
		// Timeout: complete gracefully with partial result instead of failing
		if sessionCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, t, result, workDir, timeout, startTime, log)
			return
		}
		e.handleRunError(ctx, t, err, timeout, startTime, log)
		return
	}

	// Phase 4: finalize
	e.completeSession(ctx, t, result, workDir, startTime, log)
}

// resolveTimeout determines the effective session timeout in seconds.
func (e *Executor) resolveTimeout(t *session.Session) int {
	timeout := e.cfg.DefaultTimeout
	if t.Config != nil && t.Config.TimeoutSeconds > 0 {
		timeout = t.Config.TimeoutSeconds
	}
	if timeout > e.cfg.MaxTimeout {
		timeout = e.cfg.MaxTimeout
	}
	return timeout
}

// resolveToken resolves the access token from the key registry if not already set.
func (e *Executor) resolveToken(ctx context.Context, t *session.Session, log *slog.Logger) {
	if e.keyResolver == nil || t.AccessToken != "" {
		return
	}
	token, err := e.keyResolver.ResolveToken(ctx, t.RepoURL, t.AccessToken, t.ProviderKey)
	if err != nil {
		log.Warn("token resolution failed", "error", err)
		return
	}
	t.AccessToken = token
}

// setupWorkspace resolves or clones the workspace directory.
// Returns the workDir path or calls failSession and returns an error.
func (e *Executor) setupWorkspace(sessionCtx, parentCtx context.Context, t *session.Session, startTime time.Time, log *slog.Logger) (string, error) {
	workDir := filepath.Join(e.cfg.WorkspaceBase, t.ID)
	if e.workspaceMgr != nil {
		if ws := e.workspaceMgr.Get(parentCtx, t.ID); ws != nil && ws.Path != "" {
			workDir = ws.Path
		}
	}

	// Check workspace reuse from a referenced session (e.g. code review step)
	if t.Config != nil && t.Config.WorkspaceSessionID != "" && e.workspaceMgr != nil {
		if refWs := e.workspaceMgr.Get(parentCtx, t.Config.WorkspaceSessionID); refWs != nil {
			if _, statErr := os.Stat(refWs.Path); statErr == nil {
				log.Info("reusing workspace from referenced session",
					"ref_session_id", t.Config.WorkspaceSessionID, "work_dir", refWs.Path)
				return refWs.Path, nil
			}
		}
	}

	// First iteration: clone
	if t.Iteration <= 1 {
		if err := e.cloneStep(sessionCtx, t, workDir, log); err != nil {
			e.failSession(parentCtx, t, fmt.Sprintf("clone failed: %v", err), startTime, log)
			return "", err
		}
		// Re-resolve workDir — cloneStep may have created workspace at a slug-based path
		if e.workspaceMgr != nil {
			if ws := e.workspaceMgr.Get(parentCtx, t.ID); ws != nil && ws.Path != "" {
				workDir = ws.Path
			}
		}
		return workDir, nil
	}

	// Follow-up iteration: reuse or re-clone
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		log.Warn("workspace missing for iteration, re-cloning", "work_dir", workDir)
		if err := e.cloneStep(sessionCtx, t, workDir, log); err != nil {
			e.failSession(parentCtx, t, fmt.Sprintf("re-clone failed: %v", err), startTime, log)
			return "", err
		}
	} else {
		log.Info("reusing existing workspace", "work_dir", workDir)
		if t.Branch != "" {
			e.pullBranch(sessionCtx, t, workDir, log)
		}
	}

	return workDir, nil
}

// setupMCP resolves tool definitions and MCP server configs, writes .mcp.json.
// Returns an error if the session explicitly requires tools/MCP and setup fails (fail-closed).
func (e *Executor) setupMCP(ctx context.Context, t *session.Session, workDir string, log *slog.Logger) (string, error) {
	// Resolve tool definitions → MCP servers
	var toolMCPServers []mcp.Server
	if e.toolResolver != nil && t.Config != nil && len(t.Config.Tools) > 0 {
		instances, err := e.toolResolver.Resolve(ctx, t.RepoURL, t.Config.Tools)
		if err != nil {
			// Fail-closed: session explicitly requested tools but resolve failed
			return "", fmt.Errorf("tool resolution failed: %w", err)
		}
		toolMCPServers = tools.ToMCPServers(instances)
	}

	if e.mcpInstaller == nil {
		return "", nil
	}

	// Collect session-level MCP servers
	var taskMCPServers []mcp.Server
	if t.Config != nil {
		for _, s := range t.Config.MCPServers {
			taskMCPServers = append(taskMCPServers, mcp.Server{
				Name:      s.Name,
				Transport: s.Transport,
				Command:   s.Command,
				Package:   s.Package,
				Args:      s.Args,
				Env:       s.Env,
				URL:       s.URL,
				Headers:   s.Headers,
			})
		}
	}
	taskMCPServers = append(taskMCPServers, toolMCPServers...)

	if err := e.mcpInstaller.Setup(ctx, workDir, t.RepoURL, taskMCPServers); err != nil {
		if len(taskMCPServers) > 0 {
			// Fail-closed: MCP servers were configured but install failed
			return "", fmt.Errorf("MCP setup failed: %w", err)
		}
		log.Warn("MCP setup failed (no servers configured, continuing)", "error", err)
		return "", nil
	}

	cfgPath := filepath.Join(workDir, ".mcp.json")
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		log.Info("MCP config written", "path", cfgPath)
		return cfgPath, nil
	}
	return "", nil
}

// handleTimeout gracefully completes a timed-out session instead of failing it.
// The workspace is preserved so the user can create a PR or send a follow-up instruction.
func (e *Executor) handleTimeout(ctx context.Context, t *session.Session, result *runner.RunResult, workDir string, timeout int, startTime time.Time, log *slog.Logger) {
	finalCtx := context.WithoutCancel(ctx)
	log.Warn("session timed out, completing gracefully", "timeout_seconds", timeout)

	e.emitOrLog(e.streamer.EmitSystem(finalCtx, t.ID, "task_timeout", map[string]interface{}{
		"timeout_seconds": timeout,
		"graceful":        true,
	}), log, "task_timeout", t.ID)

	// Build a partial result from whatever CLI produced
	if result == nil {
		result = &runner.RunResult{
			Output:   fmt.Sprintf("[Session timed out after %ds. Work in progress was preserved. You can continue with a follow-up instruction or create a PR from current changes.]", timeout),
			Duration: time.Since(startTime),
		}
	} else if result.Output == "" {
		result.Output = fmt.Sprintf("[Session timed out after %ds with no output captured.]", timeout)
	} else {
		result.Output += fmt.Sprintf("\n\n[Session timed out after %ds. Partial output above.]", timeout)
	}

	// Complete normally — this allows the user to instruct or create PR
	e.completeSession(finalCtx, t, result, workDir, startTime, log)
}

// handleRunError classifies the CLI run error and calls failSession with the appropriate message.
func (e *Executor) handleRunError(ctx context.Context, t *session.Session, err error, timeout int, startTime time.Time, log *slog.Logger) {
	if ctx.Err() == context.Canceled {
		e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "task_canceled", nil), log, "task_canceled", t.ID)
		e.failSession(context.Background(), t, "canceled by user", startTime, log)
		return
	}
	e.failSession(ctx, t, fmt.Sprintf("CLI execution failed: %v", err), startTime, log)
}

// completeSession handles post-CLI success: changes, result storage, status transition,
// iteration record, events, pr_review handling, and webhook delivery.
func (e *Executor) completeSession(ctx context.Context, t *session.Session, result *runner.RunResult, workDir string, startTime time.Time, log *slog.Logger) {
	changes, err := gitpkg.CalculateChanges(ctx, workDir)
	if err != nil {
		log.Warn("failed to calculate changes", "error", err)
	}

	if e.workspaceMgr != nil {
		if size, err := e.workspaceMgr.UpdateSize(ctx, t.ID); err == nil {
			log.Info("workspace size updated", "size_bytes", size)
		}
	}

	usage := &session.UsageInfo{
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		DurationSeconds: int(result.Duration.Seconds()),
	}

	if err := e.sessionService.SetResult(ctx, t.ID, result.Output, changes, usage); err != nil {
		log.Error("failed to store result", "error", err)
	}

	if err := e.sessionService.UpdateStatus(ctx, t.ID, session.StatusCompleted); err != nil {
		log.Error("failed to update status to completed", "error", err)
		return
	}
	metrics.TasksTotal.WithLabelValues(string(session.StatusCompleted)).Inc()

	// Save iteration record
	now := time.Now().UTC()
	prompt := t.CurrentPrompt
	if prompt == "" {
		prompt = t.Prompt
	}
	if err := e.sessionService.SaveIteration(ctx, t.ID, session.Iteration{
		Number:    t.Iteration,
		Prompt:    prompt,
		Result:    truncate(result.Output, 2000),
		Status:    session.StatusCompleted,
		Changes:   changes,
		Usage:     usage,
		StartedAt: startTime,
		EndedAt:   &now,
	}); err != nil {
		log.Warn("failed to save iteration", "error", err)
	}

	e.emitOrLog(e.streamer.EmitResult(ctx, t.ID, "task_completed", map[string]interface{}{
		"result":          truncate(result.Output, 2000),
		"changes_summary": changes,
		"usage":           usage,
		"iteration":       t.Iteration,
	}), log, "task_completed", t.ID)

	// Review post-processing BEFORE done — client may close stream after done event
	if t.SessionType == "pr_review" || t.SessionType == "review" {
		e.handlePRReviewCompletion(ctx, t, result.Output, log)
	}

	// Auto-review after fix: if configured and this is a follow-up iteration,
	// automatically start a review and post results back to the MR.
	if t.Config != nil && t.Config.AutoReviewAfterFix && t.Iteration > 1 {
		log.Info("auto-review: starting review after fix iteration", "iteration", t.Iteration)
		e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "auto_review_starting", nil), log, "auto_review_starting", t.ID)
		cli := defaultCLI
		if t.Config.CLI != "" {
			cli = t.Config.CLI
		}
		if _, err := e.sessionService.StartReviewAsync(ctx, t.ID, cli, ""); err != nil {
			log.Error("auto-review: failed to start", "error", err)
		}
	}

	e.emitOrLog(e.streamer.EmitDone(ctx, t.ID, session.StatusCompleted, changes), log, "task_done", t.ID)

	if t.CallbackURL != "" && e.webhook != nil {
		e.sendWebhook(ctx, t, result.Output, changes, usage, log)
	}

	log.Info("session completed", "duration", result.Duration)
}

func (e *Executor) cloneStep(ctx context.Context, t *session.Session, workDir string, log *slog.Logger) error {
	ctx, span := tracing.Tracer().Start(ctx, "task.clone")
	defer span.End()

	if err := e.sessionService.UpdateStatus(ctx, t.ID, session.StatusCloning); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	e.emitOrLog(e.streamer.EmitGit(ctx, t.ID, "clone_started", map[string]string{
		"repo_url": gitpkg.SanitizeURL(t.RepoURL),
	}), log, "clone_started", t.ID)

	// Create workspace via manager (or fallback to raw mkdir)
	if e.workspaceMgr != nil {
		prompt := t.Prompt
		if t.CurrentPrompt != "" {
			prompt = t.CurrentPrompt
		}
		ws, err := e.workspaceMgr.Create(ctx, t.ID, prompt)
		if err != nil {
			return fmt.Errorf("creating workspace: %w", err)
		}
		workDir = ws.Path
	} else {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return fmt.Errorf("creating workspace: %w", err)
		}
	}

	// For pr_review tasks: clone the target branch (or default), then fetch the PR ref.
	// This handles fork PRs where the source branch doesn't exist in the origin repo.
	if t.SessionType == "pr_review" && t.Config != nil && t.Config.PRNumber > 0 {
		targetBranch := t.Config.TargetBranch
		if targetBranch == "" {
			targetBranch = "main"
		}

		err := gitpkg.Clone(ctx, gitpkg.CloneOptions{
			RepoURL: t.RepoURL,
			DestDir: workDir,
			Token:   t.AccessToken,
			Branch:  targetBranch,
			Shallow: false, // need full history for diff
		})
		if err != nil {
			span.SetStatus(codes.Error, "clone failed")
			return err
		}

		// Determine the correct PR ref based on provider (GitHub vs GitLab)
		repo, parseErr := gitpkg.ParseRepoURL(t.RepoURL, e.cfg.ProviderDomains)
		var prRef string
		if parseErr == nil && repo.Provider == gitpkg.ProviderGitLab {
			prRef = fmt.Sprintf("merge-requests/%d/head", t.Config.PRNumber)
		} else {
			prRef = fmt.Sprintf("pull/%d/head", t.Config.PRNumber)
		}
		prBranch := fmt.Sprintf("pr-%d", t.Config.PRNumber)
		if err := e.fetchAndCheckoutPR(ctx, t, workDir, prRef, prBranch, log); err != nil {
			span.SetStatus(codes.Error, "pr fetch failed")
			return err
		}

		// Store the resolved branch name for the prompt template
		t.Config.SourceBranch = prBranch
	} else {
		branch := ""
		if t.Config != nil {
			branch = t.Config.SourceBranch
			if branch == "" {
				branch = t.Config.TargetBranch // backward compat
			}
		}

		err := gitpkg.Clone(ctx, gitpkg.CloneOptions{
			RepoURL: t.RepoURL,
			DestDir: workDir,
			Token:   t.AccessToken,
			Branch:  branch,
			Shallow: false,
		})
		if err != nil {
			span.SetStatus(codes.Error, "clone failed")
			return err
		}
	}

	e.emitOrLog(e.streamer.EmitGit(ctx, t.ID, "clone_completed", map[string]string{
		"work_dir": workDir,
	}), log, "clone_completed", t.ID)

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

func (e *Executor) pullBranch(ctx context.Context, t *session.Session, workDir string, log *slog.Logger) {
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

func (e *Executor) runStep(ctx context.Context, t *session.Session, workDir string, mcpConfigPath string, log *slog.Logger) (*runner.RunResult, error) {
	ctx, span := tracing.Tracer().Start(ctx, "task.run")
	defer span.End()

	// Transition to RUNNING — handle both fresh tasks and follow-up iterations
	if t.Status != session.StatusRunning {
		if err := e.sessionService.UpdateStatus(ctx, t.ID, session.StatusRunning); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	}

	// Resolve CLI runner from registry
	cliName := ""
	if t.Config != nil {
		cliName = t.Config.CLI
	}
	cliRunner, err := e.cliRegistry.Get(cliName)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("resolving CLI runner: %w", err)
	}

	// Resolve the effective CLI name for model lookup (registry may have
	// resolved "" to the default CLI).
	resolvedCLI := cliName
	if resolvedCLI == "" {
		resolvedCLI = defaultCLI
	}
	span.SetAttributes(attribute.String("cli.name", resolvedCLI))

	// Select stream normalizer for the resolved CLI
	var normalizer runner.StreamNormalizer
	switch resolvedCLI {
	case defaultCLI:
		normalizer = runner.NewClaudeNormalizer()
	case "codex":
		normalizer = runner.NewCodexNormalizer()
	}

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "cli_started", map[string]string{
		"cli":       resolvedCLI,
		"iteration": fmt.Sprintf("%d", t.Iteration),
	}), log, "cli_started", t.ID)

	// Build prompt with conversation context for iterations > 1
	prompt := e.buildPrompt(ctx, t)

	model := e.cfg.DefaultModels[resolvedCLI]
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

	// If no per-session AI key, try to resolve from key registry.
	if apiKey == "" && e.keyResolver != nil {
		aiProvider := "anthropic" // default for claude-code
		if resolvedCLI == "codex" {
			aiProvider = "openai"
		}
		if resolved, err := e.keyResolver.ResolveAIKey(ctx, aiProvider); err == nil {
			apiKey = resolved
		}
	}

	result, err := cliRunner.Run(ctx, runner.RunOptions{
		Prompt:        prompt,
		WorkDir:       workDir,
		Model:         model,
		APIKey:        apiKey,
		MaxTurns:      maxTurns,
		MaxBudgetUSD:  maxBudget,
		MCPConfigPath: mcpConfigPath,
		OnEvent: func(event json.RawMessage) {
			if normalizer != nil {
				if events := normalizer.Normalize(event); len(events) > 0 {
					for _, normalized := range events {
						e.emitOrLog(e.streamer.EmitNormalized(ctx, t.ID, normalized), log, "cli_normalized", t.ID)
					}
					return
				}
			}
			e.emitOrLog(e.streamer.EmitCLIOutput(ctx, t.ID, event), log, "cli_output", t.ID)
		},
	})

	if err != nil {
		return result, err
	}

	log.Info("CLI execution completed", "exit_code", result.ExitCode, "duration", result.Duration)
	return result, nil
}

// buildPrompt constructs the prompt with conversation context for multi-turn iterations.
func (e *Executor) buildPrompt(ctx context.Context, t *session.Session) string {
	currentPrompt := t.CurrentPrompt
	if currentPrompt == "" {
		currentPrompt = t.Prompt
	}

	// Apply session type template for first iteration only
	if t.Iteration <= 1 && t.SessionType != "" && t.SessionType != "code" {
		var rendered string
		var err error

		if t.SessionType == "pr_review" && t.Config != nil {
			// PR review needs richer context (branches, PR number)
			baseBranch := t.Config.TargetBranch
			if baseBranch == "" {
				baseBranch = "main"
			}
			rendered, err = prompt.RenderPRReviewPrompt(prompt.PRReviewData{
				UserPrompt: currentPrompt,
				PRNumber:   t.Config.PRNumber,
				PRBranch:   t.Config.SourceBranch,
				BaseBranch: baseBranch,
			})
		} else {
			rendered, err = prompt.RenderTaskPrompt(t.SessionType, currentPrompt)
		}

		if err != nil {
			slog.Warn("failed to render session type template, using raw prompt",
				"session_type", t.SessionType, "error", err)
		} else {
			currentPrompt = rendered
		}
	}

	// First iteration — no context needed
	if t.Iteration <= 1 {
		return currentPrompt
	}

	// Load previous iterations for context
	iterations, err := e.sessionService.GetIterations(ctx, t.ID)
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

func (e *Executor) failSession(ctx context.Context, t *session.Session, errMsg string, startTime time.Time, log *slog.Logger) {
	log.Error("session failed", "error", errMsg)

	// Use a detached context for finalization — the original ctx may be canceled
	// (e.g. user-triggered cancel), but we still need to persist the failure state.
	finalCtx := context.WithoutCancel(ctx)

	if err := e.sessionService.SetError(finalCtx, t.ID, errMsg); err != nil {
		log.Warn("failed to set error on session", "error", err)
	}
	if err := e.sessionService.UpdateStatus(finalCtx, t.ID, session.StatusFailed); err != nil {
		log.Warn("failed to update session status to failed", "error", err)
	}
	metrics.TasksTotal.WithLabelValues(string(session.StatusFailed)).Inc()

	// Save failed iteration record
	now := time.Now().UTC()
	prompt := t.CurrentPrompt
	if prompt == "" {
		prompt = t.Prompt
	}
	if err := e.sessionService.SaveIteration(finalCtx, t.ID, session.Iteration{
		Number:    t.Iteration,
		Prompt:    prompt,
		Error:     errMsg,
		Status:    session.StatusFailed,
		StartedAt: startTime,
		EndedAt:   &now,
	}); err != nil {
		log.Warn("failed to save failed iteration", "error", err)
	}

	e.emitOrLog(e.streamer.EmitSystem(finalCtx, t.ID, "task_failed", map[string]string{
		"error": errMsg,
	}), log, "task_failed", t.ID)
	e.emitOrLog(e.streamer.EmitDone(finalCtx, t.ID, session.StatusFailed, nil), log, "task_done_failed", t.ID)

	if t.CallbackURL != "" && e.webhook != nil {
		if err := e.webhook.Send(finalCtx, t.CallbackURL, webhook.Payload{
			TaskID:     t.ID,
			Status:     string(session.StatusFailed),
			Error:      errMsg,
			TraceID:    t.TraceID,
			FinishedAt: time.Now().UTC(),
		}); err != nil {
			log.Warn("failed to send failure webhook", "error", err)
		}
	}
}

func (e *Executor) sendWebhook(ctx context.Context, t *session.Session, result string, changes *gitpkg.ChangesSummary, usage *session.UsageInfo, log *slog.Logger) {
	if err := e.webhook.Send(ctx, t.CallbackURL, webhook.Payload{
		TaskID:         t.ID,
		Status:         string(session.StatusCompleted),
		Result:         result,
		ChangesSummary: changes,
		Usage:          usage,
		TraceID:        t.TraceID,
		FinishedAt:     time.Now().UTC(),
	}); err != nil {
		log.Error("webhook delivery failed", "error", err)
	}
}

// fetchAndCheckoutPR fetches a PR ref from origin and checks out a local branch.
// This handles both same-repo and fork PRs via the pull/{number}/head ref.
func (e *Executor) fetchAndCheckoutPR(ctx context.Context, t *session.Session, workDir, prRef, localBranch string, log *slog.Logger) error {
	askPassEnv, cleanup, err := gitpkg.AskPassEnv(t.AccessToken)
	if err != nil {
		return fmt.Errorf("creating askpass for PR fetch: %w", err)
	}
	defer cleanup()

	env := os.Environ()
	if len(askPassEnv) > 0 {
		env = append(env, askPassEnv...)
	}

	// git fetch origin pull/N/head:pr-N
	fetchRefSpec := fmt.Sprintf("%s:%s", prRef, localBranch)
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", fetchRefSpec)
	fetchCmd.Dir = workDir
	fetchCmd.Env = env
	var stderr strings.Builder
	fetchCmd.Stderr = &stderr

	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("git fetch %s failed: %s", prRef, stderr.String())
	}

	// git checkout pr-N
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", localBranch)
	checkoutCmd.Dir = workDir
	stderr.Reset()
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %s", localBranch, stderr.String())
	}

	log.Info("PR branch checked out", "pr_ref", prRef, "local_branch", localBranch)
	return nil
}

// handlePRReviewCompletion parses the CLI output as ReviewResult and optionally
// posts review comments to the GitHub PR / GitLab MR.
func (e *Executor) handlePRReviewCompletion(ctx context.Context, t *session.Session, output string, log *slog.Logger) {
	// Parse ReviewResult from CLI output
	reviewResult, err := review.ParseReviewOutput(output)
	if err != nil {
		metrics.ReviewParseFailures.Inc()
		log.Warn("pr_review: failed to parse review result", "error", err)
		return
	}

	// Resolve CLI + model for reviewed_by
	cli := defaultCLI
	model := e.cfg.DefaultModels[cli]
	if t.Config != nil {
		if t.Config.CLI != "" {
			cli = t.Config.CLI
		}
		if t.Config.AIModel != "" {
			model = t.Config.AIModel
		} else if m, ok := e.cfg.DefaultModels[cli]; ok {
			model = m
		}
	}
	if model != "" {
		reviewResult.ReviewedBy = cli + ":" + model
	} else {
		reviewResult.ReviewedBy = cli
	}

	// Store ReviewResult on the session
	if err := e.sessionService.SetReviewResult(ctx, t.ID, reviewResult); err != nil {
		log.Error("pr_review: failed to store review result", "error", err)
	}

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_parsed", map[string]interface{}{
		"verdict":      reviewResult.Verdict,
		"score":        reviewResult.Score,
		"issues_count": len(reviewResult.Issues),
	}), log, "review_parsed", t.ID)

	// Post comments to PR/MR if requested
	if t.Config == nil || t.Config.OutputMode != "post_comments" || t.Config.PRNumber <= 0 {
		return
	}

	// Resolve token from provider key
	token := t.AccessToken
	if token == "" && e.keyResolver != nil {
		resolved, resolveErr := e.keyResolver.ResolveToken(ctx, t.RepoURL, "", t.ProviderKey)
		if resolveErr != nil {
			log.Error("pr_review: failed to resolve token for comment posting", "error", resolveErr)
			return
		}
		token = resolved
	}

	repo, err := gitpkg.ParseRepoURL(t.RepoURL, e.cfg.ProviderDomains)
	if err != nil {
		log.Error("pr_review: failed to parse repo URL", "error", err)
		return
	}

	log.Info("pr_review: posting review comments",
		"provider", repo.Provider,
		"pr_number", t.Config.PRNumber,
		"issues", len(reviewResult.Issues),
	)

	postResult, err := gitpkg.PostReviewComments(
		ctx, repo, token, t.Config.PRNumber, reviewResult,
		review.FormatSummaryBody, review.FormatIssueComment,
	)
	if err != nil {
		log.Error("pr_review: failed to post review comments", "error", err)
		e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_post_failed", map[string]string{
			"error": err.Error(),
		}), log, "review_post_failed", t.ID)
		return
	}

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_posted", map[string]interface{}{
		"review_url":      postResult.ReviewURL,
		"comments_posted": postResult.CommentsPosted,
	}), log, "review_posted", t.ID)

	log.Info("pr_review: review comments posted",
		"review_url", postResult.ReviewURL,
		"comments_posted", postResult.CommentsPosted,
	)
}

// resolveWorkDir finds the workspace directory for a session without cloning.
func (e *Executor) resolveWorkDir(ctx context.Context, t *session.Session) string {
	if e.workspaceMgr != nil {
		if ws := e.workspaceMgr.Get(ctx, t.ID); ws != nil && ws.Path != "" {
			return ws.Path
		}
		if t.Config != nil && t.Config.WorkspaceSessionID != "" {
			if ws := e.workspaceMgr.Get(ctx, t.Config.WorkspaceSessionID); ws != nil && ws.Path != "" {
				return ws.Path
			}
		}
	}
	return filepath.Join(e.cfg.WorkspaceBase, t.ID)
}

// executeReview runs a code review on an existing session workspace.
// Called when a session is dequeued with status=reviewing (enqueued by StartReviewAsync).
func (e *Executor) executeReview(ctx context.Context, t *session.Session) {
	ctx, span := tracing.Tracer().Start(ctx, "task.review",
		tracing.WithSessionAttributes(t.ID, t.Iteration),
	)
	defer span.End()

	log := slog.With("session_id", t.ID, "trace_id", t.TraceID, "review", true)
	startTime := time.Now().UTC()

	metrics.TasksInProgress.Inc()
	defer func() {
		metrics.TasksInProgress.Dec()
		metrics.TaskDuration.WithLabelValues("review").Observe(time.Since(startTime).Seconds())
	}()

	timeout := e.resolveTimeout(t)
	sessionCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Resolve workspace — review runs on existing workspace, no clone needed
	workDir := e.resolveWorkDir(ctx, t)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		e.failSession(ctx, t, "workspace not found for review — it may have been cleaned up", startTime, log)
		return
	}

	// Resolve CLI: review param → session config → default
	cli := defaultCLI
	if t.ReviewCLI != "" {
		cli = t.ReviewCLI
	} else if t.Config != nil && t.Config.CLI != "" {
		cli = t.Config.CLI
	}

	cliRunner, err := e.cliRegistry.Get(cli)
	if err != nil {
		e.failSession(ctx, t, fmt.Sprintf("failed to resolve CLI %q for review: %v", cli, err), startTime, log)
		return
	}

	// Resolve model: review param → session config → default for CLI
	model := e.cfg.DefaultModels[cli]
	if t.ReviewModel != "" {
		model = t.ReviewModel
	} else if t.Config != nil && t.Config.AIModel != "" {
		model = t.Config.AIModel
	}

	// Build review prompt from code_review template
	reviewPrompt, err := prompt.Render("code_review", prompt.CodeReviewData{
		OriginalPrompt: t.Prompt,
	})
	if err != nil {
		e.failSession(ctx, t, fmt.Sprintf("failed to render review prompt: %v", err), startTime, log)
		return
	}

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_started", map[string]string{
		"cli":   cli,
		"model": model,
	}), log, "review_started", t.ID)

	// Select stream normalizer for CLI output
	var normalizer runner.StreamNormalizer
	switch cli {
	case defaultCLI:
		normalizer = runner.NewClaudeNormalizer()
	case "codex":
		normalizer = runner.NewCodexNormalizer()
	}

	apiKey := ""
	if t.Config != nil {
		apiKey = t.Config.AIApiKey
	}

	// If no per-session AI key, try to resolve from key registry.
	if apiKey == "" && e.keyResolver != nil {
		aiProvider := "anthropic" // default for claude-code
		if cli == "codex" {
			aiProvider = "openai"
		}
		if resolved, resolveErr := e.keyResolver.ResolveAIKey(ctx, aiProvider); resolveErr == nil {
			apiKey = resolved
		}
	}

	// Run CLI with streaming
	result, err := cliRunner.Run(sessionCtx, runner.RunOptions{
		Prompt:  reviewPrompt,
		WorkDir: workDir,
		Model:   model,
		APIKey:  apiKey,
		OnEvent: func(event json.RawMessage) {
			if normalizer != nil {
				if events := normalizer.Normalize(event); len(events) > 0 {
					for _, normalized := range events {
						e.emitOrLog(e.streamer.EmitNormalized(ctx, t.ID, normalized), log, "review_normalized", t.ID)
					}
					return
				}
			}
			e.emitOrLog(e.streamer.EmitCLIOutput(ctx, t.ID, event), log, "review_cli_output", t.ID)
		},
	})
	if err != nil {
		if sessionCtx.Err() == context.DeadlineExceeded {
			e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_timeout", map[string]interface{}{
				"timeout_seconds": timeout,
			}), log, "review_timeout", t.ID)
			e.failSession(ctx, t, fmt.Sprintf("review timed out after %ds", timeout), startTime, log)
		} else if ctx.Err() == context.Canceled {
			e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_canceled", nil), log, "review_canceled", t.ID)
			e.failSession(context.Background(), t, "review canceled by user", startTime, log)
		} else {
			e.failSession(ctx, t, fmt.Sprintf("review CLI execution failed: %v", err), startTime, log)
		}
		return
	}

	// Parse ReviewResult from CLI output
	reviewResult, parseErr := review.ParseReviewOutput(result.Output)
	if parseErr != nil {
		metrics.ReviewParseFailures.Inc()
		log.Warn("failed to parse review output", "error", parseErr)
		output := result.Output
		if len(output) > 500 {
			output = output[:500]
		}
		reviewResult = &review.ReviewResult{
			Verdict: review.VerdictComment,
			Summary: output,
		}
	}

	if model != "" {
		reviewResult.ReviewedBy = cli + ":" + model
	} else {
		reviewResult.ReviewedBy = cli
	}
	reviewResult.DurationSeconds = time.Since(startTime).Seconds()

	// Store raw result + usage
	usage := &session.UsageInfo{
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		DurationSeconds: int(result.Duration.Seconds()),
	}
	if err := e.sessionService.SetResult(ctx, t.ID, result.Output, nil, usage); err != nil {
		log.Error("failed to store review result", "error", err)
	}

	// Complete review: store ReviewResult + transition reviewing → completed
	if err := e.sessionService.CompleteReview(ctx, t.ID, reviewResult); err != nil {
		log.Error("failed to complete review", "error", err)
		e.failSession(ctx, t, fmt.Sprintf("failed to complete review: %v", err), startTime, log)
		return
	}

	metrics.TasksTotal.WithLabelValues(string(session.StatusCompleted)).Inc()

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "review_completed", map[string]interface{}{
		"verdict":      reviewResult.Verdict,
		"score":        reviewResult.Score,
		"issues_count": len(reviewResult.Issues),
		"duration":     reviewResult.DurationSeconds,
	}), log, "review_completed", t.ID)

	// Auto-post review to MR if configured
	if t.Config != nil && t.Config.AutoPostReview && t.PRNumber > 0 {
		e.autoPostReview(ctx, t, reviewResult, log)
	} else if t.Config != nil && t.Config.AutoPostReview && t.Config.PRNumber > 0 {
		// Use config PR number (for pr_review tasks)
		e.autoPostReviewToPR(ctx, t, t.Config.PRNumber, reviewResult, log)
	}

	e.emitOrLog(e.streamer.EmitDone(ctx, t.ID, session.StatusCompleted, nil), log, "review_done", t.ID)

	if t.CallbackURL != "" && e.webhook != nil {
		if err := e.webhook.Send(ctx, t.CallbackURL, webhook.Payload{
			TaskID:     t.ID,
			Status:     string(session.StatusCompleted),
			Result:     result.Output,
			Usage:      usage,
			TraceID:    t.TraceID,
			FinishedAt: time.Now().UTC(),
		}); err != nil {
			log.Warn("failed to send review completion webhook", "error", err)
		}
	}

	log.Info("review completed",
		"verdict", reviewResult.Verdict,
		"score", reviewResult.Score,
		"duration", result.Duration,
	)
}

// autoPostReview posts review results to the session's own PR (created via create-pr).
func (e *Executor) autoPostReview(ctx context.Context, t *session.Session, reviewResult *review.ReviewResult, log *slog.Logger) {
	e.autoPostReviewToPR(ctx, t, t.PRNumber, reviewResult, log)
}

// autoPostReviewToPR posts review results to a specific PR number.
func (e *Executor) autoPostReviewToPR(ctx context.Context, t *session.Session, prNumber int, reviewResult *review.ReviewResult, log *slog.Logger) {
	token := t.AccessToken
	if token == "" && e.keyResolver != nil {
		resolved, err := e.keyResolver.ResolveToken(ctx, t.RepoURL, "", t.ProviderKey)
		if err != nil {
			log.Error("auto-post: failed to resolve token", "error", err)
			return
		}
		token = resolved
	}

	repo, err := gitpkg.ParseRepoURL(t.RepoURL, e.cfg.ProviderDomains)
	if err != nil {
		log.Error("auto-post: failed to parse repo URL", "error", err)
		return
	}

	log.Info("auto-post: posting review to MR",
		"provider", repo.Provider,
		"pr_number", prNumber,
		"verdict", reviewResult.Verdict,
	)

	postResult, err := gitpkg.PostReviewComments(
		ctx, repo, token, prNumber, reviewResult,
		review.FormatSummaryBody, review.FormatIssueComment,
	)
	if err != nil {
		log.Error("auto-post: failed to post review", "error", err)
		e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "auto_review_post_failed", map[string]string{
			"error": err.Error(),
		}), log, "auto_review_post_failed", t.ID)
		return
	}

	e.emitOrLog(e.streamer.EmitSystem(ctx, t.ID, "auto_review_posted", map[string]interface{}{
		"review_url":      postResult.ReviewURL,
		"comments_posted": postResult.CommentsPosted,
	}), log, "auto_review_posted", t.ID)

	log.Info("auto-post: review posted",
		"review_url", postResult.ReviewURL,
		"comments_posted", postResult.CommentsPosted,
	)
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
