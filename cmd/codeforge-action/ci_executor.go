package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/review"
	"github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tool/runner"
	"github.com/freema/codeforge/internal/workflow"
)

// CIExecutor runs a single CI task — no Redis, no queue, no HTTP server.
type CIExecutor struct {
	cfg Config
}

// NewCIExecutor creates a CI executor with the given configuration.
func NewCIExecutor(cfg Config) *CIExecutor {
	return &CIExecutor{cfg: cfg}
}

// Execute runs the CI task and returns an exit code (0 = success, 1 = failure/request_changes).
func (e *CIExecutor) Execute(ctx context.Context) int {
	ciCtx, err := DetectCIContext()
	if err != nil {
		slog.Error("failed to detect CI context", "error", err)
		return 1
	}

	slog.Info("CI context detected",
		"platform", ciCtx.Platform,
		"repo", ciCtx.RepoURL,
		"pr_number", ciCtx.PRNumber,
		"base_branch", ciCtx.BaseBranch,
		"head_sha", ciCtx.HeadSHA,
		"work_dir", ciCtx.WorkDir,
	)

	// Ensure CLI is available
	if err := e.ensureCLI(ctx); err != nil {
		slog.Error("failed to install CLI", "cli", e.cfg.CLI, "error", err)
		return 1
	}

	// Write MCP config if provided
	mcpConfigPath, err := e.writeMCPConfig(ciCtx.WorkDir)
	if err != nil {
		slog.Error("failed to write MCP config", "error", err)
		return 1
	}

	// Build prompt
	taskPrompt, err := e.buildPrompt(ciCtx)
	if err != nil {
		slog.Error("failed to build prompt", "error", err)
		return 1
	}

	// Build system context from .codeforge/ and CLAUDE.md
	systemContext := e.buildSystemContext(ciCtx.WorkDir)

	// Create runner
	cliRunner := e.createRunner()

	slog.Info("running CLI",
		"cli", e.cfg.CLI,
		"model", e.cfg.Model,
		"task_type", e.cfg.TaskType,
		"has_system_context", systemContext != "",
	)

	startTime := time.Now()

	// Default allowed tools for review tasks — read-only to prevent wandering
	allowedTools := e.cfg.AllowedTools
	if allowedTools == "" && (e.cfg.TaskType == "pr_review" || e.cfg.TaskType == "code_review") {
		allowedTools = "Bash,Read,Glob,Grep"
	}

	// Run CLI
	result, err := cliRunner.Run(ctx, runner.RunOptions{
		Prompt:             taskPrompt,
		WorkDir:            ciCtx.WorkDir,
		Model:              e.cfg.Model,
		APIKey:             e.cfg.APIKey,
		MaxTurns:           e.cfg.MaxTurns,
		MCPConfigPath:      mcpConfigPath,
		AppendSystemPrompt: systemContext,
		AllowedTools:       allowedTools,
		OnEvent: func(event json.RawMessage) {
			// Stream events to stderr for CI log visibility
			fmt.Fprintln(os.Stderr, string(event))
		},
	})

	duration := time.Since(startTime)

	if err != nil && result == nil {
		slog.Error("CLI execution failed", "error", err, "duration", duration)
		return 1
	}

	slog.Info("CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"output_length", len(result.Output),
	)

	// Log raw output for debugging when it's short enough
	if len(result.Output) > 0 && len(result.Output) < 2000 {
		slog.Info("CLI raw output", "output", result.Output)
	} else if len(result.Output) == 0 {
		slog.Warn("CLI returned empty output")
	}

	// Handle result based on task type
	return e.handleResult(ctx, ciCtx, result, duration)
}

// ensureCLI checks if the CLI binary is available, installs it if needed.
func (e *CIExecutor) ensureCLI(ctx context.Context) error {
	var binaryName string
	switch e.cfg.CLI {
	case cliClaudeCode:
		binaryName = "claude"
	case cliCodex:
		binaryName = "codex"
	default:
		return fmt.Errorf("unknown CLI: %s", e.cfg.CLI)
	}

	// Check if already available
	if _, err := exec.LookPath(binaryName); err == nil {
		slog.Info("CLI already available", "cli", e.cfg.CLI)
		return nil
	}

	slog.Info("installing CLI", "cli", e.cfg.CLI)

	var pkg string
	switch e.cfg.CLI {
	case cliClaudeCode:
		pkg = "@anthropic-ai/claude-code"
	case cliCodex:
		pkg = "@openai/codex"
	}

	cmd := exec.CommandContext(ctx, "npm", "install", "-g", pkg)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install %s: %w", pkg, err)
	}

	// Verify installation
	if _, err := exec.LookPath(binaryName); err != nil {
		return fmt.Errorf("CLI %q not found after installation", binaryName)
	}

	slog.Info("CLI installed successfully", "cli", e.cfg.CLI)
	return nil
}

// writeMCPConfig writes the MCP configuration to .mcp.json if provided.
func (e *CIExecutor) writeMCPConfig(workDir string) (string, error) {
	if e.cfg.MCPConfig == "" {
		return "", nil
	}

	mcpPath := filepath.Join(workDir, ".mcp.json")

	// If it's a path to an existing file, use it directly
	if _, err := os.Stat(e.cfg.MCPConfig); err == nil {
		return e.cfg.MCPConfig, nil
	}

	// Otherwise treat as JSON string
	if err := os.WriteFile(mcpPath, []byte(e.cfg.MCPConfig), 0644); err != nil {
		return "", fmt.Errorf("writing .mcp.json: %w", err)
	}

	return mcpPath, nil
}

// buildPrompt constructs the prompt based on task type and CI context.
func (e *CIExecutor) buildPrompt(ciCtx *CIContext) (string, error) {
	switch e.cfg.TaskType {
	case "pr_review":
		return e.buildPRReviewPrompt(ciCtx)
	case "code_review":
		return e.buildCodeReviewPrompt(ciCtx)
	case "knowledge_update":
		return e.buildKnowledgeUpdatePrompt()
	case "custom":
		return e.cfg.Prompt, nil
	default:
		return "", fmt.Errorf("unknown task type: %s", e.cfg.TaskType)
	}
}

func (e *CIExecutor) buildPRReviewPrompt(ciCtx *CIContext) (string, error) {
	userPrompt := e.cfg.Prompt
	prNumber := ciCtx.PRNumber
	prBranch := ciCtx.PRBranch
	baseBranch := ciCtx.BaseBranch

	if baseBranch == "" {
		baseBranch = "main"
	}

	return prompt.RenderPRReviewPrompt(prompt.PRReviewData{
		UserPrompt: userPrompt,
		PRNumber:   prNumber,
		PRBranch:   prBranch,
		BaseBranch: baseBranch,
	})
}

func (e *CIExecutor) buildCodeReviewPrompt(ciCtx *CIContext) (string, error) {
	baseBranch := ciCtx.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	userPrompt := e.cfg.Prompt
	if userPrompt == "" {
		userPrompt = "Review the changes on this branch compared to " + baseBranch
	}

	return prompt.Render("code_review", prompt.CodeReviewData{
		OriginalPrompt: userPrompt,
	})
}

func (e *CIExecutor) buildKnowledgeUpdatePrompt() (string, error) {
	// Two-phase: analyze then update — combined into one prompt for CI mode
	return workflow.AnalyzeRepoPrompt + "\n\n---\n\n" + workflow.UpdateKnowledgePrompt, nil
}

// buildSystemContext reads .codeforge/ knowledge files and CLAUDE.md to build system context.
func (e *CIExecutor) buildSystemContext(workDir string) string {
	var ctx strings.Builder

	// Read .codeforge/ knowledge files
	for _, f := range []string{"OVERVIEW.md", "ARCHITECTURE.md", "CONVENTIONS.md"} {
		path := filepath.Join(workDir, ".codeforge", f)
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			ctx.WriteString("# " + strings.TrimSuffix(f, ".md") + "\n\n")
			ctx.WriteString(string(data))
			ctx.WriteString("\n\n")
		}
	}

	// Read CLAUDE.md (project instructions)
	claudeMD := filepath.Join(workDir, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMD); err == nil && len(data) > 0 {
		ctx.WriteString("# Project Instructions\n\n")
		ctx.WriteString(string(data))
		ctx.WriteString("\n\n")
	}

	return ctx.String()
}

// createRunner creates the appropriate CLI runner.
func (e *CIExecutor) createRunner() runner.Runner {
	switch e.cfg.CLI {
	case cliCodex:
		return runner.NewCodexRunner("codex")
	default:
		return runner.NewClaudeRunner("claude")
	}
}

// handleResult processes the CLI output based on task type.
func (e *CIExecutor) handleResult(ctx context.Context, ciCtx *CIContext, result *runner.RunResult, duration time.Duration) int {
	switch e.cfg.TaskType {
	case "pr_review", "code_review":
		return e.handleReviewResult(ctx, ciCtx, result, duration)
	case "knowledge_update":
		return e.handleKnowledgeResult(ctx, ciCtx, result)
	case "custom":
		return e.handleCustomResult(ciCtx, result)
	default:
		fmt.Println(result.Output)
		return 0
	}
}

func (e *CIExecutor) handleReviewResult(ctx context.Context, ciCtx *CIContext, result *runner.RunResult, duration time.Duration) int {
	// Parse review output
	reviewResult, err := review.ParseReviewOutput(result.Output)
	if err != nil {
		slog.Warn("failed to parse review output, using raw output", "error", err)
		fmt.Println(result.Output)
		return 0
	}

	reviewResult.DurationSeconds = duration.Seconds()
	reviewResult.ReviewedBy = e.cfg.CLI + ":" + e.cfg.Model

	// Write output based on format
	e.writeOutput(ciCtx, reviewResult, result.Output)

	// Post comments if enabled and we have a PR
	if e.cfg.PostComments && ciCtx.PRNumber > 0 && e.cfg.ProviderToken != "" {
		if err := e.postReviewComments(ctx, ciCtx, reviewResult); err != nil {
			slog.Error("failed to post review comments", "error", err)
			// Don't fail the action for comment posting failures
		}
	}

	// Exit code based on verdict
	if reviewResult.Verdict == review.VerdictRequestChanges {
		return 1
	}
	return 0
}

func (e *CIExecutor) handleKnowledgeResult(_ context.Context, ciCtx *CIContext, result *runner.RunResult) int {
	slog.Info("knowledge update completed", "output_length", len(result.Output))

	// Write output
	e.writeOutput(ciCtx, nil, result.Output)

	if result.ExitCode != 0 {
		return 1
	}
	return 0
}

func (e *CIExecutor) handleCustomResult(ciCtx *CIContext, result *runner.RunResult) int {
	e.writeOutput(ciCtx, nil, result.Output)

	if result.ExitCode != 0 {
		return 1
	}
	return 0
}

// writeOutput writes results to the appropriate CI output mechanism.
func (e *CIExecutor) writeOutput(ciCtx *CIContext, reviewResult *review.ReviewResult, rawOutput string) {
	switch ciCtx.Platform {
	case PlatformGitHub:
		writeGitHubOutput(ciCtx, reviewResult, rawOutput, e.cfg.OutputFormat)
	case PlatformGitLab:
		writeGitLabOutput(reviewResult, rawOutput, e.cfg.OutputFormat)
	default:
		// Fallback: write to stdout
		if reviewResult != nil && e.cfg.OutputFormat == "json" {
			data, _ := json.MarshalIndent(reviewResult, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println(rawOutput)
		}
	}
}

// postReviewComments posts review results as PR/MR comments.
func (e *CIExecutor) postReviewComments(ctx context.Context, ciCtx *CIContext, reviewResult *review.ReviewResult) error {
	repoURL := ciCtx.RepoURL
	if repoURL == "" {
		return fmt.Errorf("repo URL not available")
	}

	repoInfo, err := git.ParseRepoURL(repoURL, nil)
	if err != nil {
		return fmt.Errorf("parsing repo URL: %w", err)
	}

	postResult, err := git.PostReviewComments(
		ctx,
		repoInfo,
		e.cfg.ProviderToken,
		ciCtx.PRNumber,
		reviewResult,
		review.FormatSummaryBody,
		review.FormatIssueComment,
	)
	if err != nil {
		return err
	}

	slog.Info("review comments posted",
		"review_url", postResult.ReviewURL,
		"comments_posted", postResult.CommentsPosted,
	)
	return nil
}
