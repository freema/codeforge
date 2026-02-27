# Architecture

## System Overview

CodeForge is a Go HTTP server that orchestrates AI-powered code tasks. It receives task requests via REST API, clones repositories, runs AI CLI tools against them, and optionally creates pull requests with the changes. It supports multiple CLI runners (Claude Code, Codex), code review as a user action, and a tool system for extending AI capabilities.

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
                                    │          │   │(Claude/  │   │ Callback │
                                    └──────────┘   │ Codex)   │   └──────────┘
                                                   └──────────┘
```

## Key Components

### HTTP Server (`internal/server/`)
- Chi router with middleware (auth, logging, rate limiting, metrics, tracing)
- Handlers for tasks, keys, MCP servers, tools, workspaces, workflows, and SSE streams
- Swagger UI at `/api/docs` with embedded OpenAPI spec
- Prometheus `/metrics` and health endpoints (no auth required)
- SSE stream endpoint bypasses `otelhttp` and request timeout middleware (see Streaming below)

### Task Service (`internal/task/`)
- CRUD operations on task state stored in Redis hashes
- State machine with validated transitions (see Task Lifecycle below)
- FIFO queue via `RPUSH`/`BLPOP`
- Iteration tracking for multi-turn conversations
- PR service for commit/push/PR creation flow
- Review lifecycle methods (`StartReview`, `CompleteReview`)

### Worker Pool (`internal/worker/`)
- Configurable concurrency (N goroutines)
- Each worker polls Redis queue with `BLPOP` (5s timeout)
- Per-task cancellable contexts for cancel support
- Executor orchestrates: clone -> run CLI -> diff -> report

### CLI Runner (`internal/tool/runner/`)
- `Runner` interface for pluggable AI tools
- **Claude Code** runner: `--output-format stream-json` parsing, supports MaxTurns and MaxBudgetUSD
- **Codex** runner: JSONL stream parsing (`--json --full-auto`), `CODEX_API_KEY` env var
- Registry maps CLI names to Runner implementations
- Selected per-task via `config.cli` field (default: `claude-code`)
- Result extraction: prefers the `type: "result"` event text; falls back to the last `type: "assistant"` message text

### Stream Normalization (`internal/worker/normalizer.go`)
- Converts CLI-specific events into a common stream format
- Normalized event types: `thinking`, `text`, `tool_use`, `tool_result`, `result`, `error`, `system`
- Both Claude Code and Codex output gets normalized before being sent to SSE clients
- FE consumers only need to handle normalized event types

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

### Review System (`internal/review/`)
- **User-triggered action**, NOT an automatic step — user calls `POST /tasks/:id/review`
- `Reviewer` service orchestrates: validate state -> run CLI review -> parse output -> store result
- `TaskProvider` interface abstracts task service + workspace resolution
- Multi-strategy parser: direct JSON, markdown code block, heuristic brace matching, fallback
- Review result stored on `Task.ReviewResult` field in Redis
- Supports different CLI for review (e.g. Codex reviews Claude Code's output)

### Tool System (`internal/tools/`)
- High-level abstraction over MCP servers — users request tools by name, system handles MCP wiring
- **Registry** — SQLite-backed storage with scope (global / project-level)
- **Catalog** — 5 built-in tools: sentry, jira, git, github, playwright
- **Resolver** — lookup chain: project scope -> global scope -> built-in catalog
- **Bridge** — converts resolved tools to MCP server configs for `.mcp.json` generation
- **Validator** — checks required config fields before task execution
- Per-task tool requests via `TaskConfig.Tools` field

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
- Embedded SQLite database for persistent storage of workflow definitions, workflow runs, keys, tools, and MCP server configs
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
   │           │           │           │  ▲                           │
   │           │           │           │  │                           │
   │           │           │           ▼  │                           │
   │           │           │       reviewing                         │
   │           │           │                                         │
   │           │           │      awaiting_instruction ◀─────────────┘
   │           │           │           │  ▲
   │           │           │           │  │
   │           │           │           ▼  │
   │           │           │        running (iteration N)
   │           │           │
   ▼           ▼           ▼
 failed      failed      failed
```

### States

- **pending**: Task created, queued for processing
- **cloning**: Git repository being cloned
- **running**: AI CLI executing the prompt
- **completed**: CLI finished, results available — user can now review, instruct, or create PR
- **reviewing**: Code review in progress (user-triggered via `POST /tasks/:id/review`)
- **failed**: Terminal state (clone/run/timeout/cancel failure)
- **awaiting_instruction**: Waiting for follow-up prompt (after `POST /tasks/:id/instruct`)
- **creating_pr**: PR/MR being created
- **pr_created**: PR/MR created successfully

### Valid Transitions

| From | To |
|------|----|
| pending | cloning, failed |
| cloning | running, failed |
| running | completed, failed |
| completed | awaiting_instruction, creating_pr, reviewing |
| reviewing | completed, failed |
| awaiting_instruction | running, reviewing, failed |
| creating_pr | pr_created, failed |
| pr_created | awaiting_instruction, completed |

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
cmd/codeforge/          # Application entrypoint + review adapter
internal/
  apperror/             # Application error types
  config/               # Configuration loading (koanf)
  crypto/               # AES-256-GCM encryption
  database/             # SQLite wrapper + migrations
  keys/                 # Access key registry + resolver
  logger/               # Structured logging (slog)
  metrics/              # Prometheus metric definitions
  prompt/               # Prompt templates (embed FS, code review)
  redisclient/          # Redis client wrapper
  review/               # Code review service (reviewer, parser, models)
  server/               # HTTP server + handlers + middleware
  task/                 # Task model, service, state machine
  tool/                 # Tool subsystem namespace (low-level)
    git/                # Git operations (clone, branch, PR)
    runner/             # CLI runner interface + implementations (Claude, Codex)
    mcp/                # MCP server registry + installer
  tools/                # Tool system (high-level: catalog, registry, resolver, bridge)
  tracing/              # OpenTelemetry setup
  webhook/              # Webhook sender with HMAC + retries
  worker/               # Worker pool, executor, streamer, normalizer
  workflow/             # Workflow orchestrator, step executors, templates
  workspace/            # Workspace manager + cleanup
api/                    # OpenAPI specification
deployments/            # Docker, docker-compose files
tests/                  # Integration tests, mock CLI
```
