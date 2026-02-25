package workflow

import "context"

// RunStore persists workflow run state.
type RunStore interface {
	CreateRun(ctx context.Context, run WorkflowRun) error
	GetRun(ctx context.Context, runID string) (*WorkflowRun, error)
	UpdateRunStatus(ctx context.Context, runID string, status RunStatus, errMsg string) error
	ListRuns(ctx context.Context, workflowName string) ([]WorkflowRun, error)
	UpsertStep(ctx context.Context, step WorkflowRunStep) error
	GetSteps(ctx context.Context, runID string) ([]WorkflowRunStep, error)
}
