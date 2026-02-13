# Architecture

## System Overview

CodeForge is a Go HTTP server that orchestrates AI-powered code tasks. It receives task requests via REST API, clones repositories, runs AI CLI tools against them, and optionally creates pull requests with the changes.

```
Client (ScopeBot)
    │
    ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  HTTP Server │────▶│  Redis Queue  │────▶│  Worker Pool │
│  (Chi)       │     │  (BLPOP)      │     │  (N workers) │
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                  │
                                    ┌─────────────┼─────────────┐
                                    ▼             ▼             ▼
                              ┌──────────┐ ┌──────────┐ ┌──────────┐
                              │ Git Clone│ │ CLI Run  │ │ Webhook  │
                              │          │ │ (Claude) │ │ Callback │
                              └──────────┘ └──────────┘ └──────────┘
```

## Key Components

### HTTP Server (`internal/server/`)
- Chi router with middleware (auth, logging, rate limiting, metrics, tracing)
- Handlers for tasks, keys, MCP servers, and workspaces
- Prometheus `/metrics` and health endpoints (no auth required)

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

### CLI Registry (`internal/cli/`)
- `Runner` interface for pluggable AI tools
- `ClaudeRunner` implements stream-json output parsing
- Registry maps CLI names to Runner implementations
- Selected per-task via `config.cli` field

### Streaming (`internal/worker/stream.go`)
- Events published to Redis Pub/Sub channels (`task:{id}:stream`)
- Dual-write to history list (`task:{id}:history`) for reconnection
- Event types: system, git, cli, stream, result
- Done signal on separate channel (`task:{id}:done`)

### Git Integration (`internal/git/`)
- Clone with `GIT_ASKPASS` for token auth (never in URL or .git/config)
- Provider detection from URL (GitHub, GitLab, custom domains)
- PR creation via GitHub/GitLab APIs
- Branch management, diff calculation

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
- `codeforge_workspace_disk_usage_bytes` (gauge) - disk usage

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
  cli/                  # CLI runner interface + implementations
  config/               # Configuration loading (koanf)
  crypto/               # AES-256-GCM encryption
  git/                  # Git operations (clone, branch, PR)
  keys/                 # Access key registry + resolver
  logger/               # Structured logging (slog)
  mcp/                  # MCP server registry + installer
  metrics/              # Prometheus metric definitions
  redisclient/          # Redis client wrapper
  server/               # HTTP server + handlers + middleware
  task/                 # Task model, service, state machine
  tracing/              # OpenTelemetry setup
  webhook/              # Webhook sender with HMAC + retries
  worker/               # Worker pool, executor, streamer
  workspace/            # Workspace manager + cleanup
api/                    # OpenAPI specification
deployments/            # Docker, docker-compose files
tests/                  # Integration tests, mock CLI
```
