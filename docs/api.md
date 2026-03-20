# API Reference

Base URL: `http://localhost:8080`

All `/api/v1/*` endpoints require `Authorization: Bearer <token>` header.

Full OpenAPI 3.0 spec: [`api/openapi.yaml`](../api/openapi.yaml) | Swagger UI: `/api/docs`

---

## System (No Auth)

### Health Check

```
GET /health
```

```json
{
  "status": "ok",
  "redis": "connected",
  "sqlite": "connected",
  "version": "dev",
  "uptime": "5m30s",
  "workspace_disk_usage_mb": 123.45
}
```

### Readiness Probe

```
GET /ready
```

Returns `200` with `{"status": "ready"}` or `503` during shutdown.

### Info

```
GET /
```

```json
{
  "name": "CodeForge",
  "version": "dev",
  "links": {
    "api": "/api/v1",
    "docs": "/api/docs",
    "health": "/health",
    "metrics": "/metrics",
    "ready": "/ready"
  }
}
```

### Prometheus Metrics

```
GET /metrics
```

### API Docs

```
GET /api/docs              # Swagger UI
GET /api/docs/openapi.yaml # Raw OpenAPI spec
```

---

## Auth

### Verify Token

```
GET /api/v1/auth/verify
```

Response `200`:
```json
{ "status": "authenticated" }
```

Response `401`:
```json
{ "error": "unauthorized", "message": "missing or invalid Bearer token" }
```

---

## Tasks

### Task Lifecycle (State Machine)

See [Task Session Lifecycle](architecture.md#task-session-lifecycle) for a compact reference.

```
POST /tasks          → pending → cloning → running → completed
POST /instruct       → completed/awaiting_instruction → running → completed
POST /review         → completed/awaiting_instruction → reviewing → completed
POST /create-pr      → completed → creating_pr → pr_created
POST /tasks (pr_review) → pending → cloning → running → completed (with ReviewResult)
Webhook (PR opened)     → auto-creates pr_review task → same lifecycle as above
```

Valid transitions:

| From | To |
|------|-----|
| `pending` | `cloning`, `failed` |
| `cloning` | `running`, `failed` |
| `running` | `completed`, `failed` |
| `completed` | `awaiting_instruction`, `creating_pr`, `reviewing` |
| `reviewing` | `completed`, `failed` |
| `awaiting_instruction` | `running`, `reviewing`, `failed` |
| `creating_pr` | `pr_created`, `failed` |
| `pr_created` | `awaiting_instruction`, `completed` |
| `failed` | _(terminal)_ |

Finished (terminal) states: `completed`, `failed`, `pr_created`.

### Create Task

```
POST /api/v1/tasks
```

Request:
```json
{
  "repo_url": "https://github.com/user/repo.git",
  "prompt": "Fix the failing tests in the auth module",
  "task_type": "code",
  "provider_key": "my-github-key",
  "access_token": "ghp_xxx",
  "callback_url": "https://your-app.com/webhook",
  "config": {
    "timeout_seconds": 600,
    "cli": "claude-code",
    "ai_model": "claude-sonnet-4-20250514",
    "ai_api_key": "sk-ant-...",
    "max_turns": 20,
    "source_branch": "develop",
    "target_branch": "main",
    "max_budget_usd": 5.0,
    "workspace_task_id": "previous-task-uuid",
    "mcp_servers": [
      {
        "name": "context7",
        "command": "npx",
        "args": ["@anthropic-ai/context7"],
        "env": {"KEY": "value"}
      }
    ],
    "tools": [
      { "name": "sentry", "config": {"auth_token": "xxx"} }
    ]
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repo_url` | string | yes | Git repository URL |
| `prompt` | string | yes | Task instruction (max 100KB) |
| `task_type` | string | no | Task type: `code` (default), `plan`, `review`, `pr_review` |
| `provider_key` | string | no | Name of registered key for git auth |
| `access_token` | string | no | Inline git access token (never returned in responses) |
| `callback_url` | string | no | Webhook URL for completion notification |
| `config.timeout_seconds` | int | no | Task timeout (default: 300, max: 1800) |
| `config.cli` | string | no | CLI tool: `claude-code` (default), `codex` |
| `config.ai_model` | string | no | AI model override |
| `config.ai_api_key` | string | no | API key for AI provider (never returned) |
| `config.max_turns` | int | no | Max conversation turns |
| `config.source_branch` | string | no | Branch to clone/checkout |
| `config.target_branch` | string | no | Base branch for PR creation |
| `config.max_budget_usd` | float | no | Maximum spend in USD |
| `config.workspace_task_id` | string | no | Reuse workspace from another task |
| `config.mcp_servers` | array | no | Per-task MCP servers |
| `config.tools` | array | no | Per-task tool requests |
| `config.pr_number` | int | no | PR/MR number (required for `pr_review` tasks) |
| `config.output_mode` | string | no | `"post_comments"` or `"api_only"` (for `pr_review` tasks, default: `"api_only"`) |

Response `201`:
```json
{
  "id": "77a2ffbd-11b6-4654-9325-89306d55bc89",
  "status": "pending",
  "created_at": "2026-02-26T18:38:10.277Z"
}
```

Errors: `400` (validation), `429` (rate limited).

Rate limiting: Sliding window per bearer token — configurable via `rate_limit.sessions_per_minute`.

### List Tasks

```
GET /api/v1/tasks
GET /api/v1/tasks?status=completed&limit=10&offset=0
```

| Query Param | Type | Default | Description |
|-------------|------|---------|-------------|
| `status` | string | (all) | Filter by status |
| `limit` | int | 50 | Max results (max 200) |
| `offset` | int | 0 | Pagination offset |

Response `200`:
```json
{
  "tasks": [
    {
      "id": "77a2ffbd-...",
      "status": "completed",
      "task_type": "code",
      "repo_url": "https://github.com/user/repo.git",
      "prompt": "Fix the failing tests",
      "iteration": 1,
      "error": "",
      "branch": "codeforge/fix-the-failing-77a2ffbd",
      "pr_url": "",
      "created_at": "2026-02-26T18:38:10.277Z",
      "started_at": "2026-02-26T18:38:10.991Z",
      "finished_at": "2026-02-26T18:38:22.054Z"
    }
  ],
  "total": 1
}
```

### Get Task

```
GET /api/v1/tasks/{taskID}
GET /api/v1/tasks/{taskID}?include=iterations
```

| Query Param | Description |
|-------------|-------------|
| `include=iterations` | Load full iteration history |

Response `200`:
```json
{
  "id": "77a2ffbd-...",
  "status": "completed",
  "task_type": "code",
  "repo_url": "https://github.com/user/repo.git",
  "prompt": "Fix the failing tests",
  "result": "I fixed the authentication tests by...",
  "error": "",
  "config": {
    "timeout_seconds": 300,
    "cli": "claude-code"
  },
  "changes_summary": {
    "files_modified": 3,
    "files_created": 1,
    "files_deleted": 0,
    "diff_stats": "+142 -38"
  },
  "usage": {
    "input_tokens": 1500,
    "output_tokens": 500,
    "duration_seconds": 120
  },
  "review_result": {
    "verdict": "approve",
    "score": 8,
    "summary": "Good implementation with minor suggestions",
    "issues": [],
    "auto_fixable": false,
    "reviewed_by": "claude-code:claude-sonnet-4-20250514",
    "duration_seconds": 22.3
  },
  "iteration": 2,
  "current_prompt": "Also add unit tests",
  "iterations": [],
  "branch": "codeforge/fix-the-failing-77a2ffbd",
  "pr_number": 42,
  "pr_url": "https://github.com/user/repo/pull/42",
  "trace_id": "abc123...",
  "created_at": "2026-02-26T18:38:10.277Z",
  "started_at": "2026-02-26T18:38:10.991Z",
  "finished_at": "2026-02-26T18:38:22.054Z"
}
```

Fields with `omitempty` are omitted when empty/zero.

### Follow-up Instruction (Instruct)

Send a follow-up prompt to a completed task. Starts a new iteration in the same workspace.

```
POST /api/v1/tasks/{taskID}/instruct
```

Request:
```json
{ "prompt": "Also add unit tests for the changes you made" }
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | yes | Follow-up instruction (max 100KB) |

Task must be in `completed` or `awaiting_instruction` status.

Response `200`:
```json
{
  "id": "77a2ffbd-...",
  "status": "awaiting_instruction",
  "iteration": 2
}
```

Errors: `400` (validation), `404` (not found), `409` (wrong status).

### Code Review

Enqueue a code review of the task's workspace for async worker execution. Returns 202 immediately — the review runs in the worker pool with full SSE streaming, cancel support, and configurable timeout.

```
POST /api/v1/tasks/{taskID}/review
```

Request (all fields optional):
```json
{
  "cli": "claude-code",
  "model": "claude-sonnet-4-20250514"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cli` | string | no | CLI tool override for review (default: `claude-code`) |
| `model` | string | no | AI model override |

Task must be in `completed` or `awaiting_instruction` status.

Transitions: `completed` → `reviewing` → `completed` (with `review_result` stored on task).

Monitor progress via SSE: `GET /tasks/{taskID}/stream` (events: `review_started`, CLI output, `review_completed`).

Response `202`:
```json
{
  "id": "task-abc",
  "status": "reviewing"
}
```

The `ReviewResult` is stored on the task after completion — retrieve via `GET /tasks/{taskID}`.

**ReviewResult fields:**

| Field | Type | Description |
|-------|------|-------------|
| `verdict` | string | `approve`, `request_changes`, `comment` |
| `score` | int | Quality score 1-10 |
| `summary` | string | Human-readable summary |
| `issues` | array | List of findings |
| `issues[].severity` | string | `critical`, `major`, `minor`, `suggestion` |
| `issues[].file` | string | File path |
| `issues[].line` | int | Line number (0 if unknown) |
| `issues[].description` | string | Issue description |
| `issues[].suggestion` | string | Fix suggestion |
| `auto_fixable` | bool | Whether issues could be auto-fixed |
| `reviewed_by` | string | `cli:model` used |
| `duration_seconds` | float | Review duration |

Errors: `400` (bad CLI), `404` (not found), `409` (wrong status / workspace gone).

### PR Review (Automated)

Create a `pr_review` task to review a pull request / merge request diff. The task clones the target branch, fetches the PR ref (handles fork PRs automatically via `pull/{N}/head`), runs AI review, and stores a `ReviewResult`.

```
POST /api/v1/tasks
```

Request:
```json
{
  "repo_url": "https://github.com/user/repo.git",
  "provider_key": "my-github-key",
  "prompt": "Review pull request #42",
  "task_type": "pr_review",
  "config": {
    "cli": "claude-code",
    "pr_number": 42,
    "source_branch": "feature/login",
    "target_branch": "main",
    "output_mode": "post_comments"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `task_type` | string | yes | Must be `"pr_review"` |
| `config.pr_number` | int | yes | PR/MR number to review |
| `config.source_branch` | string | yes | PR head branch |
| `config.target_branch` | string | yes | PR base branch |
| `config.output_mode` | string | no | `"post_comments"` (post to PR) or `"api_only"` (default, results via API only) |

The task uses `git diff origin/{target_branch}...HEAD` to review only the PR changes. Fork PRs are handled automatically — CodeForge clones the target branch, then fetches the PR ref via `git fetch origin pull/{N}/head:pr-{N}`.

When `output_mode` is `"post_comments"`, the executor automatically posts review comments to the GitHub PR / GitLab MR after completion.

The `ReviewResult` is stored on the task and returned via `GET /tasks/{id}`.

### Post Review Comments

Post an existing task's `ReviewResult` as comments to a PR/MR. Useful when `output_mode` was `"api_only"` but you later want to post comments, or after a manual `POST /tasks/:id/review`.

```
POST /api/v1/tasks/{taskID}/post-review
```

Request (all fields optional):
```json
{
  "pr_number": 42
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pr_number` | int | no | Override PR number (defaults to task's `config.pr_number` or `pr_number`) |

Task must have a `review_result` (run a review first). Token is resolved from the task's `provider_key` or `access_token`.

Response `200`:
```json
{
  "review_url": "https://github.com/user/repo/pull/42#pullrequestreview-123",
  "comments_posted": 3,
  "pr_number": 42
}
```

**GitHub:** Posts a Pull Request Review with line-level comments (max 20) and verdict mapping (approve→APPROVE, request_changes→REQUEST_CHANGES).

**GitLab:** Posts MR Discussions with position-based comments. Falls back to summary-only if MR versions are unavailable.

Errors: `400` (no review result / no PR number / token resolution failed), `404` (not found).

### Cancel Task

```
POST /api/v1/tasks/{taskID}/cancel
```

Task must be in `cloning` or `running` status.

Response `200`:
```json
{
  "id": "77a2ffbd-...",
  "status": "canceling",
  "message": "task cancellation requested"
}
```

Note: `canceling` is a transient response status, not a stored state. Poll `GET /tasks/{id}` — final state will be `failed`.

Errors: `404` (not found), `409` (not running/cloning).

### Create Pull Request

```
POST /api/v1/tasks/{taskID}/create-pr
```

Request (all fields optional):
```json
{
  "title": "Fix auth tests",
  "description": "AI-generated fix for failing auth tests",
  "target_branch": "main"
}
```

Task must be in `completed` status with actual file changes.

Response `200`:
```json
{
  "pr_url": "https://github.com/user/repo/pull/42",
  "pr_number": 42,
  "branch": "codeforge/fix-the-failing-77a2ffbd"
}
```

Errors: `400` (no changes / not supported), `404` (not found), `409` (wrong status).

### Stream Task Events (SSE)

Opens a Server-Sent Events connection for real-time task progress.

```
GET /api/v1/tasks/{taskID}/stream
```

**Connection flow:**
1. Subscribes to Redis Pub/Sub (before reading history — no missed events)
2. Sends `event: connected` with current status
3. Replays all historical events from Redis
4. Streams live events
5. Sends `event: done` when task finishes
6. Auto-closes after 10 minutes

**Named SSE events:**

| Event | Data | Description |
|-------|------|-------------|
| `connected` | `{"task_id": "...", "status": "running"}` | Initial connection |
| `done` | `{"task_id": "...", "status": "completed"}` | Task finished, stream closes |
| `timeout` | `{"message": "stream closed after 10 minutes"}` | Stream timeout |

**Unnamed data events** (JSON objects):

```json
{
  "type": "system|git|stream|result",
  "event": "event_name",
  "data": { ... },
  "ts": "2026-02-26T10:15:30.123456Z"
}
```

#### Event Types

**System events** (`type: "system"`):

| Event | Data | When |
|-------|------|------|
| `cli_started` | `{"cli": "claude-code", "iteration": "1"}` | CLI execution begins |
| `task_timeout` | `{"timeout_seconds": 300}` | Task times out |
| `task_canceled` | `null` | User cancels task |
| `task_failed` | `{"error": "..."}` | Task fails |
| `review_started` | `null` | Code review starts |
| `review_completed` | `{"verdict": "approve", "score": 8, "issues_count": 0}` | Review finishes |

**Git events** (`type: "git"`):

| Event | Data | When |
|-------|------|------|
| `clone_started` | `{"repo_url": "https://github.com/..."}` | Clone begins |
| `clone_completed` | `{"work_dir": "/data/workspaces/..."}` | Clone done |

**Stream events** (`type: "stream"`) — Normalized CLI output:

```json
{
  "type": "stream",
  "event": "output",
  "data": {
    "type": "thinking|text|tool_use|tool_result|result|error|system",
    "content": "The agent's response text...",
    "cli": "claude-code",
    "raw": { ... }
  },
  "ts": "..."
}
```

| Normalized type | Description |
|-----------------|-------------|
| `thinking` | Claude thinking/reasoning block |
| `text` | Agent text response |
| `tool_use` | Agent is using a tool (MCP) |
| `tool_result` | Tool execution result |
| `result` | Final execution result |
| `error` | Execution error |
| `system` | System-level event |

**Result events** (`type: "result"`):

| Event | Data | When |
|-------|------|------|
| `task_completed` | `{"result": "...", "changes_summary": {...}, "usage": {...}, "iteration": 1}` | Task succeeds |

#### Keepalive

Comment lines (`: keepalive`) sent every 15 seconds to prevent proxy timeouts.

#### JavaScript Client Example

```javascript
// Browser EventSource does NOT support custom headers.
// Use fetch + ReadableStream:
const resp = await fetch('/api/v1/tasks/abc/stream', {
  headers: { Authorization: 'Bearer TOKEN' },
});
const reader = resp.body.getReader();
const decoder = new TextDecoder();

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  const text = decoder.decode(value);
  for (const line of text.split('\n')) {
    if (line.startsWith('event: ')) {
      // Named event: connected, done, timeout
      currentEvent = line.slice(7);
    } else if (line.startsWith('data: ')) {
      const data = JSON.parse(line.slice(6));
      handleEvent(currentEvent, data);
    }
  }
}
```

**Polling fallback:** If SSE is not feasible, poll `GET /api/v1/tasks/{id}` every 2-5 seconds.

---

## Task Types

### List Task Types

Returns available task types for the frontend toggle buttons.

```
GET /api/v1/task-types
```

Response `200`:
```json
{
  "task_types": [
    {
      "name": "code",
      "label": "Code",
      "description": "Write or modify code based on the prompt"
    },
    {
      "name": "plan",
      "label": "Plan",
      "description": "Analyze the codebase and create an implementation plan without modifying files"
    },
    {
      "name": "review",
      "label": "Review",
      "description": "Review repository code quality, security, and architecture"
    },
    {
      "name": "pr_review",
      "label": "PR Review",
      "description": "Review a pull request / merge request diff and post comments"
    }
  ]
}
```

**Task type behavior:**

| Type | Template | Behavior |
|------|----------|----------|
| `code` | None (user prompt as-is) | Default — writes/modifies code |
| `plan` | `plan.md` | Read-only analysis, creates implementation plan, does NOT modify files |
| `review` | `review.md` | Reviews code quality with structured JSON output, does NOT modify files |
| `pr_review` | `pr_review.md` | Reviews a PR/MR diff, outputs structured JSON, optionally posts comments to PR/MR |

**Review distinctions:**
- `review` task type — reviews the **entire repository** as a new task
- `POST /tasks/:id/review` — reviews **changes of a specific task** (git diff HEAD~1)
- `pr_review` task type — reviews **a specific PR/MR diff** (git diff origin/{base}...HEAD)

---

## CLI

### List CLIs

```
GET /api/v1/cli
```

Response `200`:
```json
{
  "cli": [
    {
      "name": "claude-code",
      "binary_path": "/usr/local/bin/claude",
      "default_model": "claude-sonnet-4-20250514",
      "available": true,
      "is_default": true
    },
    {
      "name": "codex",
      "binary_path": "/usr/local/bin/codex",
      "default_model": "",
      "available": true,
      "is_default": false
    }
  ]
}
```

### CLI Health Check

```
GET /api/v1/cli/health
```

Response `200`:
```json
{ "status": "ok", "cli": "claude-code", "binary": "/usr/local/bin/claude" }
```

Response `503`:
```json
{ "status": "unavailable", "cli": "claude-code", "binary": "", "message": "binary not found" }
```

---

## Keys

Manage encrypted access keys for providers (GitHub, GitLab, Sentry). Tokens are encrypted with AES-256-GCM and never returned in API responses.

### Register Key

```
POST /api/v1/keys
```

```json
{
  "name": "my-github-key",
  "provider": "github",
  "token": "ghp_xxx",
  "scope": "org/repo"
}
```

Provider values: `github`, `gitlab`, `sentry`

Sentry example:
```json
{
  "name": "my-sentry",
  "provider": "sentry",
  "token": "sntrys_xxx"
}
```

Response `201`:
```json
{ "name": "my-github-key", "provider": "github", "message": "key registered" }
```

### List Keys

```
GET /api/v1/keys
```

```json
{
  "keys": [
    { "name": "my-github-key", "provider": "github", "scope": "" }
  ]
}
```

### Verify Key

Tests a stored token against the provider API. Use to show validity status in UI.

```
GET /api/v1/keys/{name}/verify
```

Response `200` (valid):
```json
{
  "name": "my-github-key",
  "provider": "github",
  "valid": true,
  "username": "octocat",
  "email": "octocat@github.com",
  "scopes": "repo, read:org",
  "error": ""
}
```

Response `422` (invalid token):
```json
{
  "name": "my-github-key",
  "provider": "github",
  "valid": false,
  "error": "token expired"
}
```

### Delete Key

```
DELETE /api/v1/keys/{name}
```

---

## Repositories

List repositories accessible with a provider token.

```
GET /api/v1/repositories?provider_key=my-github-key&page=1&per_page=30
GET /api/v1/repositories?provider=github&page=1&per_page=30
  (with header: X-Provider-Token: ghp_xxx)
```

Two auth modes:
1. `provider_key` — uses a stored key from the registry
2. `provider` + `X-Provider-Token` header — inline token

| Query Param | Default | Description |
|-------------|---------|-------------|
| `provider_key` | — | Registered key name (mode 1) |
| `provider` | — | `github` or `gitlab` (mode 2) |
| `page` | 1 | Page number |
| `per_page` | 30 | Results per page (max 100) |

Response `200`:
```json
{
  "repositories": [
    {
      "name": "my-repo",
      "full_name": "user/my-repo",
      "clone_url": "https://github.com/user/my-repo.git",
      "default_branch": "main",
      "private": true,
      "description": "My repo description",
      "updated_at": "2026-02-20T10:00:00Z"
    }
  ],
  "count": 1,
  "provider": "github",
  "page": 1,
  "per_page": 30
}
```

---

## Sentry Proxy

Proxy endpoints for browsing Sentry data. All endpoints require a `key_name` query param pointing to a stored Sentry key. Responses from Sentry are forwarded as-is (list endpoints wrapped in a named object).

### List Organizations

```
GET /api/v1/sentry/organizations?key_name=my-sentry
```

### List Projects

```
GET /api/v1/sentry/projects?key_name=my-sentry&org=my-org
```

### List Issues

```
GET /api/v1/sentry/issues?key_name=my-sentry&org=my-org&project=my-project
```

| Query Param | Default | Description |
|-------------|---------|-------------|
| `key_name` | — | Stored Sentry key name (required) |
| `org` | — | Sentry organization slug (required) |
| `project` | — | Sentry project slug (required) |
| `query` | `is:unresolved` | Sentry search query |
| `sort` | — | Sort order (freq, date, priority) |
| `limit` | 50 | Max results |

### Get Issue Detail

```
GET /api/v1/sentry/issues/{issueID}?key_name=my-sentry
```

Returns raw Sentry issue JSON (title, culprit, count, metadata, etc.).

### Get Latest Event

```
GET /api/v1/sentry/issues/{issueID}/latest-event?key_name=my-sentry
```

Returns full event with stack trace, breadcrumbs, tags, and context.

---

## MCP Servers

Manage global MCP (Model Context Protocol) servers. These are injected into the Claude Code `.mcp.json` at task runtime.

### Register MCP Server

Supports two transport types: `stdio` (local process) and `http` (remote server).

```
POST /api/v1/mcp/servers
```

Stdio transport (default):
```json
{
  "name": "context7",
  "transport": "stdio",
  "package": "@anthropic-ai/context7",
  "command": "npx",
  "args": ["--transport", "stdio"],
  "env": { "API_KEY": "xxx" }
}
```

HTTP transport:
```json
{
  "name": "context7",
  "transport": "http",
  "url": "https://mcp.context7.com/mcp",
  "headers": { "CONTEXT7_API_KEY": "xxx" }
}
```

| Field | Transport | Required | Description |
|-------|-----------|----------|-------------|
| `name` | both | yes | Unique server name |
| `transport` | both | no | `stdio` (default) or `http` |
| `package` | stdio | yes | NPM package or binary path |
| `command` | stdio | no | Command (npx, uvx, docker) |
| `args` | stdio | no | Command arguments |
| `env` | stdio | no | Environment variables |
| `url` | http | yes | Server URL |
| `headers` | http | no | HTTP headers |

### List MCP Servers

```
GET /api/v1/mcp/servers
```

```json
{
  "servers": [
    {
      "name": "context7",
      "transport": "http",
      "url": "https://mcp.context7.com/mcp",
      "headers": { "CONTEXT7_API_KEY": "xxx" }
    }
  ]
}
```

### Delete MCP Server

```
DELETE /api/v1/mcp/servers/{name}
```

---

## Tools

High-level tool abstraction over MCP. Tools are registered in a catalog with required configuration fields. When a task requests a tool, CodeForge resolves it to an MCP server and injects it.

### List Tool Catalog (Built-in)

```
GET /api/v1/tools/catalog
```

Returns built-in tools: `sentry`, `jira`, `git`, `github`, `playwright`.

### List All Tools

```
GET /api/v1/tools
```

### Get Tool

```
GET /api/v1/tools/{name}
```

Response `200`:
```json
{
  "name": "sentry",
  "type": "mcp",
  "description": "Sentry error tracking — search issues, get stack traces, resolve errors",
  "mcp_package": "@sentry/mcp-server",
  "mcp_command": "npx",
  "required_config": [
    {
      "name": "auth_token",
      "description": "Sentry authentication token",
      "type": "secret",
      "env_var": "SENTRY_AUTH_TOKEN",
      "sensitive": true
    }
  ],
  "capabilities": ["error-tracking", "issues"],
  "builtin": true,
  "created_at": "2026-02-25T00:00:00Z"
}
```

### Register Tool

```
POST /api/v1/tools
```

```json
{
  "name": "my-tool",
  "type": "mcp",
  "description": "My custom tool",
  "mcp_package": "my-mcp-server",
  "mcp_command": "npx",
  "required_config": [
    { "name": "api_key", "description": "API key", "type": "secret", "env_var": "MY_API_KEY", "sensitive": true }
  ],
  "capabilities": ["my-feature"]
}
```

### Delete Tool

```
DELETE /api/v1/tools/{name}
```

Built-in tools cannot be deleted.

---

## Workspaces

Manage task workspace directories on disk.

### List Workspaces

```
GET /api/v1/workspaces
```

```json
{
  "workspaces": [
    {
      "task_id": "77a2ffbd-...",
      "size_mb": 45.2,
      "created_at": "2026-02-26T18:38:10Z",
      "expires_at": "2026-02-27T18:38:10Z",
      "task_status": "completed"
    }
  ],
  "total_size_mb": 45.2,
  "total_count": 1
}
```

### Delete Workspace

```
DELETE /api/v1/workspaces/{taskID}
```

Cannot delete workspace of a running task (`409`).

---

## Workflows

Multi-step workflow orchestration. Chains fetch, task, and action steps.

### Create Workflow

```
POST /api/v1/workflows
```

```json
{
  "name": "my-workflow",
  "description": "Custom workflow",
  "steps": [
    {
      "name": "fetch_data",
      "type": "fetch",
      "config": {
        "url": "https://api.example.com/data/{{.Params.item_id}}",
        "method": "GET",
        "outputs": { "title": "$.title", "body": "$.body" }
      }
    },
    {
      "name": "fix_it",
      "type": "task",
      "config": {
        "repo_url": "{{.Params.repo_url}}",
        "prompt": "Fix: {{.Steps.fetch_data.title}}\n\n{{.Steps.fetch_data.body}}",
        "provider_key": "{{.Params.provider_key}}"
      }
    },
    {
      "name": "open_pr",
      "type": "action",
      "config": {
        "kind": "create_pr",
        "task_step_ref": "fix_it",
        "title": "fix: {{.Steps.fetch_data.title}}"
      }
    }
  ],
  "parameters": [
    { "name": "item_id", "required": true },
    { "name": "repo_url", "required": true },
    { "name": "provider_key", "required": false }
  ]
}
```

**Step types:**

| Type | Description |
|------|-------------|
| `fetch` | HTTP request with JSONPath output extraction |
| `task` | Creates a CodeForge task (clone + AI CLI run) |
| `action` | Built-in action: `create_pr` |

**Template syntax:** `{{.Params.key}}` for inputs, `{{.Steps.step_name.field}}` for step outputs.

**Built-in workflows:** `sentry-fixer`, `github-issue-fixer`, `gitlab-issue-fixer`, `code-review`, `knowledge-update`.

### List Workflows

```
GET /api/v1/workflows
```

### Get Workflow

```
GET /api/v1/workflows/{name}
```

### Delete Workflow

```
DELETE /api/v1/workflows/{name}
```

Built-in workflows cannot be deleted.

### Run Workflow

```
POST /api/v1/workflows/{name}/run
```

```json
{
  "params": {
    "repo_url": "https://github.com/user/repo.git",
    "issue_number": "42",
    "provider_key": "my-github-key"
  }
}
```

Response `201`:
```json
{
  "run_id": "550e8400-...",
  "workflow_name": "github-issue-fixer",
  "status": "pending",
  "created_at": "2026-02-26T10:30:00Z"
}
```

---

## Workflow Runs

### List Runs

```
GET /api/v1/workflow-runs
GET /api/v1/workflow-runs?workflow=sentry-fixer
```

### Get Run

```
GET /api/v1/workflow-runs/{runID}
```

```json
{
  "id": "550e8400-...",
  "workflow_name": "github-issue-fixer",
  "status": "completed",
  "params": { "repo_url": "...", "issue_number": "42" },
  "steps": [
    {
      "step_name": "fetch_issue",
      "step_type": "fetch",
      "status": "completed",
      "result": { "title": "Bug in auth", "body": "..." },
      "started_at": "...",
      "finished_at": "..."
    },
    {
      "step_name": "fix_issue",
      "step_type": "task",
      "status": "completed",
      "task_id": "abc-123-...",
      "started_at": "...",
      "finished_at": "..."
    }
  ],
  "created_at": "...",
  "started_at": "...",
  "finished_at": "..."
}
```

### Stream Workflow Run (SSE)

```
GET /api/v1/workflow-runs/{runID}/stream
```

Same SSE pattern as task streaming. Named events: `connected`, `done`, `timeout` (30 min).

Workflow events:

| Event | Data |
|-------|------|
| `workflow_started` | `{"workflow": "name", "run_id": "..."}` |
| `step_started` | `{"step": "name", "type": "fetch\|task\|action"}` |
| `step_completed` | `{"step": "name"}` |
| `step_failed` | `{"step": "name", "error": "..."}` |

### Cancel Workflow Run

```
POST /api/v1/workflow-runs/{runID}/cancel
```

Cancels a running workflow run and its active task.

**Response:** `200 OK`
```json
{"status": "canceling", "message": "workflow run cancellation requested"}
```

### Cancel All Runs for Workflow

```
POST /api/v1/workflow-runs/cancel-all
```

**Query params:** `workflow` (required) — workflow name.

Cancels all pending/running runs for a specific workflow.

**Response:** `200 OK`
```json
{"canceled": 2, "message": "2 workflow runs canceled"}
```

---

## PR Webhook Receivers (No Auth)

Webhook endpoints for GitHub/GitLab to automatically create `pr_review` tasks when PRs/MRs are opened or updated. These endpoints use webhook secret verification instead of Bearer auth.

### GitHub Webhook

```
POST /api/v1/webhooks/github
```

**Setup:** In your GitHub repo → Settings → Webhooks → Add webhook:
- **Payload URL:** `https://your-codeforge.com/api/v1/webhooks/github`
- **Content type:** `application/json`
- **Secret:** same as `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITHUB`
- **Events:** select "Pull requests"

**Verification:** HMAC-SHA256 via `X-Hub-Signature-256` header.

**Handled events:** `pull_request` with actions `opened`, `synchronize`, `reopened`.

**Draft PRs:** Skipped unless `code_review.review_drafts` is `true`.

**Created task:**
```json
{
  "repo_url": "(from webhook payload)",
  "provider_key": "(from code_review.default_key_name)",
  "prompt": "Review pull request #42",
  "task_type": "pr_review",
  "config": {
    "cli": "(from code_review.default_cli)",
    "source_branch": "feature/login",
    "target_branch": "main",
    "pr_number": 42,
    "output_mode": "post_comments"
  }
}
```

Response `201`:
```json
{ "status": "created", "task_id": "abc-123-..." }
```

### GitLab Webhook

```
POST /api/v1/webhooks/gitlab
```

**Setup:** In your GitLab project → Settings → Webhooks → Add webhook:
- **URL:** `https://your-codeforge.com/api/v1/webhooks/gitlab`
- **Secret token:** same as `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITLAB`
- **Trigger:** "Merge request events"

**Verification:** Constant-time comparison of `X-Gitlab-Token` header.

**Handled events:** `Merge Request Hook` with actions `open`, `update`, `reopen`.

**Draft/WIP MRs:** Skipped unless `code_review.review_drafts` is `true`.

Response `201`:
```json
{ "status": "created", "task_id": "abc-123-..." }
```

### Webhook Configuration

| Variable | Description |
|----------|-------------|
| `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITHUB` | HMAC secret for GitHub webhook verification |
| `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITLAB` | Secret token for GitLab webhook verification |
| `CODEFORGE_CODE_REVIEW__DEFAULT_KEY_NAME` | Registered key name for git auth (required for webhooks) |
| `CODEFORGE_CODE_REVIEW__DEFAULT_CLI` | CLI to use for reviews (default: `claude-code`) |
| `CODEFORGE_CODE_REVIEW__REVIEW_DRAFTS` | Review draft PRs/MRs (default: `false`) |

---

## Webhook Callbacks

When a task completes or fails, CodeForge sends a POST to the `callback_url`:

```json
{
  "task_id": "550e8400-...",
  "status": "completed",
  "result": "Task completed successfully...",
  "changes_summary": {
    "files_modified": 3,
    "files_created": 1,
    "files_deleted": 0,
    "diff_stats": "+45 -12"
  },
  "usage": {
    "input_tokens": 1500,
    "output_tokens": 500,
    "duration_seconds": 120
  },
  "trace_id": "abc123...",
  "finished_at": "2026-02-26T10:35:00Z"
}
```

Headers:
- `X-Signature-256: sha256=<hmac>` — HMAC-SHA256 of body
- `X-CodeForge-Event: task.completed` — Event type
- `X-Trace-ID: <trace_id>` — OpenTelemetry trace ID

---

## Error Format

All errors follow this structure:

```json
{
  "error": "Bad Request",
  "message": "prompt: field is required",
  "fields": { "prompt": "field is required" }
}
```

| Status | When |
|--------|------|
| `400` | Validation error, bad input |
| `401` | Missing or invalid Bearer token |
| `404` | Resource not found |
| `409` | State conflict (wrong status for operation) |
| `429` | Rate limit exceeded (has `Retry-After` header) |
| `500` | Internal server error |

---

## Typical FE Flows

### Code Task (interactive)

```
1. Verify auth       → GET  /api/v1/auth/verify
2. Load task types   → GET  /api/v1/task-types
3. List repos        → GET  /api/v1/repositories?provider_key=...
4. Create task       → POST /api/v1/tasks  {repo_url, prompt, task_type}
5. Stream progress   → GET  /api/v1/tasks/{id}/stream  (SSE)
6. View result       → GET  /api/v1/tasks/{id}
7. (optional) Review → POST /api/v1/tasks/{id}/review
8. (optional) Fix    → POST /api/v1/tasks/{id}/instruct  {prompt}
9. (optional) PR     → POST /api/v1/tasks/{id}/create-pr
```

Steps 7-9 are repeatable in any order. The user decides what to do after each step.

### PR Review (API-triggered)

```
1. Create pr_review task → POST /api/v1/tasks  {task_type: "pr_review", config: {pr_number, ...}}
2. Stream or poll        → GET  /api/v1/tasks/{id}/stream
3. View ReviewResult     → GET  /api/v1/tasks/{id}  (review_result field)
4. (optional) Post       → POST /api/v1/tasks/{id}/post-review
```

### PR Review (webhook-triggered)

```
1. GitHub/GitLab sends webhook → POST /api/v1/webhooks/github|gitlab
2. CodeForge auto-creates pr_review task with output_mode: "post_comments"
3. On completion, review comments posted automatically to PR/MR
```

Step 2 in the code task flow can be cached — task types don't change at runtime.
