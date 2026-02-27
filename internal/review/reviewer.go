package review

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/tool/runner"
)

// TaskProvider retrieves and transitions tasks for review.
type TaskProvider interface {
	StartReview(ctx context.Context, taskID string) error
	CompleteReview(ctx context.Context, taskID string, result *ReviewResult) error
	GetTask(ctx context.Context, taskID string) (TaskInfo, error)
}

// TaskInfo is the minimal task data the reviewer needs.
type TaskInfo struct {
	ID          string
	Prompt      string
	AIApiKey    string
	CLI         string
	WorkDir     string // resolved workspace path
}

// EventEmitter publishes streaming events.
type EventEmitter interface {
	EmitSystem(ctx context.Context, taskID, event string, data interface{}) error
}

// ReviewerConfig holds default configuration for the reviewer.
type ReviewerConfig struct {
	DefaultModels map[string]string // CLI name → default model
}

// Reviewer handles code review execution as a user action.
type Reviewer struct {
	taskProvider TaskProvider
	cliRegistry  *runner.Registry
	emitter      EventEmitter
	cfg          ReviewerConfig
}

// NewReviewer creates a new Reviewer.
func NewReviewer(
	taskProvider TaskProvider,
	cliRegistry *runner.Registry,
	emitter EventEmitter,
	cfg ReviewerConfig,
) *Reviewer {
	return &Reviewer{
		taskProvider: taskProvider,
		cliRegistry:  cliRegistry,
		emitter:      emitter,
		cfg:          cfg,
	}
}

// Review runs a code review on a completed task.
// cli and model are optional overrides (empty = use defaults).
func (r *Reviewer) Review(ctx context.Context, taskID, cli, model string) (*ReviewResult, error) {
	log := slog.With("task_id", taskID)

	// 1. Get task info
	info, err := r.taskProvider.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// 2. Transition to reviewing
	if err := r.taskProvider.StartReview(ctx, taskID); err != nil {
		return nil, err
	}

	// 3. Run the review (on error, still complete the review with an error result)
	result := r.runReview(ctx, info, cli, model, log)

	// 4. Store result and transition back to completed
	if err := r.taskProvider.CompleteReview(ctx, taskID, result); err != nil {
		log.Error("failed to complete review", "error", err)
		return result, fmt.Errorf("completing review: %w", err)
	}

	return result, nil
}

func (r *Reviewer) runReview(ctx context.Context, info TaskInfo, cli, model string, log *slog.Logger) *ReviewResult {
	startTime := time.Now()

	_ = r.emitter.EmitSystem(ctx, info.ID, "review_started", nil)

	// Resolve CLI: param → task CLI → default "claude-code"
	resolvedCLI := "claude-code"
	if cli != "" {
		resolvedCLI = cli
	} else if info.CLI != "" {
		resolvedCLI = info.CLI
	}

	cliRunner, err := r.cliRegistry.Get(resolvedCLI)
	if err != nil {
		log.Error("review: failed to resolve CLI", "cli", resolvedCLI, "error", err)
		return &ReviewResult{
			Verdict:    VerdictComment,
			Summary:    fmt.Sprintf("Review failed: could not resolve CLI %q", resolvedCLI),
			ReviewedBy: resolvedCLI,
		}
	}

	// Resolve model: param → default for CLI
	resolvedModel := r.cfg.DefaultModels[resolvedCLI]
	if model != "" {
		resolvedModel = model
	}

	// Build review prompt from template
	reviewPrompt, err := prompt.Render("code_review", prompt.CodeReviewData{
		OriginalPrompt: info.Prompt,
	})
	if err != nil {
		log.Error("review: failed to render prompt", "error", err)
		return &ReviewResult{
			Verdict:    VerdictComment,
			Summary:    fmt.Sprintf("Review failed: prompt render error: %v", err),
			ReviewedBy: resolvedCLI + ":" + resolvedModel,
		}
	}

	result, err := cliRunner.Run(ctx, runner.RunOptions{
		Prompt:  reviewPrompt,
		WorkDir: info.WorkDir,
		Model:   resolvedModel,
		APIKey:  info.AIApiKey,
	})
	if err != nil {
		log.Error("review: CLI execution failed", "error", err)
		return &ReviewResult{
			Verdict:         VerdictComment,
			Summary:         fmt.Sprintf("Review failed: CLI error: %v", err),
			ReviewedBy:      resolvedCLI + ":" + resolvedModel,
			DurationSeconds: time.Since(startTime).Seconds(),
		}
	}

	// Parse review output
	reviewResult, err := ParseReviewOutput(result.Output)
	if err != nil {
		log.Warn("review: parse error", "error", err)
		output := result.Output
		if len(output) > 500 {
			output = output[:500]
		}
		reviewResult = &ReviewResult{
			Verdict: VerdictComment,
			Summary: output,
		}
	}

	reviewResult.ReviewedBy = resolvedCLI + ":" + resolvedModel
	reviewResult.DurationSeconds = time.Since(startTime).Seconds()

	_ = r.emitter.EmitSystem(ctx, info.ID, "review_completed", map[string]interface{}{
		"verdict":      reviewResult.Verdict,
		"score":        reviewResult.Score,
		"issues_count": len(reviewResult.Issues),
	})

	log.Info("review completed",
		"verdict", reviewResult.Verdict,
		"score", reviewResult.Score,
		"issues", len(reviewResult.Issues),
		"duration", reviewResult.DurationSeconds,
	)

	return reviewResult
}
