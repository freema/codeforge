# Architecture

## System Overview

CodeForge is a Go HTTP server that orchestrates AI-powered code tasks. It receives task requests via REST API, clones repositories, runs AI CLI tools against them, and optionally creates pull requests with the changes.

```
Client (ScopeBot / curl)
    │
    ├── POST /tasks ──────▶ ┌──────────────┐     ┌──────────────┐
    │                       │  HTTP Server │────▶│  Redis Queue  │
    │                       │  (Chi)       │     │  (BLPOP)      │
    │                       └──────┬───────┘     └──────┬───────┘
    │                              │                     │
    │                              │               ┌─────▼───────┐
    └── GET /tasks/{id}/stream ──▶ │               │ Worker Pool │
                                   │               │ (N workers) │
                    ┌──────────────┘               └──────┬──────┘
                    ▼                                      │
             ┌──────────────┐             ┌───────────────┼───────────────┐
             │  SSE Handler │◀── Pub/Sub ─┤               │               │
             │  (stream.go) │             ▼               ▼               ▼
             └──────────────┘       ┌──────────┐   ┌──────────┐   ┌──────────┐
                                    │ Git Clone│   │ CLI Run  │   │ Webhook  │
                                    │          │   │ (Claude) │   │ Callback │
                                    └──────────┘   └──────────┘   └──────────┘
```

## Key Components

### HTTP Server (`internal/server/`)
- Chi router with middleware (auth, logging, rate limiting, metrics, tracing)
- Handlers for tasks, keys, MCP servers, workspaces, and SSE streams
- Swagger UI at `/api/docs` with embedded OpenAPI spec
- Prometheus `/metrics` and health endpoints (no auth required)
- SSE stream endpoint bypasses `otelhttp` and request timeout middleware (see Streaming below)

### Task Service (`internal/task/`)
- CRUD operations on task state stored in Redis hashes
- State machine with validated transitions
- FIFO queue via `RPUSH`/`BLPOP`
- Iteration tracking for multi-turn conversations
- PR service for commit/push/PR creation flow

### Worker Pool (`internal/worker/`)
- Configurable concurrency (N goroutines)
- Each worker polls Redis queue with `BLPOP` (5s timeout)
- Per-task cancellable contexts for cancel support
- Executor orchestrates: clone -> run CLI -> diff -> report

### CLI Runner (`internal/tool/runner/`)
- `Runner` interface for pluggable AI tools
- `ClaudeRunner` implements `--output-format stream-json` parsing
- Registry maps CLI names to Runner implementations
- Selected per-task via `config.cli` field
- Result extraction: prefers the `type: "result"` event text; falls back to the last `type: "assistant"` message text (handles cases where result has `subtype: "error_during_execution"` with an empty result field)

### Streaming

**Worker side (`internal/worker/stream.go`):**
- Events published to Redis Pub/Sub channels (`task:{id}:stream`)
- Dual-write to history list (`task:{id}:history`) for reconnection
- Event types: system, git, cli, stream, result
- Done signal on separate channel (`task:{id}:done`)

**SSE handler (`internal/server/handlers/stream.go`):**
- `GET /api/v1/tasks/{id}/stream` opens a long-lived SSE connection
- Subscribes to Redis Pub/Sub *before* reading history to avoid missed events
- Replays full history, then streams live events
- Named events: `connected`, `done`, `timeout`; keepalive comments every 15s
- For terminal tasks (completed/failed/pr_created): replays history + sends `done` immediately
- Uses `http.ResponseController` for per-write deadlines (30s) instead of global `WriteTimeout`
- Auto-closes after 10 minutes

**Middleware considerations for SSE:**
- The SSE endpoint is excluded from the `chimw.Timeout` middleware group (long-lived connection)
- `otelhttp` wraps `http.ResponseWriter` without `http.Flusher` support — SSE requests bypass `otelhttp` via path-suffix check in `server.go`
- The PrometheusMetrics middleware's `responseWriter` implements `Flush()` (delegates to underlying writer) and `Unwrap()` (for `http.ResponseController` compatibility)
- Global `http.Server.WriteTimeout` is set to `0` (disabled) — SSE handler manages its own deadlines

### Workflow System (`internal/workflow/`)
- Multi-step workflow orchestrator consuming from Redis FIFO queue (`BLPOP queue:workflows`)
- Three step types:
  - **fetch** — HTTP request to external APIs (e.g., Sentry, GitHub Issues) with JSONPath output extraction
  - **task** — creates and waits for a CodeForge task (clone + AI CLI run)
  - **action** — built-in actions (e.g., `create_pr`, `notify`) that operate on previous step results
- Go `text/template` engine for step configuration: `{{.Params.key}}`, `{{.Steps.step_name.field}}`
- Built-in workflows: `sentry-fixer`, `github-issue-fixer`
- Workflow definitions stored in SQLite (user-created + built-in, seeded on startup)
- Run state tracked in SQLite with per-step status records
- Streaming via Redis Pub/Sub (`workflow:{runID}:stream`) with history replay, same SSE pattern as tasks

```
Workflow Run Lifecycle:

pending ──▶ running ──▶ completed
   │           │
   ▼           ▼
 (queue)     failed
```

### SQLite (`internal/database/`)
- Embedded SQLite database for persistent storage of workflow definitions, workflow runs, keys, and MCP server configs
- Auto-migration on startup
- Default path: `/data/codeforge.db` (configurable via `CODEFORGE_SQLITE__PATH`)

### Git Integration (`internal/tool/git/`)
- Clone with `GIT_ASKPASS` for token auth (never in URL or .git/config)
- Provider detection from URL (GitHub, GitLab, custom domains)
- PR creation via GitHub/GitLab APIs
- Branch management, diff calculation

### MCP Server Registry (`internal/tool/mcp/`)
- Global and per-project MCP server registration
- Generates `.mcp.json` consumed by Claude Code at runtime
- Server configs stored in SQLite

### Security (`internal/crypto/`, `internal/keys/`)
- AES-256-GCM encryption for sensitive fields in Redis
- Key registry with 3-tier resolution: inline token -> registry lookup -> env var
- HMAC-SHA256 webhook signatures
- Path traversal guards on workspace deletion

## Redis Schema

All keys use configurable prefix (default: `codeforge:`).

| Key Pattern | Type | Description |
|-------------|------|-------------|
| `task:{id}` | Hash | Task state (all fields) |
| `task:{id}:stream` | Pub/Sub | Live event stream |
| `task:{id}:history` | List | Event history for reconnection |
| `task:{id}:done` | Pub/Sub | Completion signal |
| `task:{id}:iterations` | List | Iteration records (JSON) |
| `queue:tasks` | List | FIFO task queue (RPUSH/BLPOP) |
| `key:{name}` | Hash | Encrypted access key |
| `keys:index` | Set | Index of all key names |
| `mcp:global:{name}` | Hash | Global MCP server config |
| `mcp:global:index` | Set | Index of global MCP servers |
| `mcp:project:{id}:{name}` | Hash | Per-project MCP server config |
| `mcp:project:{id}:index` | Set | Per-project MCP server index |
| `workspace:{id}` | Hash | Workspace metadata |
| `workspaces:index` | Set | Index of all workspaces |
| `ratelimit:{token_hash}` | Sorted Set | Sliding window rate limit |
| `input:tasks` | List | Redis-based task input channel |
| `queue:workflows` | List | FIFO workflow run queue (RPUSH/BLPOP) |
| `workflow:{runID}:stream` | Pub/Sub | Live workflow event stream |
| `workflow:{runID}:history` | List | Workflow event history for reconnection |
| `workflow:{runID}:done` | Pub/Sub | Workflow run completion signal |
| `workflow:{runID}:context` | Hash | Step outputs for template interpolation |

## Task Lifecycle

```
pending ──▶ cloning ──▶ running ──▶ completed ──▶ creating_pr ──▶ pr_created
   │           │           │           │                              │
   │           │           │           ▼                              │
   │           │           │    awaiting_instruction ◀────────────────┘
   │           │           │           │
   ▼           ▼           ▼           ▼
 failed      failed      failed      failed
```

- **pending**: Task created, queued for processing
- **cloning**: Git repository being cloned
- **running**: AI CLI executing the prompt
- **completed**: CLI finished, results available
- **failed**: Terminal state (clone/run/timeout/cancel failure)
- **awaiting_instruction**: Waiting for follow-up prompt
- **creating_pr**: PR/MR being created
- **pr_created**: PR/MR created successfully

## Observability

### Prometheus Metrics
- `codeforge_tasks_total` (counter) - tasks by status
- `codeforge_tasks_duration_seconds` (histogram) - execution time
- `codeforge_tasks_in_progress` (gauge) - active tasks
- `codeforge_queue_depth` (gauge) - queue size
- `codeforge_workers_active/total` (gauge) - worker utilization
- `codeforge_http_requests_total` (counter) - HTTP requests
- `codeforge_http_request_duration_seconds` (histogram) - HTTP latency
- `codeforge_webhook_deliveries_total` (counter) - webhook outcomes

### OpenTelemetry Tracing
- Spans: `task.execute`, `task.clone`, `task.run`
- Trace ID propagated through task lifecycle and webhook headers
- Configurable sampling rate, OTLP HTTP export
- HTTP instrumentation via `otelhttp`

## Directory Structure

```
cmd/codeforge/          # Application entrypoint
internal/
  apperror/             # Application error types
  config/               # Configuration loading (koanf)
  crypto/               # AES-256-GCM encryption
  database/             # SQLite wrapper + migrations
  keys/                 # Access key registry + resolver
  logger/               # Structured logging (slog)
  metrics/              # Prometheus metric definitions
  redisclient/          # Redis client wrapper
  server/               # HTTP server + handlers + middleware
  task/                 # Task model, service, state machine
  tool/                 # Tool subsystem namespace
    git/                # Git operations (clone, branch, PR)
    runner/             # CLI runner interface + implementations
    mcp/                # MCP server registry + installer
  tracing/              # OpenTelemetry setup
  webhook/              # Webhook sender with HMAC + retries
  worker/               # Worker pool, executor, streamer
  workflow/             # Workflow orchestrator, step executors, templates
  workspace/            # Workspace manager + cleanup
api/                    # OpenAPI specification
deployments/            # Docker, docker-compose files
tests/                  # Integration tests, mock CLI
```
