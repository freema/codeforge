# CodeForge

Remote AI code task runner. Submit code tasks via HTTP API, monitor progress through Redis Pub/Sub streaming, and create pull requests from AI-generated changes.

## Features

- **Task lifecycle management** with 8-state machine (pending, cloning, running, completed, failed, awaiting_instruction, creating_pr, pr_created)
- **Multi-turn conversations** with iteration tracking and conversation context
- **Git integration** with automatic PR/MR creation for GitHub and GitLab
- **Real-time streaming** via Redis Pub/Sub with event history
- **Multi-CLI support** via pluggable Runner interface (Claude Code out of the box)
- **Key registry** with AES-256-GCM encrypted storage and 3-tier token resolution
- **MCP server management** with per-project and global configuration
- **Workspace management** with TTL cleanup and disk monitoring
- **Prometheus metrics** at `/metrics`
- **OpenTelemetry tracing** with OTLP export
- **Rate limiting** per Bearer token (Redis sliding window)
- **Webhook callbacks** with HMAC-SHA256 signatures and exponential backoff

## Quick Start

### Prerequisites

- Docker and Docker Compose
- [Task](https://taskfile.dev/) (optional, for convenience commands)

### Run with Docker Compose

```bash
# Start development environment
task dev

# Or without Task:
docker compose -f deployments/docker-compose.yaml -f deployments/docker-compose.dev.yaml up --build
```

The server starts at `http://localhost:8080`.

### Create a Task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "access_token": "ghp_your_token",
    "prompt": "Fix the failing tests in the auth module"
  }'
```

### Check Task Status

```bash
curl http://localhost:8080/api/v1/tasks/{task_id} \
  -H "Authorization: Bearer dev-token"
```

### Create a Pull Request

```bash
curl -X POST http://localhost:8080/api/v1/tasks/{task_id}/create-pr \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"target_branch": "main"}'
```

## API Reference

Full OpenAPI 3.0 spec available at [`api/openapi.yaml`](api/openapi.yaml).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness probe |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/api/v1/tasks` | Create a task |
| `GET` | `/api/v1/tasks/{id}` | Get task status |
| `POST` | `/api/v1/tasks/{id}/instruct` | Send follow-up instruction |
| `POST` | `/api/v1/tasks/{id}/cancel` | Cancel a running task |
| `POST` | `/api/v1/tasks/{id}/create-pr` | Create PR from changes |
| `POST` | `/api/v1/keys` | Register access key |
| `GET` | `/api/v1/keys` | List keys (tokens redacted) |
| `DELETE` | `/api/v1/keys/{name}` | Delete a key |
| `POST` | `/api/v1/mcp/servers` | Register MCP server |
| `GET` | `/api/v1/mcp/servers` | List MCP servers |
| `DELETE` | `/api/v1/mcp/servers/{name}` | Delete MCP server |
| `GET` | `/api/v1/workspaces` | List workspaces |
| `DELETE` | `/api/v1/workspaces/{id}` | Delete workspace |

## Configuration

Configuration is loaded in order: defaults -> YAML file -> environment variables.

Set `CODEFORGE_CONFIG` to specify a YAML config file path, or use environment variables with the `CODEFORGE_` prefix (double underscore `__` for nesting).

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_SERVER__PORT` | `8080` | HTTP server port |
| `CODEFORGE_SERVER__AUTH_TOKEN` | (required) | Bearer token for API auth |
| `CODEFORGE_REDIS__URL` | (required) | Redis connection URL |
| `CODEFORGE_REDIS__PREFIX` | `codeforge:` | Redis key prefix |
| `CODEFORGE_ENCRYPTION__KEY` | (required) | Base64-encoded 32-byte AES key |
| `CODEFORGE_WORKERS__CONCURRENCY` | `3` | Number of worker goroutines |
| `CODEFORGE_WORKERS__QUEUE_NAME` | `queue:tasks` | Redis queue name |
| `CODEFORGE_TASKS__DEFAULT_TIMEOUT` | `300` | Default task timeout (seconds) |
| `CODEFORGE_TASKS__MAX_TIMEOUT` | `1800` | Maximum task timeout (seconds) |
| `CODEFORGE_TASKS__WORKSPACE_BASE` | `/data/workspaces` | Workspace directory |
| `CODEFORGE_TASKS__WORKSPACE_TTL` | `86400` | Workspace TTL (seconds) |
| `CODEFORGE_TASKS__STATE_TTL` | `604800` | Task state TTL (seconds) |
| `CODEFORGE_CLI__DEFAULT` | `claude-code` | Default CLI tool |
| `CODEFORGE_CLI__CLAUDE_CODE__PATH` | `claude` | Claude Code binary path |
| `CODEFORGE_CLI__CLAUDE_CODE__DEFAULT_MODEL` | `claude-sonnet-4-20250514` | Default AI model |
| `CODEFORGE_GIT__BRANCH_PREFIX` | `codeforge/` | PR branch prefix |
| `CODEFORGE_GIT__COMMIT_AUTHOR` | `CodeForge Bot` | Git commit author |
| `CODEFORGE_WEBHOOKS__HMAC_SECRET` | | HMAC secret for webhook signatures |
| `CODEFORGE_WEBHOOKS__RETRY_COUNT` | `3` | Webhook retry attempts |
| `CODEFORGE_RATE_LIMIT__ENABLED` | `true` | Enable rate limiting |
| `CODEFORGE_RATE_LIMIT__TASKS_PER_MINUTE` | `10` | Rate limit per token |
| `CODEFORGE_TRACING__ENABLED` | `false` | Enable OpenTelemetry tracing |
| `CODEFORGE_TRACING__ENDPOINT` | | OTLP collector endpoint |
| `CODEFORGE_TRACING__SAMPLING_RATE` | `0.1` | Trace sampling rate (0-1) |
| `CODEFORGE_LOGGING__LEVEL` | `info` | Log level (debug/info/warn/error) |
| `CODEFORGE_LOGGING__FORMAT` | `json` | Log format (json/text) |

## Development

```bash
# Start dev environment with hot reload
task dev

# Run unit tests
task test

# Run integration tests (requires running dev environment)
task test:integration

# Run all tests
task test:all

# Build production image
task build

# Open Redis CLI
task redis:cli

# View logs
task logs
```

## Architecture

See [docs/architecture.md](docs/architecture.md) for system design, Redis schema, and task lifecycle details.

## License

Proprietary.
