package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/redisclient"
)

// OrchestratorConfig holds orchestrator configuration.
type OrchestratorConfig struct {
	ContextTTLHours    int
	MaxRunDurationSec  int
}

// Orchestrator manages workflow execution via a Redis FIFO queue.
type Orchestrator struct {
	registry      Registry
	runStore      RunStore
	fetchExecutor *FetchExecutor
	taskExecutor  *TaskExecutor
	actionExecutor *ActionExecutor
	streamer      *Streamer
	redis         *redisclient.Client
	cfg           OrchestratorConfig
}

// NewOrchestrator creates a new workflow orchestrator.
func NewOrchestrator(
	registry Registry,
	runStore RunStore,
	fetchExecutor *FetchExecutor,
	taskExecutor *TaskExecutor,
	actionExecutor *ActionExecutor,
	streamer *Streamer,
	redis *redisclient.Client,
	cfg OrchestratorConfig,
) *Orchestrator {
	return &Orchestrator{
		registry:       registry,
		runStore:       runStore,
		fetchExecutor:  fetchExecutor,
		taskExecutor:   taskExecutor,
		actionExecutor: actionExecutor,
		streamer:       streamer,
		redis:          redis,
		cfg:            cfg,
	}
}

// Start begins the orchestrator loop, consuming workflow runs from the queue.
func (o *Orchestrator) Start(ctx context.Context) {
	queueKey := o.redis.Key("queue:workflows")
	slog.Info("workflow orchestrator started", "queue", queueKey)

	for {
		// BLPOP blocks until a run ID is available or context is cancelled
		result, err := o.redis.Unwrap().BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("workflow orchestrator stopping")
				return
			}
			continue
		}

		runID := result[1]
		slog.Info("workflow run dequeued", "run_id", runID)

		o.executeRun(ctx, runID)
	}
}

// CreateRun validates parameters, creates a WorkflowRun, and enqueues it.
func (o *Orchestrator) CreateRun(ctx context.Context, workflowName string, params map[string]string) (*WorkflowRun, error) {
	def, err := o.registry.Get(ctx, workflowName)
	if err != nil {
		return nil, err
	}

	// Validate and fill default params
	resolvedParams, err := validateParams(def.Parameters, params)
	if err != nil {
		return nil, err
	}

	run := WorkflowRun{
		ID:           uuid.New().String(),
		WorkflowName: workflowName,
		Status:       RunStatusPending,
		Params:       resolvedParams,
		CreatedAt:    time.Now().UTC(),
	}

	if err := o.runStore.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("creating workflow run: %w", err)
	}

	// Enqueue
	queueKey := o.redis.Key("queue:workflows")
	if err := o.redis.Unwrap().RPush(ctx, queueKey, run.ID).Err(); err != nil {
		return nil, fmt.Errorf("enqueuing workflow run: %w", err)
	}

	slog.Info("workflow run created", "run_id", run.ID, "workflow", workflowName)
	return &run, nil
}

func (o *Orchestrator) executeRun(ctx context.Context, runID string) {
	log := slog.With("run_id", runID)

	run, err := o.runStore.GetRun(ctx, runID)
	if err != nil {
		log.Error("failed to load workflow run", "error", err)
		return
	}

	def, err := o.registry.Get(ctx, run.WorkflowName)
	if err != nil {
		log.Error("failed to load workflow definition", "error", err)
		o.failRun(ctx, runID, fmt.Sprintf("workflow definition not found: %v", err))
		return
	}

	// Apply run timeout
	timeout := time.Duration(o.cfg.MaxRunDurationSec) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Mark as running
	if err := o.runStore.UpdateRunStatus(ctx, runID, RunStatusRunning, ""); err != nil {
		log.Error("failed to update run status", "error", err)
		return
	}

	_ = o.streamer.EmitSystem(ctx, runID, "workflow_started", map[string]string{
		"workflow": run.WorkflowName,
		"run_id":   runID,
	})

	// Build template context
	tctx := TemplateContext{
		Params: run.Params,
		Steps:  make(map[string]map[string]string),
	}

	// Store context in Redis for persistence
	contextKey := o.redis.Key("workflow", runID, "context")
	contextTTL := time.Duration(o.cfg.ContextTTLHours) * time.Hour

	// Execute steps sequentially
	for _, stepDef := range def.Steps {
		log := log.With("step", stepDef.Name, "type", stepDef.Type)

		now := time.Now().UTC()
		// Record step start
		_ = o.runStore.UpsertStep(ctx, WorkflowRunStep{
			RunID:     runID,
			StepName:  stepDef.Name,
			StepType:  stepDef.Type,
			Status:    StepStatusRunning,
			StartedAt: &now,
		})

		_ = o.streamer.EmitSystem(ctx, runID, "step_started", map[string]string{
			"step": stepDef.Name,
			"type": string(stepDef.Type),
		})

		outputs, err := o.executeStep(runCtx, stepDef, tctx)
		if err != nil {
			log.Error("step failed", "error", err)

			finishedAt := time.Now().UTC()
			_ = o.runStore.UpsertStep(ctx, WorkflowRunStep{
				RunID:      runID,
				StepName:   stepDef.Name,
				StepType:   stepDef.Type,
				Status:     StepStatusFailed,
				Error:      err.Error(),
				FinishedAt: &finishedAt,
			})

			_ = o.streamer.EmitSystem(ctx, runID, "step_failed", map[string]interface{}{
				"step":  stepDef.Name,
				"error": err.Error(),
			})

			o.failRun(ctx, runID, fmt.Sprintf("step '%s' failed: %v", stepDef.Name, err))
			return
		}

		// Store outputs in template context
		tctx.Steps[stepDef.Name] = outputs

		// Persist to Redis context hash
		for k, v := range outputs {
			field := stepDef.Name + "." + k
			o.redis.Unwrap().HSet(ctx, contextKey, field, v)
		}
		o.redis.Unwrap().Expire(ctx, contextKey, contextTTL)

		finishedAt := time.Now().UTC()

		// Determine task_id from outputs (for task steps)
		taskID := ""
		if outputs != nil {
			taskID = outputs["task_id"]
		}

		_ = o.runStore.UpsertStep(ctx, WorkflowRunStep{
			RunID:      runID,
			StepName:   stepDef.Name,
			StepType:   stepDef.Type,
			Status:     StepStatusCompleted,
			Result:     outputs,
			TaskID:     taskID,
			FinishedAt: &finishedAt,
		})

		_ = o.streamer.EmitSystem(ctx, runID, "step_completed", map[string]string{
			"step": stepDef.Name,
		})

		log.Info("step completed")
	}

	// Mark run as completed
	if err := o.runStore.UpdateRunStatus(ctx, runID, RunStatusCompleted, ""); err != nil {
		log.Error("failed to mark run as completed", "error", err)
	}

	_ = o.streamer.EmitDone(ctx, runID, RunStatusCompleted)
	log.Info("workflow run completed")
}

func (o *Orchestrator) executeStep(ctx context.Context, stepDef StepDefinition, tctx TemplateContext) (map[string]string, error) {
	switch stepDef.Type {
	case StepTypeFetch:
		return o.fetchExecutor.Execute(ctx, stepDef, tctx)
	case StepTypeTask:
		return o.taskExecutor.Execute(ctx, stepDef, tctx)
	case StepTypeAction:
		return o.actionExecutor.Execute(ctx, stepDef, tctx)
	default:
		return nil, fmt.Errorf("unknown step type: %s", stepDef.Type)
	}
}

func (o *Orchestrator) failRun(ctx context.Context, runID, errMsg string) {
	if err := o.runStore.UpdateRunStatus(ctx, runID, RunStatusFailed, errMsg); err != nil {
		slog.Error("failed to mark run as failed", "run_id", runID, "error", err)
	}
	_ = o.streamer.EmitDone(ctx, runID, RunStatusFailed)
}

func validateParams(defs []ParameterDefinition, params map[string]string) (map[string]string, error) {
	resolved := make(map[string]string)

	// Copy provided params
	for k, v := range params {
		resolved[k] = v
	}

	// Check required and fill defaults
	for _, def := range defs {
		if _, ok := resolved[def.Name]; !ok {
			if def.Default != "" {
				resolved[def.Name] = def.Default
			} else if def.Required {
				return nil, apperror.Validation("missing required parameter: %s", def.Name)
			}
		}
	}

	return resolved, nil
}
