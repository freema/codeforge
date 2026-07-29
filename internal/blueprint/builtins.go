package blueprint

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/tools"
)

// sentryFixerPrompt is the prompt template for the sentry-fixer builtin,
// ported verbatim from the legacy workflow definition.
const sentryFixerPrompt = `You are an automated bug-fixing agent fixing Sentry errors for a project.

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
Do not paste raw stack traces or error dumps into the summary.`

// BuiltinBlueprints defines the set of built-in blueprints.
var BuiltinBlueprints = []Blueprint{
	{
		Name:        "sentry-fixer",
		Description: "Analyze unresolved Sentry errors for a project, fix the ones fixable in code, and open a single PR with all fixes",
		Builtin:     true,
		Request: mustJSON(map[string]any{
			"repo_url":     "{{.Params.repo_url}}",
			"prompt":       sentryFixerPrompt,
			"provider_key": "{{.Params.provider_key}}",
			"tool_key_ref": "{{.Params.key_name}}",
			"config": map[string]any{
				"tools": []tools.SessionTool{
					{Name: "sentry"},
				},
				"auto_create_pr": true,
				"pr_title":       "{{.Params.pr_title}}",
				"target_branch":  "{{.Params.target_branch}}",
			},
		}),
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
	{
		Name:        "knowledge-update",
		Description: "Analyze the repo and refresh .codeforge/ knowledge docs, opening a PR with the changes",
		Builtin:     true,
		Request: mustJSON(map[string]any{
			"repo_url":     "{{.Params.repo_url}}",
			"provider_key": "{{.Params.provider_key}}",
			"session_type": "knowledge",
			"prompt":       "{{.Params.focus}}",
			"config": map[string]any{
				"auto_create_pr": true,
				"pr_title":       "docs: update .codeforge knowledge",
			},
		}),
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "provider_key", Required: false},
			{Name: "focus", Required: false},
		},
	},
	{
		Name:        "repo-review",
		Description: "Run a full repository review (quality, security, architecture)",
		Builtin:     true,
		Request: mustJSON(map[string]any{
			"repo_url":     "{{.Params.repo_url}}",
			"provider_key": "{{.Params.provider_key}}",
			"session_type": "review",
			"prompt":       "{{.Params.focus}}",
		}),
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "provider_key", Required: false},
			{Name: "focus", Required: false},
		},
	},
}

// SeedBuiltins inserts or updates built-in blueprints and removes stale
// builtin rows whose name is no longer defined in code. It never clobbers
// non-builtin user rows. Idempotent and safe to call on every startup.
func SeedBuiltins(ctx context.Context, store *Store) error {
	// Build set of current builtin names.
	currentNames := make(map[string]bool, len(BuiltinBlueprints))
	for i := range BuiltinBlueprints {
		currentNames[BuiltinBlueprints[i].Name] = true
	}

	// Remove stale builtins that are no longer in code.
	existing, err := store.List(ctx)
	if err == nil {
		for _, b := range existing {
			if b.Builtin && !currentNames[b.Name] {
				if err := store.DeleteBuiltinByName(ctx, b.Name); err != nil {
					slog.Warn("failed to remove stale builtin blueprint", "name", b.Name, "error", err)
				} else {
					slog.Info("removed stale built-in blueprint", "name", b.Name)
				}
			}
		}
	}

	// Seed current builtins (upsert: create new, update existing builtin rows).
	for i := range BuiltinBlueprints {
		bp := BuiltinBlueprints[i] // copy — Create mutates ID/timestamps
		err := store.Create(ctx, &bp)
		if err == nil {
			slog.Info("seeded built-in blueprint", "name", bp.Name)
			continue
		}
		if !errors.Is(err, ErrNameTaken) {
			return err
		}
		// Name already exists — update it in place if it is a builtin row so
		// code changes propagate; a user's non-builtin blueprint holding the
		// name is left untouched.
		if err := store.UpdateBuiltinByName(ctx, &bp); err != nil {
			slog.Warn("failed to update built-in blueprint", "name", bp.Name, "error", err)
		}
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
