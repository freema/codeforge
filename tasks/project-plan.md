# CodeForge — Project Plan & Task Breakdown

> **Repository:** github.com/freema/codeforge
> **Language:** Go
> **Created:** 2025-02-12
> **Author:** Tomas Grasl

---

## 1. Project Overview

CodeForge is an open-source Go HTTP server that acts as a remote code task runner. It receives tasks (repo URL + AI prompt), clones the repository, runs AI CLI tools (starting with Claude Code) against the codebase, streams progress via Redis, and returns raw text results. PR/MR creation is optional and only triggered by explicit signal from the consumer.

**Primary consumer:** ScopeBot (Next.js) — but CodeForge is generic and can serve any service needing AI-powered code operations on repositories.

### Core Flow

```
Default:    Client → CodeForge → Clone Repo → Run AI CLI → Stream Progress → Return Result
Optional:   Client → POST /tasks/:id/create-pr → CodeForge → Create Branch → Push → Open PR/MR
```

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Task input | Redis message (primary) + HTTP POST | Redis for ScopeBot (shared Redis), HTTP for other clients |
| State & queue | Redis (required) | Single store for tasks, keys, streaming, queue |
| Output streaming | Redis Pub/Sub (`task:{id}:stream`) | Full `stream-json` output from Claude Code + structured events |
| Result delivery | Combo: Redis key + Pub/Sub completion + webhook + HTTP polling | Multiple channels for different consumers |
| Realtime API | WebSocket (optional, future) | Proxy over Redis Pub/Sub for browser clients |
| Workspace | Persistent volume with TTL cleanup | Reuse for iterative tasks, auto-cleanup |
| PR/MR creation | **Only on explicit signal** — never automatic | Consumer decides based on changes_summary |
| Output format | Raw text | CodeForge returns raw text, consumer parses/stores as needed |
| CLI runners | Claude Code first, pluggable architecture | Support for multiple AI CLIs over time |
| CLI streaming | `--output-format stream-json --verbose --permission-mode bypassPermissions` | Full visibility into what Claude Code does |
| MCP servers | npx-based, dynamic registration | Global + per-project + per-task config |
| Config | Env vars + config file (YAML) | Standard for containerized Go services |
| Auth | Bearer token + registered provider keys | API keys for clients, Git provider tokens stored in Redis |

---

## 2. Architecture

### 2.1 High-Level Components

```
┌─────────────────────────────────────────────────────┐
│                    CodeForge                         │
│                                                      │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │ HTTP API │  │  Redis    │  │   Worker Pool     │  │
│  │          │  │ Listener  │  │                   │  │
│  │ POST/GET │  │          │  │  ┌─────────────┐  │  │
│  │ /tasks   │──│ BLPOP    │──│  │  Executor   │  │  │
│  │ /health  │  │ queue:   │  │  │  (clone,    │  │  │
│  │ /keys    │  │ tasks    │  │  │   run CLI,  │  │  │
│  └──────────┘  └──────────┘  │  │   stream)   │  │  │
│                              │  └─────────────┘  │  │
│  ┌──────────────────────┐    │  ┌─────────────┐  │  │
│  │  Key Registry        │    │  │  MR/PR      │  │  │
│  │  (GitHub/GitLab      │    │  │  Creator    │  │  │
│  │   tokens in Redis)   │    │  └─────────────┘  │  │
│  └──────────────────────┘    └───────────────────┘  │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │  MCP Manager (register, install, configure)  │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │  Workspace Manager (volumes, TTL, cleanup)   │    │
│  └──────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
         │                    │
         ▼                    ▼
   ┌──────────┐        ┌──────────┐
   │  Redis   │        │ Git Host │
   │  (state, │        │ (GitHub/ │
   │  stream, │        │  GitLab) │
   │  queue)  │        │          │
   └──────────┘        └──────────┘
```

### 2.2 Redis Schema

```
# Task queue
queue:tasks                    → List (RPUSH/BLPOP for FIFO ordering)

# Task state (with TTL policy)
task:{id}:state                → Hash {status, created_at, updated_at, ...}  [TTL: tasks.state_ttl, default 7d, set on terminal state]
task:{id}:result               → String (JSON result)                        [TTL: tasks.result_ttl, default 7d, set when stored]
task:{id}:stream               → Pub/Sub channel (real-time output)          [no TTL — ephemeral]
task:{id}:done                 → Pub/Sub channel (one-shot completion)       [no TTL — ephemeral]
task:{id}:history              → List (all stream messages persisted)         [TTL: tasks.workspace_ttl, default 24h]
task:{id}:iterations           → List (iteration history)                    [TTL: tasks.state_ttl, default 7d]

# Provider keys
keys:github:{name}             → Hash {encrypted_token, created_at, scope}
keys:gitlab:{name}             → Hash {encrypted_token, created_at, scope}

# MCP config (per-server keys — no race conditions)
mcp:global:{name}              → Hash {package, args_json, env_json}
mcp:global:_index              → Set (server names for listing)
mcp:project:{repo_url}:{name}  → Hash (per-project overrides)

# Redis input (ScopeBot → CodeForge)
input:tasks                    → List (RPUSH by external client, BLPOP by CodeForge)
input:result:{correlation_id}  → String (task ID response, EX 300)

# Rate limiting
ratelimit:{token_hash}         → ZSET (sliding window timestamps)

# Workspace tracking
workspace:{task_id}             → Hash {path, created_at, ttl, size_bytes}
```

### 2.3 Task Lifecycle

**Default flow (read-only, no PR):**
```
PENDING → CLONING → RUNNING → COMPLETED
                       ↓
                     FAILED
```

**With PR creation (on explicit signal):**
```
COMPLETED → CREATING_PR → PR_CREATED
                ↓
              FAILED
```

**Iterative flow (follow-up instructions):**
```
COMPLETED → AWAITING_INSTRUCTION → RUNNING → COMPLETED
                                      ↓
                                    FAILED
```

1. **PENDING** — Task received, queued
2. **CLONING** — Repository being cloned
3. **RUNNING** — AI CLI executing against codebase (streaming output)
4. **COMPLETED** — Result available, includes changes_summary
5. **FAILED** — Error at any stage
6. **AWAITING_INSTRUCTION** — Waiting for follow-up instructions (iterative flow)
7. **CREATING_PR** — Creating branch, pushing changes, opening PR/MR (only on explicit signal)
8. **PR_CREATED** — PR/MR created, URL available

> **Key change:** ANALYZING and CREATING_MR are NOT part of the default flow.
> PR/MR is never automatic. Consumer receives `changes_summary` and decides.

### 2.4 Task Payload

```json
{
  "repo_url": "https://github.com/org/repo",
  "provider_key": "my-github-key",
  "access_token": "ghp_...",
  "prompt": "Analyze this codebase and generate a knowledge base summary",
  "callback_url": "https://scopebot.example.com/webhooks/codeforge",
  "config": {
    "timeout_seconds": 600,
    "cli": "claude-code",
    "ai_model": "claude-sonnet-4-20250514",
    "ai_api_key": "sk-ant-...",
    "max_turns": 10,
    "target_branch": "main",
    "mcp_servers": [
      { "name": "custom-server", "package": "@org/mcp-server", "args": ["--flag"] }
    ]
  }
}
```

> **Token resolution order:** `access_token` (inline) → `provider_key` (registry lookup) → env var fallback (`GITHUB_TOKEN`/`GITLAB_TOKEN`)

### 2.5 Task Result

```json
{
  "task_id": "uuid",
  "status": "completed",
  "result": "raw text output from AI CLI...",
  "changes_summary": {
    "files_modified": 3,
    "files_created": 1,
    "files_deleted": 0,
    "diff_stats": "+142 -38"
  },
  "usage": {
    "input_tokens": 15000,
    "output_tokens": 4200,
    "duration_seconds": 45
  },
  "finished_at": "2025-02-12T..."
}
```

> **Consumer decides** what to do: store as knowledge doc, request PR creation, discard, etc.

### 2.6 Stream Event Categories

All events published to `task:{id}:stream` Redis Pub/Sub:

```json
{"type": "system",  "event": "task_started",      "data": {...}, "ts": "..."}
{"type": "system",  "event": "workspace_created",  "data": {"path": "..."}, "ts": "..."}
{"type": "git",     "event": "clone_started",      "data": {"repo_url": "..."}, "ts": "..."}
{"type": "git",     "event": "clone_completed",    "data": {"duration_ms": 3400}, "ts": "..."}
{"type": "cli",     "event": "cli_started",        "data": {"tool": "claude-code", "model": "..."}, "ts": "..."}
{"type": "cli",     "event": "tool_call",          "data": {"tool": "Read", "file": "..."}, "ts": "..."}
{"type": "stream",  "event": "output",             "data": {"raw": {...}}, "ts": "..."}
{"type": "cli",     "event": "cli_completed",      "data": {"exit_code": 0, "duration_ms": 45000}, "ts": "..."}
{"type": "result",  "event": "task_completed",     "data": {"changes_summary": {...}}, "ts": "..."}
```

**Event groups:**
| Group | What it contains |
|-------|-----------------|
| `system` | CodeForge internal events (task lifecycle, workspace, errors) |
| `git` | Git operations (clone, branch, push, PR creation) |
| `cli` | Claude Code lifecycle (started, tool calls, completed) |
| `stream` | Raw Claude Code `stream-json` output (every line) |
| `result` | Final result events (completed, failed, changes_summary) |

### 2.7 Delivery Channels

| Channel | When | Who | Phase |
|---------|------|-----|-------|
| Redis `task:{id}:result` key | Always — final result stored | Any Redis consumer | v0.2.0 |
| Redis Pub/Sub `task:{id}:stream` | Real-time — CLI output + events | ScopeBot (progress) | v0.2.0 |
| Redis Pub/Sub `task:{id}:done` | One-shot completion signal | ScopeBot (don't need to poll) | v0.2.0 |
| Webhook POST to callback_url | Push notification with HMAC | HTTP clients without Redis | v0.2.0 |
| HTTP `GET /tasks/:id` | Polling fallback | Any client, debug | v0.2.0 |
| WebSocket | Live stream for browsers | Admin UI (future) | v0.4.0+ |

---

## 3. API Endpoints

### 3.1 Tasks

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/tasks` | Submit new task (HTTP input) |
| `GET` | `/api/v1/tasks/:id` | Get task status + result + changes_summary |
| `POST` | `/api/v1/tasks/:id/instruct` | Send follow-up instruction |
| `POST` | `/api/v1/tasks/:id/cancel` | Cancel running task |
| `POST` | `/api/v1/tasks/:id/create-pr` | **Explicit PR/MR creation** (only when consumer requests it) |
| `GET` | `/api/v1/tasks/:id/stream` | WebSocket stream (future) |

### 3.2 Keys (Provider Token Registry)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/keys` | Register a GitHub/GitLab token |
| `GET` | `/api/v1/keys` | List registered keys (names only) |
| `DELETE` | `/api/v1/keys/:name` | Remove a registered key |

### 3.3 MCP Servers

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/mcp/servers` | Register MCP server (global) |
| `GET` | `/api/v1/mcp/servers` | List registered MCP servers |
| `DELETE` | `/api/v1/mcp/servers/:name` | Remove MCP server |
| `PUT` | `/api/v1/mcp/projects/:repo` | Set per-project MCP config |

### 3.4 System

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (Redis ping, worker status) |
| `GET` | `/ready` | Readiness check |

---

## 4. Configuration

```yaml
# codeforge.yaml
server:
  port: 8080
  auth_token: "${CODEFORGE_AUTH_TOKEN}"

redis:
  url: "${REDIS_URL}"
  password: "${REDIS_PASSWORD}"
  db: 0
  prefix: "codeforge:"

workers:
  concurrency: 3
  queue_name: "queue:tasks"

tasks:
  default_timeout: 300
  max_timeout: 1800
  workspace_ttl: 86400
  workspace_base: "/data/workspaces"
  state_ttl: 604800          # 7 days — TTL for task:{id}:state and task:{id}:result
  result_ttl: 604800         # 7 days — TTL for task:{id}:result
  disk_warning_threshold_gb: 10
  disk_critical_threshold_gb: 20

cli:
  default: "claude-code"
  claude_code:
    path: "claude"
    version: ""                # pinned CLI version (empty = use installed)
    default_model: "claude-sonnet-4-20250514"

git:
  branch_prefix: "codeforge/"
  commit_author: "CodeForge Bot"
  commit_email: "codeforge@noreply"
  provider_domains: {}         # custom domains: {"git.company.com": "gitlab"}

encryption:
  key: "${CODEFORGE_ENCRYPTION_KEY}"  # 32 bytes, base64-encoded (required)

mcp:
  global_servers: []

webhooks:
  hmac_secret: "${CODEFORGE_HMAC_SECRET}"
  retry_count: 3
  retry_delay: 5s

rate_limit:
  tasks_per_minute: 10         # per Bearer token
  enabled: true

tracing:
  enabled: false
  exporter: "otlp"             # "otlp" | "jaeger"
  endpoint: ""                 # e.g., "localhost:4317"
  sampling_rate: 0.1

logging:
  level: "info"
  format: "json"
```

---

## 5. Directory Structure

```
codeforge/
├── cmd/
│   └── codeforge/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── server/
│   │   ├── server.go
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── logging.go
│   │   │   └── recovery.go
│   │   └── handlers/
│   │       ├── tasks.go
│   │       ├── keys.go
│   │       ├── mcp.go
│   │       └── health.go
│   ├── task/
│   │   ├── model.go
│   │   ├── service.go
│   │   ├── queue.go
│   │   └── state.go
│   ├── worker/
│   │   ├── pool.go
│   │   ├── executor.go
│   │   └── stream.go
│   ├── git/
│   │   ├── clone.go
│   │   ├── branch.go
│   │   ├── github.go
│   │   └── gitlab.go
│   ├── cli/
│   │   ├── runner.go
│   │   ├── claude.go
│   │   └── analyzer.go
│   ├── mcp/
│   │   ├── manager.go
│   │   ├── registry.go
│   │   └── installer.go
│   ├── keys/
│   │   ├── registry.go
│   │   └── crypto.go
│   ├── workspace/
│   │   ├── manager.go
│   │   └── cleanup.go
│   ├── webhook/
│   │   └── sender.go
│   └── redis/
│       ├── client.go
│       └── pubsub.go
├── configs/
│   └── codeforge.example.yaml
├── deployments/
│   ├── Dockerfile
│   ├── Dockerfile.dev
│   ├── docker-compose.yaml
│   └── docker-compose.dev.yaml
├── .github/
│   └── workflows/
│       ├── ci.yaml
│       └── release.yaml
├── Taskfile.yaml
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

---

## 6. Phased Delivery Overview

| Phase | Version | Focus |
|-------|---------|-------|
| 0 | v0.1.0 | Project Scaffold — repo, CI/CD, Docker, HTTP skeleton |
| 1 | v0.2.0 | Core Task Runner — submit, clone, run, stream, callback |
| 2 | v0.3.0 | Git Integration — branches, PR/MR creation |
| 3 | v0.4.0 | Iterative Flow — follow-up instructions, workspace reuse |
| 4 | v0.5.0 | Key Registry & MCP Management |
| 5 | v0.6.0 | Workspace Management — TTL, cleanup, monitoring |
| 6 | v1.0.0 | Production Hardening — metrics, tracing, security, docs |

See individual phase files for detailed task breakdowns:
- [Phase 0 — Scaffold](./phase-0-scaffold.md)
- [Phase 1 — Core Task Runner](./phase-1-core.md)
- [Phase 2 — Git Integration](./phase-2-git-integration.md)
- [Phase 3 — Iterative Flow](./phase-3-iterative-flow.md)
- [Phase 4 — Keys & MCP](./phase-4-keys-mcp.md)
- [Phase 5 — Workspace](./phase-5-workspace.md)
- [Phase 6 — Hardening](./phase-6-hardening.md)
- [Testing Strategy](./testing-strategy.md)

> **Note:** Task files are in the `tasks/` directory.

---

## 7. Docker & Development

**All development happens inside Docker — no local Go/npm required.**

```
task dev              # start dev env (hot reload + Redis)
task test             # run tests inside Docker
task lint             # run linter inside Docker
task build            # build production image
task test:integration # integration tests with Redis
task down             # stop everything
task logs             # tail logs
task redis:cli        # open redis-cli
```

Two Dockerfiles:
- `deployments/Dockerfile` — production (multi-stage, minimal runtime)
- `deployments/Dockerfile.dev` — development (Go + air hot reload + golangci-lint + Claude CLI)

Two Compose files:
- `deployments/docker-compose.yaml` — base (production-like)
- `deployments/docker-compose.dev.yaml` — dev overlay (source mount, debug logging, Go caches)

---

## 8. Security Considerations

- **Bearer token auth** on all API endpoints (except `/health`)
- **HMAC-SHA256** signed webhook callbacks
- **AES-256-GCM** encryption for stored Git provider tokens
- **No shell injection** — all CLI execution via `exec.Command` with explicit args
- **Workspace isolation** — each task gets its own directory, cleanup on TTL
- **Token scoping** — registered keys use minimum required permissions
- **Redis auth** — password-protected Redis connection
- **Rate limiting** — prevent abuse of task submission

---

## 9. Open Questions & Future Ideas

1. WebSocket proxy for browser clients over Redis Pub/Sub?
2. Multi-model support — different API keys per model?
3. Task priorities via Redis sorted sets?
4. Repo caching — `git fetch` instead of fresh clone for same repo?
5. Slack/Discord notifications for task completion?
6. Admin UI for monitoring?
7. Multi-container workers — scale independently from API server?

---

## 10. Decisions Log

Decisions made during planning sessions:

| Date | Decision | Context |
|------|----------|---------|
| 2025-02-12 | Redis is primary communication channel (shared with ScopeBot), HTTP secondary | ScopeBot will add Redis; both apps share same Redis instance |
| 2025-02-12 | PR/MR never automatic — only on explicit signal via `POST /tasks/:id/create-pr` | Primary use case (knowledge docs) is read-only; consumer sees changes_summary and decides |
| 2025-02-12 | Output is raw text — CodeForge doesn't structure results | Consumer (ScopeBot) parses and stores in its own schema (e.g., knowledgeBlock) |
| 2025-02-12 | Combo delivery: Redis Pub/Sub + result key + done signal + webhook + HTTP polling | Different consumers need different channels |
| 2025-02-12 | Full CLI streaming via `--output-format stream-json --verbose` | Everything Claude Code does (tool calls, output) streamed to Redis |
| 2025-02-12 | Stream events grouped into categories: system, git, cli, stream, result | Clean separation for consumers to filter what they need |
| 2025-02-12 | Sensitive fields (AccessToken, AIApiKey) stored ENCRYPTED in Redis (AES-256-GCM) | Must survive runner restart; never returned in API responses; reuses CryptoService from Task 4.2 |
| 2025-02-12 | Git auth via GIT_ASKPASS, NOT URL-embedded tokens | Token never stored in .git/config — can't leak via workspace inspection |
| 2025-02-12 | Validation via go-playground/validator/v10 | Industry standard for Go struct validation, used in HTTP + Redis input |
| 2025-02-12 | MCP servers launched by Claude Code from .mcp.json | CodeForge only generates the config file; Claude Code manages MCP lifecycle |
| 2025-02-12 | MCP registry: per-server Redis keys (not JSON array) | Prevents race conditions on concurrent MCP server registration |
| 2025-02-12 | Task data TTL: state/result 7d, history 24h, no TTL while running | Prevents unbounded Redis growth |
| 2025-02-12 | Sliding window rate limiting (Redis ZSET) | Simpler than token bucket, atomic via pipeline, accurate per-window counting |
| 2025-02-12 | Iteration summary = truncated raw result (first 2000 chars) | Simple, fast — no extra AI call for summarization |
| 2025-02-12 | Claude Code CLI version pinned in Dockerfile | Prevents breaking changes from CLI updates |
| 2025-02-12 | Taskfile.yaml (go-task) instead of Makefile | YAML-based, readable, task dependencies |
| 2025-02-12 | All development inside Docker — no local Go/npm | Consistent env, Dockerfile.dev + air hot reload, source mounted as volume |
