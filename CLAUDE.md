# CodeForge — Developer Guide

## Overview

CodeForge is a Go HTTP server that acts as a remote code task runner. It clones repos, runs AI CLI tools (Claude Code), streams progress via Redis, and returns results.

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
cmd/codeforge/         Main entry point
internal/
  config/              Configuration (koanf, YAML + env vars)
  server/              HTTP server (Chi router)
    handlers/          Request handlers
    middleware/        Auth, logging, recovery
  task/                Task model, service, state machine
  worker/              Worker pool, executor, streaming
  git/                 Clone, branch, GitHub/GitLab PR
  cli/                 AI CLI runner interface + Claude Code implementation
  keys/                Key registry + AES-256-GCM encryption
  mcp/                 MCP server registry + installer
  workspace/           Workspace lifecycle, cleanup
  webhook/             HMAC-signed webhook callbacks
  redis/               Redis client wrapper
configs/               Example config files
deployments/           Dockerfiles, docker-compose
tests/                 Integration + E2E tests
tasks/                 Planning documents (not code)
```

## Conventions

- **Go 1.23+**, standard library preferred where possible
- **Structured logging** via `log/slog` (JSON in production, text in dev)
- **Error handling**: return errors, don't panic; use typed errors from `internal/errors`
- **Testing**: table-driven tests, `_test.go` next to source files
- **Config**: koanf with YAML + env var override (`CODEFORGE_` prefix, `__` for nested)
- **Redis keys**: prefixed with `codeforge:` in production
- **No shell injection**: all CLI via `exec.Command` with explicit args
- **Sensitive fields**: encrypted in Redis (AES-256-GCM), never in API responses
- **Git auth**: GIT_ASKPASS helper, never URL-embedded tokens

## Architecture

- **HTTP API**: Chi router at `/api/v1/`
- **Task queue**: Redis RPUSH + BLPOP (FIFO)
- **Streaming**: Redis Pub/Sub `task:{id}:stream`
- **State**: Redis hashes `task:{id}:state`
- **Worker pool**: configurable concurrency, graceful shutdown
