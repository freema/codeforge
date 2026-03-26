package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/tools"
)

// BuiltinWorkflows defines the set of built-in workflow definitions.
var BuiltinWorkflows = []WorkflowDefinition{
	{
		Name:        "sentry-fixer",
		Description: "Analyze and fix unresolved Sentry errors in a project, then create a PR with all fixes",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "fix_bugs",
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
					RepoURL: "{{.Params.repo_url}}",
					Prompt: `You are fixing Sentry errors for a project.

## Sentry Project
- **Organization:** {{.Params.sentry_org}}
- **Project:** {{.Params.sentry_project}}
{{if .Params.max_issues}}- **Max issues to fix:** {{.Params.max_issues}}{{end}}

## Instructions
1. Use the Sentry MCP tools to list unresolved issues for this project:
   - Call list_sentry_issues with organization "{{.Params.sentry_org}}" and project "{{.Params.sentry_project}}" to get all unresolved errors
2. Prioritize issues by occurrence count and severity (fatal > error > warning)
3. {{if .Params.max_issues}}Process the top {{.Params.max_issues}} most important issues only.{{else}}Process all promising issues.{{end}} For each:
   a. Call get_sentry_issue to get full details (title, culprit, message)
   b. Call get_sentry_issue_events to get the latest event with stack trace, breadcrumbs, and context
   c. Analyze the stack trace — find the relevant code in this repository
   d. Determine if it's fixable in code (skip infrastructure/network/external service errors)
   e. If fixable: implement the fix and create a separate git commit with message "fix(sentry): <short description>"
4. After processing issues, summarize what you fixed and what you skipped (and why)

## Rules
- Do NOT create placeholder or stub fixes
- Do NOT add generic try/catch wrappers that hide errors
- If an error is from an external dependency or infrastructure, skip it and explain why
- Only modify files directly related to each fix
- Each fix should be a SEPARATE commit so the PR is easy to review
- If no issues are fixable in code, make NO changes and explain why`,
					ProviderKey: "{{.Params.provider_key}}",
					ToolKeyRef:  "{{.Params.key_name}}",
					Tools: mustJSON([]tools.TaskTool{
						{Name: "sentry"},
					}),
				}),
			},
			{
				Name: "create_pr",
				Type: StepTypeAction,
				Config: mustJSON(ActionConfig{
					Kind:        ActionCreatePR,
					TaskStepRef: "fix_bugs",
					Title:       "fix(sentry): automated error fixes for {{.Params.sentry_project}}",
					Description: "## Sentry Error Fixes\n\nAutomated fixes for unresolved errors in **{{.Params.sentry_org}}/{{.Params.sentry_project}}**.\n\nSee individual commits for details on each fix.\n\n---\n*Automated by CodeForge sentry-fixer workflow.*",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "sentry_org", Required: true},
			{Name: "sentry_project", Required: true},
			{Name: "repo_url", Required: true},
			{Name: "key_name", Required: true},
			{Name: "provider_key", Required: false},
			{Name: "max_issues", Default: "5"},
		},
	},
}

// SeedBuiltins inserts or replaces built-in workflow definitions.
// It also removes stale built-in workflows that are no longer defined in code.
// This is idempotent and safe to call on every startup.
func SeedBuiltins(ctx context.Context, reg Registry) error {
	// Build set of current builtin names
	currentNames := make(map[string]bool, len(BuiltinWorkflows))
	for _, def := range BuiltinWorkflows {
		currentNames[def.Name] = true
	}

	// Remove stale builtins that are no longer in code
	existing, err := reg.List(ctx)
	if err == nil {
		for _, wf := range existing {
			if wf.Builtin && !currentNames[wf.Name] {
				if err := reg.DeleteBuiltin(ctx, wf.Name); err != nil {
					slog.Warn("failed to remove stale builtin workflow", "name", wf.Name, "error", err)
				} else {
					slog.Info("removed stale built-in workflow", "name", wf.Name)
				}
			}
		}
	}

	// Seed current builtins (upsert: create new, update existing)
	for _, def := range BuiltinWorkflows {
		err := reg.Create(ctx, def)
		if err != nil {
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && errors.Is(appErr, apperror.ErrConflict) {
				// Already exists — update it so code changes propagate
				if updateErr := reg.UpdateBuiltin(ctx, def); updateErr != nil {
					slog.Warn("failed to update built-in workflow", "name", def.Name, "error", updateErr)
				} else {
					slog.Info("updated built-in workflow", "name", def.Name)
				}
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
