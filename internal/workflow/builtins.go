package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/tools"
)

// codeReviewPrompt is pre-rendered at init time. The prompt template's
// {{.OriginalPrompt}} is filled with a workflow template expression so
// the actual task prompt gets substituted at workflow runtime.
var codeReviewPrompt = mustRenderCodeReviewPrompt()

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
					RepoURL: "{{.Params.repo_url}}",
					Prompt: `Fix the following Sentry error in this codebase.

## Error Details
- **Error:** {{.Steps.fetch_issue.title}}
- **Location:** {{.Steps.fetch_issue.culprit}}
- **Message:** {{.Steps.fetch_issue.message}}
- **Platform:** {{.Steps.fetch_issue.platform}}

## Instructions
1. Use the Sentry MCP tool to fetch more context about this error (stack traces, breadcrumbs, latest events)
2. Find the code referenced in the culprit path
3. Analyze what causes this error — read the relevant code and understand the context
4. Determine if this is fixable in code (some errors are infrastructure/network issues that cannot be fixed in code)
5. If fixable: implement a proper fix with error handling
6. If NOT fixable in code (e.g., external service timeouts, network errors): do NOT create any files or changes — just explain why in your output

## Rules
- Do NOT create placeholder or stub fixes
- Do NOT add generic try/catch wrappers that hide the error
- If the error is from an external dependency or infrastructure, say so and make NO changes
- Only modify files that are directly related to the fix`,
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
					TaskStepRef: "fix_bug",
					Title:       "fix(sentry): {{.Steps.fetch_issue.culprit}}",
					Description: "## Sentry Error Fix\n\n**Error:** {{.Steps.fetch_issue.title}}\n**Location:** {{.Steps.fetch_issue.culprit}}\n**Message:** {{.Steps.fetch_issue.message}}\n\n---\n*Automated fix by CodeForge sentry-fixer workflow.*",
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
		Name:        "gitlab-issue-fixer",
		Description: "Fetches a GitLab issue, creates a task to fix it, then creates a merge request",
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
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
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
	{
		Name:        "knowledge-update",
		Description: "Analyze a repository and create/update .codeforge/ knowledge documentation via PR",
		Builtin:     true,
		Steps: []StepDefinition{
			{
				Name: "analyze_repo",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:     "{{.Params.repo_url}}",
					Prompt:      AnalyzeRepoPrompt,
					TaskType:    "plan",
					ProviderKey: "{{.Params.provider_key}}",
					CLI:         "{{.Params.cli}}",
				}),
			},
			{
				Name: "update_knowledge",
				Type: StepTypeTask,
				Config: mustJSON(TaskStepConfig{
					RepoURL:          "{{.Params.repo_url}}",
					Prompt:           UpdateKnowledgePrompt,
					TaskType:         "code",
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

	// Seed current builtins
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
