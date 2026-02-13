# API Reference

All endpoints require `Authorization: Bearer <token>` unless noted otherwise.

Full OpenAPI 3.0 spec: [`api/openapi.yaml`](../api/openapi.yaml)

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
