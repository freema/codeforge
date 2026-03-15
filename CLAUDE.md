# CodeForge — Developer Guide

## Overview

CodeForge is a Go HTTP server that orchestrates AI-powered code work over git repositories. A task is a **session over a repo** (not a one-shot job) — it tracks workspace, conversation history, review results, and PR state. Supports multi-turn conversations, automated PR reviews, webhook triggers, tool integration, and PR creation.

## Development Setup

**All development happens inside Docker — no local Go/npm required.**

```bash
# Install go-task: https://taskfile.dev/installation/
# Install Docker + Docker Compose

# Start dev environment (hot reload)
task dev

# Run in background
task dev:detach
```

## Commands (Taskfile.yaml)

```bash
task dev              # Start dev env with hot reload (CodeForge + Redis)
task down             # Stop containers
task down:clean       # Stop containers + remove volumes
task build            # Build production Docker image
task test             # Run unit tests
task test:integration # Run integration tests (needs Redis)
task lint             # Run golangci-lint
task fmt              # Run gofmt
task logs             # Tail all logs
task shell            # Open shell in container
task redis:cli        # Open redis-cli
task mod:tidy         # Run go mod tidy
task build:action     # Build CI Action Docker image
```

## Project Structure

```
cmd/codeforge/         Server entry point
cmd/codeforge-action/  CI Action entry point (GitHub Action / GitLab CI)
internal/
  apperror/            Application error types
  config/              Configuration (koanf, YAML + env vars)
  crypto/              AES-256-GCM encryption
  database/            SQLite wrapper + migrations
  keys/                Key registry + resolver
  logger/              Structured logging (slog)
  metrics/             Prometheus metrics
  prompt/              Prompt templates (embed FS, task types + code/PR review)
  redisclient/         Redis client wrapper
  review/              Code review types (models, parser, formatting)
  server/              HTTP server (Chi router)
    handlers/          Request handlers (tasks, webhook receiver, stream, etc.)
    middleware/        Auth, logging, recovery, rate limit
  task/                Task model, service, state machine
  tool/                Tool subsystem namespace
    git/               Clone, branch, GitHub/GitLab PR, review posting
    runner/            AI CLI runner interface + implementations (Claude, Codex)
    mcp/               MCP server registry + installer
  tools/               Tool system (catalog, registry, resolver, bridge)
  tracing/             OpenTelemetry setup
  webhook/             HMAC-signed webhook callbacks
  worker/              Worker pool, executor, streaming, normalizer
  workflow/            Workflow orchestrator, step executors, templates
  workspace/           Workspace lifecycle, cleanup
api/                   OpenAPI specification
configs/               Example config files
deployments/           Dockerfiles, docker-compose
tests/                 Integration + E2E tests
tasks/                 Planning documents (not code)
```

## Conventions

- **Go 1.24+**, standard library preferred where possible
- **Structured logging** via `log/slog` (JSON in production, text in dev)
- **Error handling**: return errors, don't panic; use typed errors from `internal/apperror`
- **Testing**: table-driven tests, `_test.go` next to source files
- **Config**: koanf with YAML + env var override (`CODEFORGE_` prefix, `__` for nested)
- **Redis keys**: prefixed with `codeforge:` in production
- **No shell injection**: all CLI via `exec.Command` with explicit args
- **Sensitive fields**: encrypted in Redis (AES-256-GCM), never in API responses (`json:"-"`)
- **Git auth**: GIT_ASKPASS helper, never URL-embedded tokens
- **Multi-CLI**: per-task CLI selection via `config.cli` field (claude-code, codex)
- **Review as action**: user triggers review via `POST /tasks/:id/review`, not automatic
- **PR review is a task**: `pr_review` task type reuses the entire task system, no separate infrastructure

## Architecture

- **HTTP API**: Chi router at `/api/v1/`
- **Task queue**: Redis RPUSH + BLPOP (FIFO)
- **Streaming**: Redis Pub/Sub `task:{id}:stream` + SSE
- **State**: Redis hashes `task:{id}:state`
- **Persistence**: SQLite for workflows, tools, keys, MCP configs
- **Worker pool**: configurable concurrency, graceful shutdown
- **Task lifecycle**: pending → cloning → running → completed (+ reviewing, awaiting_instruction, creating_pr, pr_created, failed)

## Key Flows

1. **Create task** → clone repo → run AI CLI → stream progress → store result
2. **Stream** → SSE with history replay + live events
3. **Instruct** → follow-up turn in same workspace (multi-turn)
4. **Review task** → AI reviews task's changes (user-triggered action)
5. **PR review** → `pr_review` task type reviews PR/MR diff, optionally posts comments
6. **Webhook review** → GitHub/GitLab webhooks auto-create `pr_review` tasks
7. **Post review** → post ReviewResult as PR/MR comments
8. **Create PR/MR** → push changes + open PR from task workspace
9. **Workflows** → multi-step fetch → task → action pipelines

## Design Philosophy

1. Backend orchestrator for AI work over git — not a chat app
2. Task = session over a repo — stateful workspace with conversation history
3. Queue-first execution — Redis FIFO, worker pool, state machine, SSE
4. Human-in-the-loop — review, instruct, create PR at any point
5. Two integration axes — provider data (GitHub/GitLab/Sentry) + MCP tools
6. Workflow layer composes multi-step scenarios on top of task runtime
