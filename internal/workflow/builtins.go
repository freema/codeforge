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
		Description: "Analyze unresolved Sentry errors for a project, fix the ones fixable in code, and open a single PR with all fixes",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "fix_bugs",
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
					RepoURL: "{{.Params.repo_url}}",
					Prompt: `You are an automated bug-fixing agent fixing Sentry errors for a project.

## Sentry Project
- **Organization:** {{.Params.sentry_org}}
- **Project:** {{.Params.sentry_project}}
{{if .Params.max_issues}}- **Max issues to fix:** {{.Params.max_issues}}{{end}}

## Workflow
You have the Sentry MCP server connected. Discover the available Sentry tools and use them — do not assume exact tool names.

1. **Analyze first.** List the unresolved issues for organization "{{.Params.sentry_org}}" / project "{{.Params.sentry_project}}". Prioritize by occurrence count and severity (fatal > error > warning). {{if .Params.max_issues}}Pick the top {{.Params.max_issues}} most impactful issues.{{else}}Consider all promising issues.{{end}}
2. For each candidate, pull full details and the latest event (stack trace, breadcrumbs, context) via the Sentry tools.
3. Map the stack trace to the relevant code in THIS repository and decide whether it is fixable in code (skip infrastructure/network/external-service/transient errors).
4. For each fixable issue: implement a real, minimal fix and create a SEPARATE git commit with a message like "fix(sentry): <short description>".

## Rules
- Do NOT create placeholder or stub fixes, and do NOT add generic try/catch wrappers that merely hide errors.
- Only modify files directly related to each fix; one commit per fix so the PR is easy to review.
- NEVER commit the .mcp.json file (it is gitignored — do not force-add it).
- If an error is external/infrastructure, skip it and explain why.
- If nothing is fixable in code, make NO changes and explain why.

## Final summary (IMPORTANT)
End your response with a concise, human-readable Markdown summary suitable as a PR description:
- A short overview sentence.
- A bullet list: for each Sentry issue, what you changed and which files (or "skipped — reason").
Do not paste raw stack traces or error dumps into the summary.`,
					ProviderKey: "{{.Params.provider_key}}",
					ToolKeyRef:  "{{.Params.key_name}}",
					Tools: mustJSON([]tools.SessionTool{
						{Name: "sentry"},
					}),
					AutoCreatePR: true,
					PRTitle:      "{{.Params.pr_title}}",
					TargetBranch: "{{.Params.target_branch}}",
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
			{Name: "pr_title", Default: "fix: resolve Sentry errors"},
			{Name: "target_branch", Required: false},
		},
	},
}

// SeedBuiltins inserts or replaces built-in workflow definitions.
// It also removes stale built-in workflows (and their configs) that are no longer defined in code.
// This is idempotent and safe to call on every startup.
func SeedBuiltins(ctx context.Context, reg Registry, cfgStore *SQLiteConfigStore) error {
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
				// Remove orphaned configs referencing this workflow
				if cfgStore != nil {
					if n, err := cfgStore.DeleteByWorkflow(ctx, wf.Name); err != nil {
						slog.Warn("failed to remove configs for stale workflow", "workflow", wf.Name, "error", err)
					} else if n > 0 {
						slog.Info("removed orphaned workflow configs", "workflow", wf.Name, "count", n)
					}
				}
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
