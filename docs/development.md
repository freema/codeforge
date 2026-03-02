# Development

All development happens inside Docker — no local Go or npm required.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Task](https://taskfile.dev/installation/) (go-task runner)

## Getting Started

```bash
# Start dev environment with hot reload
task dev

# Or run in background
task dev:detach

# View logs
task logs
```

The server starts at `http://localhost:8080` with hot reload via [air](https://github.com/air-verse/air).

## Commands

| Command | Description |
|---------|-------------|
| `task dev` | Start dev environment with hot reload |
| `task dev:detach` | Start dev environment in background |
| `task down` | Stop containers |
| `task down:clean` | Stop containers + remove volumes |
| `task build` | Build production Docker image |
| `task test` | Run unit tests |
| `task test:integration` | Run integration tests (needs running dev env) |
| `task test:e2e` | Run E2E tests (needs `task dev:e2e`) |
| `task test:all` | Run unit + integration tests |
| `task lint` | Run golangci-lint |
| `task fmt` | Run gofmt |
| `task logs` | Tail all container logs |
| `task logs:app` | Tail app logs only |
| `task shell` | Open shell in app container |
| `task redis:cli` | Open redis-cli |
| `task mod:tidy` | Run go mod tidy |

## Testing

### Unit Tests

```bash
task test
```

Unit tests run without Redis or any external services (`--no-deps`).

### Integration Tests

Integration tests hit the running HTTP server. They require the dev environment:

```bash
# Terminal 1: start the server
task dev:detach

# Terminal 2: run integration tests
task test:integration
```

### E2E Tests

E2E tests exercise the full task lifecycle (create -> clone -> run -> complete) using a mock CLI that simulates Claude Code output:

```bash
# Build mock CLI + start server with it
task dev:e2e:detach

# Run E2E tests
task test:e2e
```

The mock CLI (`tests/mockcli/main.go`) supports special prompts:
- `FAIL` — exits with code 1
- `TIMEOUT` — sleeps for 10 minutes (for cancel/timeout tests)
- `EMPTY` — produces no output
- Anything else — returns simulated stream-json output

## Project Structure

```
cmd/codeforge/          Application entry point + review adapter
internal/
  apperror/             Application error types (NotFound, Validation, Conflict, etc.)
  config/               Configuration loading (koanf, YAML + env vars)
  crypto/               AES-256-GCM encryption
  database/             SQLite wrapper + auto-migrations
  keys/                 Access key registry + 3-tier resolver
  logger/               Structured logging (slog)
  metrics/              Prometheus metric definitions
  prompt/               Prompt templates (embed FS, task types + code review)
  redisclient/          Redis client wrapper
  review/               Code review service (reviewer, parser, models)
  server/               HTTP server + handlers + middleware
    handlers/           Request handlers (tasks, keys, tools, workflows, stream, etc.)
    middleware/         Auth, logging, recovery, rate limit, metrics, tracing
  task/                 Task model, service, state machine, PR service
  tool/                 Tool subsystem namespace (low-level)
    git/                Git operations (clone, branch, PR creation)
    runner/             CLI runner interface + implementations (Claude Code, Codex)
    mcp/                MCP server registry + installer
  tools/                Tool system (high-level: catalog, registry, resolver, bridge)
  tracing/              OpenTelemetry setup
  webhook/              Webhook sender with HMAC signatures + retries
  worker/               Worker pool, executor, streamer, stream normalizer
  workflow/             Workflow orchestrator, step executors, templates
  workspace/            Workspace manager + cleanup
api/                    OpenAPI specification (openapi.yaml)
configs/                Example configuration files
deployments/            Docker, docker-compose files, .env
tests/
  integration/          Integration tests (HTTP API)
  e2e/                  E2E tests (full task lifecycle)
  mockcli/              Mock Claude Code CLI for testing
docs/                   Documentation
tasks/                  Planning documents (not code)
```

## Conventions

- **Go 1.24+**, standard library preferred where possible
- **Structured logging** via `log/slog` (JSON in production, text in dev)
- **Error handling**: return errors, don't panic; use typed errors from `internal/apperror`
- **Testing**: table-driven tests, `_test.go` next to source files
- **No shell injection**: all CLI invocations via `exec.Command` with explicit args
- **Git auth**: `GIT_ASKPASS` helper, never URL-embedded tokens
- **Sensitive fields**: encrypted in Redis (AES-256-GCM), never in API responses (`json:"-"`)
- **Multi-CLI**: tasks can specify `cli: "claude-code"` or `cli: "codex"` — registry resolves to runner
- **Task types**: `code` (default), `plan`, `review` — each wraps the user prompt with a template in the executor. New types: add template in `internal/prompt/templates/`, register in `prompt.go`
- **Stream normalizers**: each CLI has its own normalizer (`normalizer_claude.go`, `normalizer_codex.go`) mapping raw events to `NormalizedEvent`. New CLIs need a corresponding normalizer
- **Review as action**: code review is triggered by user via endpoint, not automatic in executor
