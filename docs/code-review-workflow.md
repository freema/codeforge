# Code Review

CodeForge supports code review as a **user-triggered action** on completed tasks. The user decides when to review, which CLI to use, and what to do with the results.

## How It Works

Code review is an action the user triggers via `POST /tasks/:id/review` — it is NOT automatic. The task must be in `completed` or `awaiting_instruction` status.

```
1. POST /tasks         → pending → cloning → running → completed
2. POST /tasks/:id/review  → reviewing → completed (with ReviewResult)
3. (user reads review, decides next step)
4. POST /tasks/:id/instruct  → fix issues   (optional)
5. POST /tasks/:id/create-pr → create PR    (optional)
```

Steps 2-4 are optional and repeatable. The user can:
- Skip review entirely and go straight to PR
- Review multiple times
- Instruct the AI to fix review issues
- Do their own fixes and re-review

## API Usage

### Start a Review

```bash
curl -X POST http://localhost:8080/api/v1/tasks/$TASK_ID/review \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "cli": "codex",
    "model": "o3"
  }'
```

Both `cli` and `model` are optional. Defaults:
- `cli` — task's original CLI, or server default (`claude-code`)
- `model` — configured default model for the selected CLI

**Response (200):**
```json
{
  "verdict": "request_changes",
  "score": 6,
  "summary": "Implementation looks good overall but has two issues that should be fixed.",
  "issues": [
    {
      "severity": "major",
      "file": "src/handlers/users.go",
      "line": 42,
      "description": "Missing error check on json.Unmarshal",
      "suggestion": "Add if err != nil { return err } after unmarshal"
    },
    {
      "severity": "minor",
      "file": "src/handlers/users.go",
      "line": 55,
      "description": "Consider using a constant for max name length"
    }
  ],
  "auto_fixable": true,
  "reviewed_by": "codex:o3",
  "duration_seconds": 12.5
}
```

### State Transitions

| Current Status | After `POST /review` | After Review Completes |
|---|---|---|
| `completed` | `reviewing` | `completed` (with `review_result`) |
| `awaiting_instruction` | `reviewing` | `completed` (with `review_result`) |
| `running` / `cloning` / `failed` | **409 Conflict** | — |

### ReviewResult on Task

After review, the `ReviewResult` is stored on the task and returned in `GET /tasks/:id`:

```json
{
  "id": "task-abc",
  "status": "completed",
  "result": "...",
  "review_result": {
    "verdict": "approve",
    "score": 9,
    "summary": "Clean implementation, no issues found.",
    "issues": [],
    "auto_fixable": false,
    "reviewed_by": "codex:o3",
    "duration_seconds": 8.2
  }
}
```

## Review Output Format

### Verdicts
- **approve** — no issues or only minor suggestions
- **request_changes** — has major issues that should be fixed
- **comment** — informational review, no strong opinion

### Issue Severities
- **critical** — must fix before merging
- **major** — should fix, significant issue
- **minor** — nice to fix, not blocking
- **suggestion** — style or improvement idea

### Score
1-10 scale where 10 is perfect code.

## Workflow-Based Review (Alternative)

CodeForge also has a built-in `code-review` workflow that chains task execution + review in a single workflow run:

```bash
curl -X POST http://localhost:8080/api/v1/workflows/code-review/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "params": {
      "repo_url": "https://github.com/owner/repo.git",
      "prompt": "Add input validation",
      "cli": "claude-code",
      "review_cli": "codex"
    }
  }'
```

This creates two tasks sequentially — the review task reuses the first task's workspace via `WorkspaceTaskID`. The workflow approach is useful for automation, while the direct `POST /review` endpoint is better for interactive use.

## Internal Architecture

### Review Package (`internal/review/`)

```
internal/review/
  reviewer.go    # Reviewer service + TaskProvider interface
  model.go       # ReviewResult, ReviewIssue, Verdict types
  parser.go      # Multi-strategy output parser
```

### Key Types

```go
// Reviewer orchestrates the review process
type Reviewer struct {
    taskProvider TaskProvider
    cliRegistry  *runner.Registry
    emitter      EventEmitter
    cfg          ReviewerConfig
}

// TaskProvider abstracts task service + workspace
type TaskProvider interface {
    StartReview(ctx context.Context, taskID string) error
    CompleteReview(ctx context.Context, taskID string, result *ReviewResult) error
    GetTask(ctx context.Context, taskID string) (TaskInfo, error)
}

// ReviewResult is stored on Task.ReviewResult
type ReviewResult struct {
    Verdict         Verdict       `json:"verdict"`
    Score           int           `json:"score"`
    Summary         string        `json:"summary"`
    Issues          []ReviewIssue `json:"issues"`
    AutoFixable     bool          `json:"auto_fixable"`
    ReviewedBy      string        `json:"reviewed_by"`
    DurationSeconds float64       `json:"duration_seconds"`
}
```

### Parser Strategies

The `ParseReviewOutput()` function tries 4 strategies in order:
1. Direct JSON unmarshal of the entire output
2. Extract JSON from markdown `` ```json ... ``` `` code block
3. Heuristic brace matching for JSON object containing "verdict"
4. Fallback: `VerdictComment` with truncated summary from raw text

### Prompt Template (`internal/prompt/`)

The code review prompt template (`internal/prompt/templates/code_review.md`) instructs the review CLI to:
- Run `git diff HEAD~1` to see changes
- Analyze code quality, security, performance
- Output structured JSON with verdict, score, issues

Templates are embedded via `//go:embed templates/*.md` and rendered with Go `text/template`.
