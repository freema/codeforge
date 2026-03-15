# CodeForge CI Action — Implementation Plan (Variant 2: Self-Contained)

## Goal

Make CodeForge run as a self-contained GitHub Action / GitLab CI step — like `claude-code-action` but better (multi-CLI, tools, GitLab support, shared codebase with server).

## Architecture

```
cmd/codeforge-action/        <- new entry point (single-shot binary)
  main.go                    <- CLI flags + orchestration
  ci_executor.go             <- simplified executor (no Redis/webhooks)
  github_output.go           <- GitHub Actions output ($GITHUB_OUTPUT, comments)
  gitlab_output.go           <- GitLab CI output (MR notes)

action.yml                   <- GitHub Action definition
.gitlab-ci-template.yml      <- GitLab CI template
deployments/Dockerfile.action <- CI-optimized image
```

## Reusable Components (~60% of codebase)

| Component | Path | Reusability | Dependencies |
|---|---|---|---|
| CLI runners (Claude, Codex) | `internal/tool/runner/` | Full | stdlib only |
| Git ops (clone, diff, PR, review posting) | `internal/tool/git/` | Full | stdlib only |
| Review parser & formatter | `internal/review/` | Full | stdlib only |
| Prompt templates | `internal/prompt/` | Full | embed FS only |
| MCP config writer | `internal/tool/mcp/installer.go` | WriteMCPConfig() | stdlib only |
| Task types | `internal/task/model.go` | Types only | stdlib only |
| Executor logic | `internal/worker/executor.go` | ~80% adaptable | replace streaming |

## Server vs CI Mode

| Server Mode | CI Mode |
|---|---|
| Redis queue + BLPOP | direct execution, no queue |
| Redis state hashes | in-memory state |
| Redis Pub/Sub streaming | stdout + CI runner logs |
| SQLite persistence | no DB |
| HTTP API + SSE | exit code + output files |
| Worker pool | single execution |
| Multi-task | single task, single run |

## action.yml Inputs

```yaml
name: 'CodeForge'
description: 'AI-powered code review & tasks for GitHub and GitLab'
inputs:
  task_type:        # code_review, pr_review, plan, custom
  prompt:           # user prompt (or auto-detect from PR)
  cli:              # claude-code, codex (default: claude-code)
  model:            # AI model override
  api_key:          # Anthropic/OpenAI API key
  provider_token:   # GitHub/GitLab token for PR operations
  mcp_config:       # JSON string or path to .mcp.json
  post_comments:    # true/false - post review as PR comments
  output_format:    # json, markdown, text
  max_turns:        # max conversation turns
  allowed_tools:    # comma-separated tool allowlist
runs:
  using: 'docker'
  image: 'docker://ghcr.io/freema/codeforge-action:latest'
```

## Execution Flow

```
1. Detect context
   - GitHub: parse $GITHUB_EVENT_PATH JSON -> repo URL, PR number, branch, base, diff
   - GitLab: parse CI env vars ($CI_MERGE_REQUEST_IID, $CI_PROJECT_URL, etc.)

2. Setup workspace
   - CI runner already clones repo ($GITHUB_WORKSPACE / $CI_PROJECT_DIR)
   - No clone needed — use existing checkout

3. Write .mcp.json (if mcp_config provided)

4. Build prompt
   - pr_review: PR diff + review template
   - code_review: branch diff + review template
   - custom: user prompt as-is

5. Run CLI (Claude/Codex)
   - Stream to stdout (CI logs)
   - Capture result

6. Parse output
   - Review: ParseReviewOutput() -> ReviewResult
   - Task: raw result

7. Post results
   - GitHub: PR review comments via API, summary comment, $GITHUB_OUTPUT
   - GitLab: MR notes via API, $CI_JOB_OUTPUT

8. Exit
   - exit 0: success / approve
   - exit 1: request_changes or error
```

## Competitive Advantage vs claude-code-action

| Feature | claude-code-action | CodeForge Action |
|---|---|---|
| CLI support | Claude Code only | Claude Code + Codex (+ future) |
| GitLab | No | Yes, native |
| Review posting | custom implementation | shared with server |
| Prompt templates | none | embedded templates (review, plan) |
| MCP support | Yes | Yes + config from file |
| Runtime | Bun/Node.js | Go binary (faster start, smaller image) |
| Server mode | No | Yes, same codebase |

## Docker Image

Lightweight image (~130 MB) — CLI is installed at runtime based on user's `cli` input. No prebaked CLI tools.

```dockerfile
# Build stage - shared with server
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -o /codeforge-action ./cmd/codeforge-action

# Runtime - minimal, no Redis/SQLite/CLI prebaked
FROM alpine:3.20
RUN apk add --no-cache git nodejs npm ca-certificates
COPY --from=builder /codeforge-action /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/codeforge-action"]
```

### Runtime CLI Installation

The binary installs the selected CLI on startup (~30s, negligible vs 2-5 min AI execution):

```go
func ensureCLI(cli string) error {
    if _, err := exec.LookPath(cli); err == nil {
        return nil // already available (e.g., prebaked or cached)
    }
    switch cli {
    case "claude-code":
        return exec.Command("npm", "install", "-g", "@anthropic-ai/claude-code").Run()
    case "codex":
        return exec.Command("npm", "install", "-g", "@openai/codex").Run()
    }
    return fmt.Errorf("unknown CLI: %s", cli)
}
```

### Image Size Breakdown

| Component | Size |
|---|---|
| Alpine base | ~5 MB |
| git + ca-certificates | ~30 MB |
| Node.js (Alpine) | ~50 MB |
| npm | ~30 MB |
| Go binary (static) | ~20 MB |
| **Total image** | **~130 MB** |
| CLI installed at runtime | +200-300 MB (ephemeral, not in image) |

Published to `ghcr.io/freema/codeforge-action` alongside `ghcr.io/freema/codeforge`.

## Implementation Steps

### Step 1: Entry point + CI executor
- `cmd/codeforge-action/main.go` — CLI flags (cobra or bare flags)
- `cmd/codeforge-action/ci_executor.go` — simplified executor reusing `runner.Run()`, `git.Clone()`, `review.Parse()`

### Step 2: Context parsers
- `cmd/codeforge-action/github_context.go` — parse `$GITHUB_EVENT_PATH` JSON
- `cmd/codeforge-action/gitlab_context.go` — parse GitLab CI env vars

### Step 3: Output formatters
- `cmd/codeforge-action/github_output.go` — PR comments, `$GITHUB_OUTPUT`, annotations
- `cmd/codeforge-action/gitlab_output.go` — MR notes, CI output

### Step 4: Action definitions
- `action.yml` — GitHub Action definition
- `.gitlab-ci-template.yml` — GitLab CI template

### Step 5: Docker
- `deployments/Dockerfile.action` — multi-stage build, minimal image

### Step 6: Tests
- Unit tests for context parsing
- Integration test with mock CLI

## Scope Estimate

- **New code:** ~800-1200 lines
- **Reused code:** ~3000+ lines (runner, git, review, prompt - no changes)
- **New files:** 6-8
- **Changes to existing code:** minimal (possibly extract helper functions from executor.go)

## Files to Create

| File | Description |
|---|---|
| `cmd/codeforge-action/main.go` | Entry point, CLI flags, orchestration |
| `cmd/codeforge-action/ci_executor.go` | Simplified executor (no Redis/webhooks) |
| `cmd/codeforge-action/github_context.go` | GitHub event parser |
| `cmd/codeforge-action/gitlab_context.go` | GitLab CI env parser |
| `cmd/codeforge-action/github_output.go` | GitHub output (comments, $GITHUB_OUTPUT) |
| `cmd/codeforge-action/gitlab_output.go` | GitLab output (MR notes) |
| `action.yml` | GitHub Action definition |
| `deployments/Dockerfile.action` | CI-optimized Docker image |

## Files to Possibly Modify

| File | Change |
|---|---|
| `internal/worker/executor.go` | Extract reusable helpers (optional) |
| `.github/workflows/` | CI pipeline for building action image |
| `Taskfile.yaml` | Add `build:action` target |

---

## Knowledge System — Self-Updating Project Context

### Killer Feature: Automatic .codeforge/ Knowledge Base

CodeForge already has a `knowledge-update` workflow (server mode) that generates project documentation. The CI Action reuses this as a dedicated `task_type`, creating a **self-improving review loop**.

### How It Works

The action binary supports multiple task types via a single `task_type` input:

```go
switch cfg.TaskType {
case "pr_review":
    // 1. Read .codeforge/*.md → inject into system prompt
    // 2. Run CLI with review prompt + project context
    // 3. Parse output, post PR comments

case "knowledge_update":
    // 1. Run CLI with analyze + update prompts
    //    (reuse prompts from workflow/builtins.go)
    // 2. git add .codeforge/
    // 3. git commit + push branch
    // 4. Create PR via GitHub/GitLab API

case "custom":
    // User prompt as-is
}
```

### Knowledge Files Generated

```
.codeforge/
  OVERVIEW.md       ← project purpose, tech stack, build/test commands
  ARCHITECTURE.md   ← system design, directory structure, data flow
  CONVENTIONS.md    ← coding patterns, error handling, naming conventions
```

Prompts reused from `internal/workflow/builtins.go` (`analyzeRepoPrompt` + `updateKnowledgePrompt`).

### Knowledge Injection in PR Review

Before running CLI for review, the action reads `.codeforge/` files and injects them into system prompt:

```go
func (e *CIExecutor) buildSystemContext(workDir string) string {
    var ctx strings.Builder

    // Read .codeforge/ knowledge files (if they exist)
    for _, f := range []string{"OVERVIEW.md", "ARCHITECTURE.md", "CONVENTIONS.md"} {
        path := filepath.Join(workDir, ".codeforge", f)
        if data, err := os.ReadFile(path); err == nil {
            ctx.WriteString(string(data))
            ctx.WriteString("\n\n")
        }
    }

    // Read CLAUDE.md (if it exists — customer's own instructions)
    if data, err := os.ReadFile(filepath.Join(workDir, "CLAUDE.md")); err == nil {
        ctx.WriteString(string(data))
    }

    return ctx.String()
}
```

Passed to CLI via `--append-system-prompt`.

### Self-Improving Loop

```
Developer merges PR to main
        │
        ▼
knowledge_update workflow runs (on: push to main)
        │
        ▼
AI analyzes current repo state
        │
        ▼
Updates .codeforge/OVERVIEW.md
        .codeforge/ARCHITECTURE.md
        .codeforge/CONVENTIONS.md
        │
        ▼
Creates PR "docs: update knowledge base"
        │
        ▼
Team merges (or auto-merge)
        │
        ▼
Next PR review reads fresh .codeforge/ context
        │
        ▼
Review is more accurate — knows new architecture
        │
        ▼
... cycle repeats
```

### Customer Workflow Setup (Two YAML Files)

**Workflow 1: PR Review (on every PR)**

```yaml
name: Code Review
on: pull_request

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: freema/codeforge-action@v1
        with:
          task_type: pr_review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

**Workflow 2: Knowledge Update (on merge to main)**

```yaml
name: Update Knowledge
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  knowledge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: freema/codeforge-action@v1
        with:
          task_type: knowledge_update
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

**Optional: Weekly refresh (for fast-changing repos)**

```yaml
on:
  schedule:
    - cron: '0 3 * * 1'  # Monday 3:00 AM
  push:
    branches: [main]
```

### Competitive Advantage

| Feature | claude-code-action | CodeForge Action |
|---|---|---|
| Project context | Reads CLAUDE.md if manually written | Auto-generates .codeforge/ knowledge base |
| Context freshness | Static (manual updates) | Self-updating on every merge |
| Architecture awareness | None (unless in CLAUDE.md) | Full (ARCHITECTURE.md auto-generated) |
| Convention awareness | None | Full (CONVENTIONS.md auto-generated) |

**Nobody else has self-updating project knowledge for AI code review.**

---

## Token & Auth Analysis

### Required secrets (user must configure)

Only **one secret** is needed:

| CLI | Required Secret |
|---|---|
| Claude Code | `ANTHROPIC_API_KEY` |
| Codex | `OPENAI_API_KEY` |

### Automatic tokens (no user setup)

| Platform | Token | How |
|---|---|---|
| GitHub Actions | `$GITHUB_TOKEN` | Automatic per-job, set by Actions runtime |
| GitLab CI | `$CI_JOB_TOKEN` | Automatic per-job, set by GitLab runner |

These tokens are sufficient for reading repo content and posting PR/MR review comments (with correct permissions).

### GitHub Actions permissions

```yaml
permissions:
  contents: read       # read code
  pull-requests: write # post review comments
```

No PAT, no OAuth, no app installation needed.

### Minimal workflow (GitHub Actions)

```yaml
name: Code Review
on:
  pull_request:

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: freema/codeforge-action@v1
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

**12 lines YAML, 1 secret.** Same simplicity as `claude-code-action`, but with multi-CLI and GitLab support.

### Minimal workflow (GitLab CI)

```yaml
code-review:
  stage: review
  image: ghcr.io/freema/codeforge-action:latest
  variables:
    CODEFORGE_CLI: claude-code
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

`$ANTHROPIC_API_KEY` set as GitLab CI/CD variable. `$CI_JOB_TOKEN` is automatic.

### Advanced: Custom provider token

If the automatic token doesn't have sufficient permissions (e.g., cross-repo access), user can override:

```yaml
- uses: freema/codeforge-action@v1
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
    GITHUB_TOKEN: ${{ secrets.CUSTOM_PAT }}  # override automatic token
```

### How the binary resolves tokens

```
1. AI API key:
   - $ANTHROPIC_API_KEY (for Claude)
   - $OPENAI_API_KEY (for Codex)
   - Error if missing for selected CLI

2. Provider token (for PR comments):
   - GitHub: $GITHUB_TOKEN (automatic) or input override
   - GitLab: $CI_JOB_TOKEN (automatic) or $GITLAB_TOKEN override

3. MCP server tokens:
   - Passed via mcp_config input JSON (env field)
   - e.g., SENTRY_AUTH_TOKEN for Sentry MCP
```

### PR Review Output Example

**Summary comment** (sticky, updates on re-push):

> ### CodeForge Review — Score: 7/10
>
> **Verdict:** Request Changes
>
> Overall solid implementation. The event dispatching pattern is clean.
> However, there are concerns about error handling in the async pipeline.
>
> | Severity | Count |
> |----------|-------|
> | Critical | 1 |
> | Warning  | 2 |
> | Info     | 1 |
>
> *Reviewed by Claude Code (claude-sonnet-4-20250514) via CodeForge*

**Inline comment** on specific line:

> **`src/notifications/dispatcher.go:47`** — `critical`
>
> This goroutine swallows the error from `Send()`. If the notification
> provider fails, the user will never know.
> ```go
> // suggestion:
> if err := n.provider.Send(ctx, msg); err != nil {
>     n.logger.Error("send failed", "err", err)
>     return err
> }
> ```
