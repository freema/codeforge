package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/tools"
)

// AnalyzeRepoPrompt is the prompt template for analyzing a repository's structure.
// Exported for reuse by the CI Action's knowledge_update task type.
const AnalyzeRepoPrompt = `You are analyzing a codebase to understand its architecture, conventions, and key components.

{{if .Params.focus}}Focus area: {{.Params.focus}}{{end}}

## Instructions

1. Explore the repository structure thoroughly
2. Read key files: README, configs, entry points, core modules
3. Identify: architecture patterns, tech stack, coding conventions, important abstractions
4. Summarize your findings — this will be used to generate documentation

## Rules

- Do NOT modify any files
- Be thorough but concise in your analysis
- Focus on information that would help a new developer understand the codebase`

// UpdateKnowledgePrompt is the prompt template for creating/updating .codeforge/ knowledge docs.
// Exported for reuse by the CI Action's knowledge_update task type.
const UpdateKnowledgePrompt = `You are a technical writer creating project knowledge documentation.

Based on your analysis of this codebase, create or update documentation files in the ` + "`.codeforge/`" + ` directory.

{{if .Params.focus}}Focus area: {{.Params.focus}}{{end}}

## Files to create/update

Create these files in ` + "`.codeforge/`" + ` directory at the project root:

### ` + "`.codeforge/OVERVIEW.md`" + `
- Project name and purpose (1-2 sentences)
- Tech stack summary
- How to run/build/test
- Key entry points

### ` + "`.codeforge/ARCHITECTURE.md`" + `
- High-level system design
- Directory structure with descriptions
- Key abstractions and their relationships
- Data flow (request lifecycle, etc.)

### ` + "`.codeforge/CONVENTIONS.md`" + `
- Coding patterns and style
- Error handling approach
- Testing patterns
- Naming conventions
- Configuration patterns

## Rules

- If ` + "`.codeforge/`" + ` files already exist, UPDATE them — don't overwrite blindly, preserve accurate existing content
- Be concise — each file should be scannable, not a novel
- Focus on STABLE knowledge (architecture, patterns) not volatile details (specific line numbers)
- Use markdown with clear headers
- If the repo already has good docs (README, CONTRIBUTING), reference them rather than duplicating`

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
	{
		Name:        "github-issue-fixer",
		Description: "Fetches a GitHub issue, creates a session to fix it, then creates a PR",
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
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
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
		Name:        "gitlab-issue-fixer",
		Description: "Fetches a GitLab issue, creates a session to fix it, then creates a merge request",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "fetch_issue",
				Type: StepTypeFetch,
				Config: mustJSON(FetchConfig{
					URL:     "{{repoHost .Params.repo_url}}/api/v4/projects/{{urlEncode (repoPath .Params.repo_url)}}/issues/{{.Params.issue_iid}}",
					Method:  "GET",
					KeyName: "{{.Params.key_name}}",
					Outputs: map[string]string{
						"title":       "$.title",
						"description": "$.description",
						"iid":         "$.iid",
						"labels":      "$.labels",
					},
				}),
			},
			{
				Name: "fix_issue",
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
					RepoURL:     "{{.Params.repo_url}}",
					Prompt:      "Fix the following GitLab issue (!{{.Steps.fetch_issue.iid}}):\n\nTitle: {{.Steps.fetch_issue.title}}\n\n{{.Steps.fetch_issue.description}}\n\nAnalyze the codebase, understand the issue, and implement a fix.",
					ProviderKey: "{{.Params.provider_key}}",
				}),
			},
			{
				Name: "create_mr",
				Type: StepTypeAction,
				Config: mustJSON(ActionConfig{
					Kind:        ActionCreatePR,
					TaskStepRef: "fix_issue",
					Title:       "fix: {{.Steps.fetch_issue.title}} (#{{.Steps.fetch_issue.iid}})",
					Description: "Closes #{{.Steps.fetch_issue.iid}}\n\n{{.Steps.fetch_issue.title}}",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "issue_iid", Required: true},
			{Name: "key_name", Required: true},
			{Name: "provider_key", Required: false},
		},
	},
	{
		Name:        "knowledge-update",
		Description: "Analyze a repository and create/update .codeforge/ knowledge documentation via PR",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "analyze_repo",
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
					RepoURL:     "{{.Params.repo_url}}",
					Prompt:      AnalyzeRepoPrompt,
					SessionType:    "plan",
					ProviderKey: "{{.Params.provider_key}}",
					CLI:         "{{.Params.cli}}",
				}),
			},
			{
				Name: "update_knowledge",
				Type: StepTypeSession,
				Config: mustJSON(SessionStepConfig{
					RepoURL:          "{{.Params.repo_url}}",
					Prompt:           UpdateKnowledgePrompt,
					SessionType:         "code",
					ProviderKey:      "{{.Params.provider_key}}",
					CLI:              "{{.Params.cli}}",
					WorkspaceTaskRef: "analyze_repo",
				}),
			},
			{
				Name: "create_pr",
				Type: StepTypeAction,
				Config: mustJSON(ActionConfig{
					Kind:        ActionCreatePR,
					TaskStepRef: "update_knowledge",
					Title:       "docs: update .codeforge/ knowledge base",
					Description: "Automated knowledge update generated by CodeForge.\n\nAnalyzed repository structure, architecture, and conventions to create/update `.codeforge/` documentation files.",
				}),
			},
		},
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "focus"},
			{Name: "provider_key", Required: true},
			{Name: "cli"},
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
