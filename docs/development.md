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
cmd/codeforge/          Application entry point
internal/
  apperror/             Application error types
  cli/                  CLI runner interface + implementations
  config/               Configuration loading (koanf)
  crypto/               AES-256-GCM encryption
  git/                  Git operations (clone, branch, PR)
  keys/                 Access key registry + resolver
  logger/               Structured logging (slog)
  mcp/                  MCP server registry + installer
  metrics/              Prometheus metric definitions
  redisclient/          Redis client wrapper
  server/               HTTP server + handlers + middleware
  task/                 Task model, service, state machine
  tracing/              OpenTelemetry setup
  webhook/              Webhook sender with HMAC + retries
  worker/               Worker pool, executor, streamer
  workspace/            Workspace manager + cleanup
api/                    OpenAPI specification
deployments/            Docker, docker-compose files
tests/
  integration/          Integration tests (HTTP API)
  e2e/                  E2E tests (full task lifecycle)
  mockcli/              Mock Claude Code CLI
docs/                   Documentation
```

## Conventions

- **Go 1.24+**, standard library preferred where possible
- **Structured logging** via `log/slog` (JSON in production, text in dev)
- **Error handling**: return errors, don't panic; use typed errors from `internal/apperror`
- **Testing**: table-driven tests, `_test.go` next to source files
- **No shell injection**: all CLI invocations via `exec.Command` with explicit args
- **Git auth**: `GIT_ASKPASS` helper, never URL-embedded tokens
