# CodeForge — Developer Guide

## Overview

CodeForge is a Go HTTP server that acts as a remote code task runner. It clones repos, runs AI CLI tools (Claude Code, Codex), streams progress via Redis, and returns results. Supports multi-turn conversations, code review, tool integration, and PR creation.

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
```

## Project Structure

```
cmd/codeforge/         Main entry point + review adapter
internal/
  apperror/            Application error types
  config/              Configuration (koanf, YAML + env vars)
  crypto/              AES-256-GCM encryption
  database/            SQLite wrapper + migrations
  keys/                Key registry + resolver
  logger/              Structured logging (slog)
  metrics/             Prometheus metrics
  prompt/              Prompt templates (embed FS)
  redisclient/         Redis client wrapper
  review/              Code review service (reviewer, parser, models)
  server/              HTTP server (Chi router)
    handlers/          Request handlers
    middleware/        Auth, logging, recovery, rate limit
  task/                Task model, service, state machine
  tool/                Tool subsystem namespace
    git/               Clone, branch, GitHub/GitLab PR
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

## Architecture

- **HTTP API**: Chi router at `/api/v1/`
- **Task queue**: Redis RPUSH + BLPOP (FIFO)
- **Streaming**: Redis Pub/Sub `task:{id}:stream` + SSE
- **State**: Redis hashes `task:{id}:state`
- **Persistence**: SQLite for workflows, tools, keys, MCP configs
- **Worker pool**: configurable concurrency, graceful shutdown
- **Task lifecycle**: pending → cloning → running → completed (+ reviewing, awaiting_instruction, creating_pr, pr_created, failed)
