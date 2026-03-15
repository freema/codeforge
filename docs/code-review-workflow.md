# Code Review

CodeForge supports three kinds of code review:

1. **Task review** (`POST /tasks/:id/review`) — user-triggered review of a specific task's changes
2. **PR review** (`task_type: "pr_review"`) — automated review of a pull request / merge request diff
3. **Webhook review** — GitHub/GitLab webhooks auto-create PR review tasks on PR open/update

All three store a structured `ReviewResult` on the task and optionally post comments to the PR/MR.

## 1. Task Review (User-Triggered)

CodeForge supports code review as a **user-triggered action** on completed tasks. The user decides when to review, which CLI to use, and what to do with the results.

## How It Works

Code review is an action the user triggers via `POST /tasks/:id/review` — it is NOT automatic. The task must be in `completed` or `awaiting_instruction` status. The review is **enqueued for async worker execution** (returns 202 immediately) — the same queue/worker model as regular tasks. Monitor progress via SSE stream. See [Task Session Lifecycle](architecture.md#task-session-lifecycle) for the full flow.

```
1. POST /tasks                → pending → cloning → running → completed
2. POST /tasks/:id/review     → 202 Accepted, reviewing (async via worker pool)
3. GET  /tasks/:id/stream     → SSE: review_started, cli output, review_completed
4. GET  /tasks/:id            → completed (with review_result)
5. POST /tasks/:id/instruct   → fix issues   (optional)
6. POST /tasks/:id/create-pr  → create PR    (optional)
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

**Response (202 Accepted):**
```json
{
  "id": "task-abc",
  "status": "reviewing"
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
  model.go       # ReviewResult, ReviewIssue, Verdict types
  parser.go      # Multi-strategy output parser
  format.go      # PR/MR comment formatting (summary body, issue comments)
```

The review package provides types and parsing — execution is handled by the worker executor (`executeReview` for `/review` endpoint, `handlePRReviewCompletion` for task_type=review/pr_review).

### Key Types

```go
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

## 2. PR Review (Automated)

The `pr_review` task type creates a review task for a specific pull request / merge request. It is triggered via the standard `POST /api/v1/tasks` endpoint or automatically via webhooks.

### How It Works

```
1. POST /api/v1/tasks {task_type: "pr_review", config: {pr_number: 42, ...}}
2. pending → cloning (target branch, non-shallow)
3. git fetch origin pull/{N}/head:pr-{N} → checkout pr-{N}
4. running (AI CLI reviews diff via git diff origin/{base}...HEAD)
5. completed (ReviewResult stored on task)
6. (if output_mode: "post_comments") → comments posted to PR/MR
```

### Fork PR Handling

Fork PRs are handled automatically. The executor:
1. Clones the **target branch** (not the source branch, which may not exist on origin)
2. Fetches the PR ref via `git fetch origin pull/{N}/head:pr-{N}`
3. Checks out the local `pr-{N}` branch

This works for both same-repo and cross-fork PRs on GitHub. GitLab uses merge request refs similarly.

### API Usage

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "Review PR #42",
    "task_type": "pr_review",
    "config": {
      "pr_number": 42,
      "source_branch": "feature/login",
      "target_branch": "main",
      "output_mode": "api_only"
    }
  }'
```

| `output_mode` | Behavior |
|---------------|----------|
| `api_only` (default) | ReviewResult stored on task, available via `GET /tasks/{id}` |
| `post_comments` | ReviewResult posted as PR/MR comments automatically on completion |

### Post Comments Manually

```bash
# Post an existing ReviewResult to the PR
curl -X POST http://localhost:8080/api/v1/tasks/$TASK_ID/post-review \
  -H "Authorization: Bearer $TOKEN"
```

### Comment Posting Details

**GitHub:** Uses the Pull Request Reviews API (`POST /repos/{owner}/{repo}/pulls/{number}/reviews`):
- Line-level comments on changed files (max 20 per review)
- Verdict mapping: `approve` → `APPROVE`, `request_changes` → `REQUEST_CHANGES`, `comment` → `COMMENT`
- Summary body with score, verdict, and general issues

**GitLab:** Uses the Discussions API (`POST /projects/{id}/merge_requests/{iid}/discussions`):
- Position-based comments with MR version SHAs (fetched from `/versions` endpoint)
- Falls back to summary-only comment if versions are unavailable
- Summary posted as top-level discussion

### Comment Formatting (`internal/review/format.go`)

- Severity labels: `[CRITICAL]`, `[MAJOR]`, `[MINOR]`, `[SUGGESTION]`
- Summary body: verdict, score, general issues, "Reviewed by CodeForge" footer
- Issue comments: severity label + description + suggestion

## 3. Webhook-Triggered PR Review

GitHub/GitLab can send webhooks to CodeForge when PRs/MRs are opened or updated. CodeForge automatically creates a `pr_review` task with `output_mode: "post_comments"`.

### Setup

**GitHub:**
1. Go to repo → Settings → Webhooks → Add webhook
2. Payload URL: `https://your-codeforge.com/api/v1/webhooks/github`
3. Content type: `application/json`
4. Secret: set to match `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITHUB`
5. Select "Pull requests" event

**GitLab:**
1. Go to project → Settings → Webhooks → Add webhook
2. URL: `https://your-codeforge.com/api/v1/webhooks/gitlab`
3. Secret token: set to match `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITLAB`
4. Enable "Merge request events"

### Configuration

```yaml
code_review:
  review_drafts: false          # skip draft PRs/MRs
  default_cli: "claude-code"    # CLI for reviews
  default_key_name: "my-github" # registered key for git auth (required)
  webhook_secrets:
    github: "your-github-secret"
    gitlab: "your-gitlab-secret"
```

### Security

- **GitHub**: HMAC-SHA256 signature verification via `X-Hub-Signature-256` header
- **GitLab**: Constant-time comparison of `X-Gitlab-Token` header
- Webhook endpoints are registered **outside** the Bearer auth group — they use webhook-specific verification
- Body size limited to 5MB

### Handled Events

| Provider | Event | Actions |
|----------|-------|---------|
| GitHub | `pull_request` | `opened`, `synchronize`, `reopened` |
| GitLab | `Merge Request Hook` | `open`, `update`, `reopen` |

Draft PRs and WIP MRs are skipped unless `review_drafts` is `true`.
