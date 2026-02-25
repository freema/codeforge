package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/prompt"
)

// codeReviewPrompt is pre-rendered at init time. The prompt template's
// {{.OriginalPrompt}} is filled with a workflow template expression so
// the actual task prompt gets substituted at workflow runtime.
var codeReviewPrompt = mustRenderCodeReviewPrompt()

func mustRenderCodeReviewPrompt() string {
	s, err := prompt.Render("code_review", prompt.CodeReviewData{
		OriginalPrompt: "{{.Params.prompt}}",
	})
	if err != nil {
		panic("code_review prompt: " + err.Error())
	}
	return s
}

// BuiltinWorkflows defines the set of built-in workflow definitions.
var BuiltinWorkflows = []WorkflowDefinition{
	{
		Name:        "sentry-fixer",
		Description: "Fetches a Sentry issue, creates a task to fix it, then creates a PR",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "fetch_issue",
				Type: StepTypeFetch,
				Config: mustJSON(FetchConfig{
					URL:     "{{.Params.sentry_url}}/api/0/issues/{{.Params.issue_id}}/",
					Method:  "GET",
					KeyName: "{{.Params.key_name}}",
					Outputs: map[string]string{
						"title":    "$.title",
						"culprit":  "$.culprit",
						"message":  "$.metadata.value",
						"platform": "$.platform",
					},
				}),
			},
			{
				Name: "fix_bug",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:     "{{.Params.repo_url}}",
					Prompt:      "Fix the following Sentry error:\n\nTitle: {{.Steps.fetch_issue.title}}\nCulprit: {{.Steps.fetch_issue.culprit}}\nMessage: {{.Steps.fetch_issue.message}}\nPlatform: {{.Steps.fetch_issue.platform}}\n\nAnalyze the code, find the root cause, and implement a fix.",
					ProviderKey: "{{.Params.provider_key}}",
				}),
			},
			{
				Name: "create_pr",
				Type: StepTypeAction,
				Config: mustJSON(ActionConfig{
					Kind:        ActionCreatePR,
					TaskStepRef: "fix_bug",
					Title:       "fix: {{.Steps.fetch_issue.title}}",
					Description: "Fixes Sentry issue: {{.Steps.fetch_issue.title}}\n\nCulprit: {{.Steps.fetch_issue.culprit}}\nMessage: {{.Steps.fetch_issue.message}}",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "sentry_url", Required: true},
			{Name: "issue_id", Required: true},
			{Name: "repo_url", Required: true},
			{Name: "key_name", Required: true},
			{Name: "provider_key", Required: false},
		},
	},
	{
		Name:        "github-issue-fixer",
		Description: "Fetches a GitHub issue, creates a task to fix it, then creates a PR",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "fetch_issue",
				Type: StepTypeFetch,
				Config: mustJSON(FetchConfig{
					URL:     "https://api.github.com/repos/{{repoPath .Params.repo_url}}/issues/{{.Params.issue_number}}",
					Method:  "GET",
					KeyName: "{{.Params.key_name}}",
					Headers: map[string]string{
						"Accept": "application/vnd.github+json",
					},
					Outputs: map[string]string{
						"title":  "$.title",
						"body":   "$.body",
						"number": "$.number",
						"labels": "$.labels",
					},
				}),
			},
			{
				Name: "fix_issue",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:     "{{.Params.repo_url}}",
					Prompt:      "Fix the following GitHub issue (#{{.Steps.fetch_issue.number}}):\n\nTitle: {{.Steps.fetch_issue.title}}\n\n{{.Steps.fetch_issue.body}}\n\nAnalyze the codebase, understand the issue, and implement a fix.",
					ProviderKey: "{{.Params.provider_key}}",
				}),
			},
			{
				Name: "create_pr",
				Type: StepTypeAction,
				Config: mustJSON(ActionConfig{
					Kind:        ActionCreatePR,
					TaskStepRef: "fix_issue",
					Title:       "fix: {{.Steps.fetch_issue.title}} (#{{.Steps.fetch_issue.number}})",
					Description: "Closes #{{.Steps.fetch_issue.number}}\n\n{{.Steps.fetch_issue.title}}",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "issue_number", Required: true},
			{Name: "key_name", Required: true},
			{Name: "provider_key", Required: false},
		},
	},
	{
		Name:        "code-review",
		Description: "Executes a task then runs a code review on the changes in the same workspace",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "execute_task",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:      "{{.Params.repo_url}}",
					Prompt:       "{{.Params.prompt}}",
					ProviderKey:  "{{.Params.provider_key}}",
					AccessToken:  "{{.Params.access_token}}",
					CLI:          "{{.Params.cli}}",
					SourceBranch: "{{.Params.source_branch}}",
				}),
			},
			{
				Name: "code_review",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:          "{{.Params.repo_url}}",
					Prompt:           codeReviewPrompt,
					ProviderKey:      "{{.Params.provider_key}}",
					AccessToken:      "{{.Params.access_token}}",
					CLI:              "{{.Params.review_cli}}",
					WorkspaceTaskRef: "execute_task",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "prompt", Required: true},
			{Name: "provider_key", Required: false},
			{Name: "access_token", Required: false},
			{Name: "source_branch", Required: false},
			{Name: "cli", Required: false, Default: "claude-code"},
			{Name: "review_cli", Required: false, Default: "codex"},
		},
	},
}

// SeedBuiltins inserts or replaces built-in workflow definitions.
// This is idempotent and safe to call on every startup.
func SeedBuiltins(ctx context.Context, reg Registry) error {
	for _, def := range BuiltinWorkflows {
		err := reg.Create(ctx, def)
		if err != nil {
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && errors.Is(appErr, apperror.ErrConflict) {
				slog.Debug("built-in workflow already exists, skipping", "name", def.Name)
				continue
			}
			return err
		}
		slog.Info("seeded built-in workflow", "name", def.Name)
	}
	return nil
}

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
