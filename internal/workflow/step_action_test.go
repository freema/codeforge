package workflow

import (
	"context"
	"testing"

	"github.com/freema/codeforge/internal/task"
)

type mockPRCreator struct {
	result *task.CreatePRResponse
	err    error
}

func (m *mockPRCreator) CreatePR(_ context.Context, _ string, _ task.CreatePRRequest) (*task.CreatePRResponse, error) {
	return m.result, m.err
}

func TestActionExecutor_CreatePR(t *testing.T) {
	pr := &mockPRCreator{
		result: &task.CreatePRResponse{
			PRURL:    "https://github.com/owner/repo/pull/1",
			PRNumber: 1,
			Branch:   "codeforge/fix-bug",
		},
	}
	executor := NewActionExecutor(pr)

	stepDef := StepDefinition{
		Name: "create_pr",
		Type: StepTypeAction,
		Config: mustJSON(ActionConfig{
			Kind:        ActionCreatePR,
			TaskStepRef: "fix_bug",
			Title:       "fix: {{.Steps.fetch_issue.title}}",
			Description: "Fixes issue",
		}),
	}

	tctx := TemplateContext{
		Steps: map[string]map[string]string{
			"fetch_issue": {"title": "NPE in handler"},
			"fix_bug":     {"task_id": "task-123"},
		},
	}

	outputs, err := executor.Execute(context.Background(), stepDef, tctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if outputs["pr_url"] != "https://github.com/owner/repo/pull/1" {
		t.Errorf("pr_url = %q", outputs["pr_url"])
	}
	if outputs["pr_number"] != "1" {
		t.Errorf("pr_number = %q", outputs["pr_number"])
	}
}

func TestActionExecutor_MissingTaskRef(t *testing.T) {
	executor := NewActionExecutor(nil)

	stepDef := StepDefinition{
		Name: "create_pr",
		Type: StepTypeAction,
		Config: mustJSON(ActionConfig{
			Kind:        ActionCreatePR,
			TaskStepRef: "nonexistent",
		}),
	}

	tctx := TemplateContext{Steps: map[string]map[string]string{}}
	_, err := executor.Execute(context.Background(), stepDef, tctx)
	if err == nil {
		t.Fatal("expected error for missing task ref")
	}
}

func TestActionExecutor_Notify(t *testing.T) {
	executor := NewActionExecutor(nil)

	stepDef := StepDefinition{
		Name:   "notify",
		Type:   StepTypeAction,
		Config: mustJSON(ActionConfig{Kind: ActionNotify}),
	}

	outputs, err := executor.Execute(context.Background(), stepDef, TemplateContext{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outputs["status"] != "skipped" {
		t.Errorf("expected status=skipped, got %v", outputs)
	}
}
