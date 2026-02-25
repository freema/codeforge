# API Reference

All endpoints require `Authorization: Bearer <token>` unless noted otherwise.

Full OpenAPI 3.0 spec: [`api/openapi.yaml`](../api/openapi.yaml)

## Auth

### Verify Token

```bash
curl http://localhost:8080/api/v1/auth/verify \
  -H "Authorization: Bearer $TOKEN"
```

Response (200):
```json
{
  "authenticated": true
}
```

## Repositories

### List Repositories

Lists repositories accessible via a registered key or inline token.

**Using a registered key:**

```bash
curl "http://localhost:8080/api/v1/repositories?provider_key=my-github-key&page=1&per_page=30" \
  -H "Authorization: Bearer $TOKEN"
```

**Using an inline token:**

```bash
curl "http://localhost:8080/api/v1/repositories?provider=github&page=1&per_page=30" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Provider-Token: ghp_xxx"
```

Response (200):
```json
{
  "repositories": [
    {
      "name": "my-repo",
      "full_name": "user/my-repo",
      "clone_url": "https://github.com/user/my-repo.git",
      "default_branch": "main",
      "private": true
    }
  ],
  "count": 1,
  "provider": "github",
  "page": 1,
  "per_page": 30
}
```

Query parameters:

| Parameter | Description |
|-----------|-------------|
| `provider_key` | Name of a registered key (mode 1) |
| `provider` | `github` or `gitlab` (mode 2, requires `X-Provider-Token` header) |
| `page` | Page number (default: 1) |
| `per_page` | Results per page, max 100 (default: 30) |

## Tasks

### Create Task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "access_token": "ghp_xxx",
    "prompt": "Fix the failing tests in the auth module",
    "callback_url": "https://your-app.com/webhook",
    "config": {
      "timeout_seconds": 600,
      "ai_model": "claude-sonnet-4-20250514",
      "max_turns": 20,
      "target_branch": "develop"
    }
  }'
```

Response (201):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "created_at": "2025-01-15T10:30:00Z"
}
```

### Get Task

```bash
curl http://localhost:8080/api/v1/tasks/{id} \
  -H "Authorization: Bearer $TOKEN"

# Include iteration history:
curl http://localhost:8080/api/v1/tasks/{id}?include=iterations \
  -H "Authorization: Bearer $TOKEN"
```

### Follow-up Instruction

Send a follow-up prompt to a completed task (starts a new iteration in the same workspace):

```bash
curl -X POST http://localhost:8080/api/v1/tasks/{id}/instruct \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Also add unit tests for the changes you made"}'
```

### Cancel Task

```bash
curl -X POST http://localhost:8080/api/v1/tasks/{id}/cancel \
  -H "Authorization: Bearer $TOKEN"
```

### Stream Task Events (SSE)

Opens a Server-Sent Events connection to stream real-time task progress. The endpoint replays all historical events first, then streams live events via Redis Pub/Sub.

```bash
curl -N http://localhost:8080/api/v1/tasks/{id}/stream \
  -H "Authorization: Bearer $TOKEN"
```

**Named events:**

| Event | Description |
|-------|-------------|
| `connected` | Initial event with task ID and current status |
| `done` | Task finished (completed/failed/pr_created). Connection closes after this. |
| `timeout` | Connection closed after 10 minutes of inactivity |

**Unnamed data events** are JSON objects with the structure:

```json
{
  "type": "stream",
  "event": "output",
  "data": { "...Claude Code stream-json event..." },
  "ts": "2025-01-15T10:31:00Z"
}
```

Event types: `system`, `git`, `cli`, `stream`, `result`.

**Behavior:**
1. Subscribes to live Redis Pub/Sub channels *before* reading history (prevents missed events).
2. Sends `event: connected` with current task state.
3. Replays all events from Redis history list.
4. For terminal tasks (completed/failed/pr_created), sends history + `done` immediately and closes.
5. For active tasks, streams live events with 15s keepalive comments (`: keepalive`).
6. Auto-closes after 10 minutes.

**JavaScript client example:**

```javascript
const es = new EventSource('/api/v1/tasks/abc/stream', {
  headers: { Authorization: 'Bearer TOKEN' }
});

es.onmessage = (e) => {
  const event = JSON.parse(e.data);
  console.log(event.type, event.event, event.data);
};

es.addEventListener('done', (e) => {
  const { status } = JSON.parse(e.data);
  console.log('Task finished:', status);
  es.close();
});

es.addEventListener('connected', (e) => {
  const { task_id, status } = JSON.parse(e.data);
  console.log('Connected to task:', task_id, status);
});
```

**Polling fallback:** If your client doesn't support SSE, use `GET /api/v1/tasks/{id}` to poll task status.

### Create Pull Request

```bash
curl -X POST http://localhost:8080/api/v1/tasks/{id}/create-pr \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Fix auth tests",
    "description": "AI-generated fix for failing auth tests",
    "target_branch": "main"
  }'
```

Response (200):
```json
{
  "pr_url": "https://github.com/user/repo/pull/42",
  "pr_number": 42,
  "branch": "codeforge/task-550e8400"
}
```

## Keys

### Register Key

```bash
curl -X POST http://localhost:8080/api/v1/keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-github-key",
    "provider": "github",
    "token": "ghp_xxx"
  }'
```

### List Keys

Tokens are redacted in the response.

```bash
curl http://localhost:8080/api/v1/keys \
  -H "Authorization: Bearer $TOKEN"
```

### Verify Key

Tests a registered key against its provider API and returns token validity, scopes, and user info.

```bash
curl http://localhost:8080/api/v1/keys/my-github-key/verify \
  -H "Authorization: Bearer $TOKEN"
```

Response (200 if valid, 422 if invalid):
```json
{
  "name": "my-github-key",
  "provider": "github",
  "valid": true,
  "username": "octocat",
  "email": "octocat@github.com",
  "scopes": ["repo", "read:org"],
  "error": ""
}
```

### Delete Key

```bash
curl -X DELETE http://localhost:8080/api/v1/keys/my-github-key \
  -H "Authorization: Bearer $TOKEN"
```

## MCP Servers

### Register Global MCP Server

```bash
curl -X POST http://localhost:8080/api/v1/mcp/servers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "context7",
    "package": "@anthropic-ai/context7",
    "args": ["--transport", "stdio"]
  }'
```

### List MCP Servers

```bash
curl http://localhost:8080/api/v1/mcp/servers \
  -H "Authorization: Bearer $TOKEN"
```

### Delete MCP Server

```bash
curl -X DELETE http://localhost:8080/api/v1/mcp/servers/context7 \
  -H "Authorization: Bearer $TOKEN"
```

## Workspaces

### List Workspaces

```bash
curl http://localhost:8080/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN"
```

### Delete Workspace

```bash
curl -X DELETE http://localhost:8080/api/v1/workspaces/{task_id} \
  -H "Authorization: Bearer $TOKEN"
```

## Workflows

### Create Workflow

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-workflow",
    "description": "Custom workflow",
    "steps": [
      {
        "name": "fetch_data",
        "type": "fetch",
        "config": {
          "url": "https://api.example.com/data/{{.Params.item_id}}",
          "method": "GET",
          "outputs": {"title": "$.title", "body": "$.body"}
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
      {"name": "item_id", "required": true},
      {"name": "repo_url", "required": true},
      {"name": "provider_key", "required": false}
    ]
  }'
```

Response (201):
```json
{
  "name": "my-workflow",
  "message": "workflow created"
}
```

Step types:

| Type | Description |
|------|-------------|
| `fetch` | HTTP request to external API with JSONPath output extraction |
| `task` | Creates a CodeForge task (clone repo + AI CLI run) |
| `action` | Built-in action (`create_pr`, `notify`) operating on previous step results |

Template syntax: `{{.Params.key}}` for input parameters, `{{.Steps.step_name.field}}` for previous step outputs.

### List Workflows

```bash
curl http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer $TOKEN"
```

Response (200):
```json
{
  "workflows": [
    {
      "name": "sentry-fixer",
      "description": "Fetches a Sentry issue, creates a task to fix it, then creates a PR",
      "builtin": true,
      "steps": [...],
      "parameters": [...]
    },
    {
      "name": "github-issue-fixer",
      "description": "Fetches a GitHub issue, creates a task to fix it, then creates a PR",
      "builtin": true,
      "steps": [...],
      "parameters": [...]
    }
  ]
}
```

### Get Workflow

```bash
curl http://localhost:8080/api/v1/workflows/sentry-fixer \
  -H "Authorization: Bearer $TOKEN"
```

### Delete Workflow

Built-in workflows cannot be deleted.

```bash
curl -X DELETE http://localhost:8080/api/v1/workflows/my-workflow \
  -H "Authorization: Bearer $TOKEN"
```

### Run Workflow

```bash
curl -X POST http://localhost:8080/api/v1/workflows/github-issue-fixer/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "params": {
      "repo_url": "https://github.com/user/repo.git",
      "issue_number": "42",
      "key_name": "my-github-key",
      "provider_key": "my-github-key"
    }
  }'
```

Response (201):
```json
{
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "workflow_name": "github-issue-fixer",
  "status": "pending",
  "created_at": "2025-01-15T10:30:00Z"
}
```

## Workflow Runs

### List Runs

```bash
# All runs
curl http://localhost:8080/api/v1/workflow-runs \
  -H "Authorization: Bearer $TOKEN"

# Filter by workflow
curl "http://localhost:8080/api/v1/workflow-runs?workflow=sentry-fixer" \
  -H "Authorization: Bearer $TOKEN"
```

Response (200):
```json
{
  "runs": [
    {
      "id": "550e8400-...",
      "workflow_name": "github-issue-fixer",
      "status": "completed",
      "params": {"repo_url": "...", "issue_number": "42"},
      "created_at": "2025-01-15T10:30:00Z",
      "started_at": "2025-01-15T10:30:01Z",
      "finished_at": "2025-01-15T10:35:00Z"
    }
  ]
}
```

### Get Run

Returns run details including step execution records.

```bash
curl http://localhost:8080/api/v1/workflow-runs/{runID} \
  -H "Authorization: Bearer $TOKEN"
```

Response (200):
```json
{
  "id": "550e8400-...",
  "workflow_name": "github-issue-fixer",
  "status": "completed",
  "params": {"repo_url": "...", "issue_number": "42"},
  "steps": [
    {
      "step_name": "fetch_issue",
      "step_type": "fetch",
      "status": "completed",
      "result": {"title": "Bug in auth", "body": "..."},
      "started_at": "2025-01-15T10:30:01Z",
      "finished_at": "2025-01-15T10:30:02Z"
    },
    {
      "step_name": "fix_issue",
      "step_type": "task",
      "status": "completed",
      "task_id": "abc-123-...",
      "started_at": "2025-01-15T10:30:02Z",
      "finished_at": "2025-01-15T10:34:50Z"
    },
    {
      "step_name": "create_pr",
      "step_type": "action",
      "status": "completed",
      "result": {"pr_url": "https://github.com/user/repo/pull/43"},
      "started_at": "2025-01-15T10:34:50Z",
      "finished_at": "2025-01-15T10:35:00Z"
    }
  ],
  "created_at": "2025-01-15T10:30:00Z",
  "started_at": "2025-01-15T10:30:01Z",
  "finished_at": "2025-01-15T10:35:00Z"
}
```

### Stream Workflow Run (SSE)

Opens a Server-Sent Events connection to stream real-time workflow progress. Same pattern as task streaming: history replay + live events.

```bash
curl -N http://localhost:8080/api/v1/workflow-runs/{runID}/stream \
  -H "Authorization: Bearer $TOKEN"
```

**Named events:**

| Event | Description |
|-------|-------------|
| `connected` | Initial event with run ID and current status |
| `done` | Workflow finished (completed/failed). Connection closes after this. |
| `timeout` | Connection closed after 30 minutes |

**Unnamed data events** include step lifecycle events (`step_started`, `step_completed`, `step_failed`) and nested task stream events.

## System (No Auth)

### Health Check

```bash
curl http://localhost:8080/health
```

### Readiness Probe

```bash
curl http://localhost:8080/ready
```

### Metrics

```bash
curl http://localhost:8080/metrics
```

### API Documentation (Swagger UI)

Interactive API docs served from the embedded OpenAPI spec:

```bash
# Swagger UI
open http://localhost:8080/api/docs

# Raw OpenAPI 3.0 YAML spec
curl http://localhost:8080/api/docs/openapi.yaml
```

## Webhook Callbacks

When a task completes or fails, CodeForge sends a POST request to the `callback_url`:

```json
{
  "task_id": "550e8400-...",
  "status": "completed",
  "result": "Task completed successfully...",
  "changes_summary": {
    "files_changed": 3,
    "insertions": 45,
    "deletions": 12
  },
  "usage": {
    "input_tokens": 1500,
    "output_tokens": 500,
    "duration_seconds": 120
  },
  "trace_id": "abc123...",
  "finished_at": "2025-01-15T10:35:00Z"
}
```

Headers:
- `X-Signature-256: sha256=<hmac>` - HMAC-SHA256 of the body
- `X-CodeForge-Event: task.completed` - Event type
- `X-Trace-ID: <trace_id>` - OpenTelemetry trace ID (if tracing enabled)

### Verifying Webhook Signatures

```python
import hmac, hashlib

def verify(body: bytes, signature: str, secret: str) -> bool:
    expected = hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```
