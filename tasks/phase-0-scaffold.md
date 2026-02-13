# Phase 0 — Project Scaffold (v0.1.0)

> Foundation: repository structure, CI/CD, Docker, basic wiring.

---

## Task 0.1: Repository Setup

**Priority:** P0
**Files:** `go.mod`, `Taskfile.yaml`, `.editorconfig`, `CLAUDE.md`

### Description

Initialize the Go module as `github.com/freema/codeforge`. Create `Taskfile.yaml` (go-task) with all dev commands — everything runs inside Docker, no local Go/npm required. Add `.editorconfig` for consistent formatting. Create `CLAUDE.md` with project conventions.

### Acceptance Criteria

- [ ] `go mod init github.com/freema/codeforge` creates valid `go.mod`
- [ ] `task dev` starts docker-compose dev environment (CodeForge + Redis) with hot reload
- [ ] `task build` builds production Docker image
- [ ] `task test` runs all tests inside Docker
- [ ] `task lint` runs golangci-lint inside Docker
- [ ] `task logs` tails docker-compose logs
- [ ] `task down` stops and cleans up containers
- [ ] `task test:integration` runs integration tests with Redis inside Docker
- [ ] `.editorconfig` configures Go standard formatting (tabs, 120 line width)
- [ ] `CLAUDE.md` documents task commands, conventions, directory structure
- [ ] **No local Go/npm required** — all commands execute inside Docker containers

### Implementation Notes

```yaml
# Taskfile.yaml
version: '3'

vars:
  DOCKER_COMPOSE: docker compose -f deployments/docker-compose.yaml
  DOCKER_COMPOSE_DEV: docker compose -f deployments/docker-compose.yaml -f deployments/docker-compose.dev.yaml

tasks:
  dev:
    desc: Start dev environment with hot reload
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} up --build"

  dev:detach:
    desc: Start dev environment in background
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} up --build -d"

  down:
    desc: Stop all containers
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} down"

  build:
    desc: Build production Docker image
    cmds:
      - docker build -f deployments/Dockerfile -t codeforge .

  test:
    desc: Run unit tests inside Docker
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} run --rm codeforge go test -race -cover ./..."

  lint:
    desc: Run linter inside Docker
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} run --rm codeforge golangci-lint run"

  test:integration:
    desc: Run integration tests (needs Redis)
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} run --rm codeforge go test -tags integration -race ./tests/integration/..."

  logs:
    desc: Tail docker-compose logs
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} logs -f"

  redis:cli:
    desc: Open redis-cli
    cmds:
      - "{{.DOCKER_COMPOSE_DEV}} exec redis redis-cli"
```

### Dependencies

None — this is the starting point.

---

## Task 0.2: Config System

**Priority:** P0
**Files:** `internal/config/config.go`, `configs/codeforge.example.yaml`

### Description

Implement configuration loading from YAML file + environment variable overrides. Use `koanf` library (lighter alternative to Viper). Define all config structs with validation and sensible defaults.

### Acceptance Criteria

- [ ] Config loads from `codeforge.yaml` (or path via `CODEFORGE_CONFIG` env var)
- [ ] Every YAML field can be overridden by env var (e.g., `CODEFORGE_SERVER_PORT`)
- [ ] Missing required fields (Redis URL, auth token) cause startup failure with clear error
- [ ] Default values: port=8080, workers=3, timeout=300s, workspace_ttl=86400s, state_ttl=604800s, result_ttl=604800s, disk_warning_threshold_gb=10, disk_critical_threshold_gb=20
- [ ] Example config file provided at `configs/codeforge.example.yaml`
- [ ] Unit tests for config loading and validation

### Implementation Notes

**Library:** `github.com/knadh/koanf/v2` — cleaner and lighter than Viper.

```go
type Config struct {
    Server   ServerConfig   `koanf:"server"`
    Redis    RedisConfig    `koanf:"redis"`
    Workers  WorkersConfig  `koanf:"workers"`
    Tasks    TasksConfig    `koanf:"tasks"`
    CLI      CLIConfig      `koanf:"cli"`
    Git      GitConfig      `koanf:"git"`
    MCP      MCPConfig      `koanf:"mcp"`
    Webhooks WebhookConfig  `koanf:"webhooks"`
    Logging  LoggingConfig  `koanf:"logging"`
}

// Loading order: defaults → YAML file → env vars
k := koanf.New(".")
k.Load(structs.Provider(defaults, "koanf"), nil)
k.Load(file.Provider("codeforge.yaml"), yaml.Parser())
k.Load(env.Provider("CODEFORGE_", ".", func(s string) string {
    return strings.Replace(strings.ToLower(strings.TrimPrefix(s, "CODEFORGE_")), "_", ".", -1)
}), nil)
```

> **⚠ Known issue:** Underscore-to-dot mapping is ambiguous for nested config
> (e.g., `CODEFORGE_TASKS_WORKSPACE_BASE` → `tasks.workspace.base` vs `tasks.workspace_base`).
> Use koanf's `__` (double underscore) delimiter for nested keys:
> `CODEFORGE_TASKS__WORKSPACE_BASE` → `tasks.workspace_base`

### Dependencies

None.

---

## Task 0.3: Docker Multi-Stage Build

**Priority:** P0
**Files:** `deployments/Dockerfile`, `deployments/Dockerfile.dev`, `deployments/docker-compose.yaml`, `deployments/docker-compose.dev.yaml`, `.dockerignore`

### Description

Create a multi-stage production Dockerfile + dev Dockerfile with hot reload. **All development happens inside Docker** — no local Go/npm installation required. Docker Compose for full dev environment with Redis.

### Acceptance Criteria

- [ ] **Production Dockerfile:** multi-stage build, working image under 500MB
- [ ] **Dev Dockerfile:** Go + golangci-lint + air (hot reload) + git + Node.js + Claude CLI
- [ ] Runtime image has: `git`, `node`, `npm`, `claude` CLI
- [ ] Binary runs as non-root user (production)
- [ ] `task dev` starts dev environment: CodeForge (hot reload) + Redis
- [ ] `task build` builds production image
- [ ] Health check configured in production Dockerfile
- [ ] `.dockerignore` excludes `.git`, `bin/`, `*.test`
- [ ] Source code mounted as volume in dev (live changes without rebuild)
- [ ] Go module cache preserved in named volume (fast rebuilds)

### Implementation Notes

```dockerfile
# Stage 1: Build
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o codeforge ./cmd/codeforge

# Stage 2: Runtime
FROM alpine:3.20
ARG CLAUDE_CODE_VERSION=1.0.0
RUN apk add --no-cache git nodejs npm ca-certificates tzdata \
    && npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} \
    && adduser -D -h /home/codeforge codeforge
USER codeforge
COPY --from=builder /build/codeforge /usr/local/bin/
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["codeforge"]
```

```yaml
# deployments/docker-compose.yaml (base — production-like)
services:
  codeforge:
    build:
      context: ../
      dockerfile: deployments/Dockerfile
    ports: ["8080:8080"]
    environment:
      CODEFORGE_REDIS__URL: redis://redis:6379
      CODEFORGE_SERVER__AUTH_TOKEN: dev-token
      CODEFORGE_WEBHOOKS__HMAC_SECRET: dev-secret
      CODEFORGE_ENCRYPTION__KEY: "dGVzdC1lbmNyeXB0aW9uLWtleS0zMi1ieXRlcyE=" # dev only
    volumes:
      - workspaces:/data/workspaces
    depends_on:
      redis:
        condition: service_healthy
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 3
volumes:
  workspaces:
```

```yaml
# deployments/docker-compose.dev.yaml (dev overlay — hot reload, source mount)
services:
  codeforge:
    build:
      context: ../
      dockerfile: deployments/Dockerfile.dev
    volumes:
      - ../:/app                    # mount source code (live changes)
      - go-mod-cache:/go/pkg/mod    # persist Go module cache
      - go-build-cache:/root/.cache/go-build
      - workspaces:/data/workspaces
    environment:
      CODEFORGE_LOGGING__LEVEL: debug
      CODEFORGE_LOGGING__FORMAT: text
volumes:
  go-mod-cache:
  go-build-cache:
  workspaces:
```

```dockerfile
# deployments/Dockerfile.dev (development — hot reload with air)
FROM golang:1.23-alpine

RUN apk add --no-cache git nodejs npm ca-certificates tzdata curl
ARG CLAUDE_CODE_VERSION=1.0.0
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}

# Install air for hot reload
RUN go install github.com/air-verse/air@latest
# Install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin

WORKDIR /app
# Go module download happens on first run (source mounted as volume)

EXPOSE 8080
CMD ["air", "-c", ".air.toml"]
```

### Dependencies

- Task 0.1 (go.mod must exist)

---

## Task 0.4: GitHub Actions CI

**Priority:** P0
**Files:** `.github/workflows/ci.yaml`, `.github/workflows/release.yaml`

### Description

CI pipeline: lint, test, build on every push/PR. Release pipeline: build and push Docker image to `ghcr.io/freema/codeforge` on tag push.

### Acceptance Criteria

- [ ] CI runs on push to `main` and all PRs
- [ ] CI steps: checkout → setup Go → lint → test → build
- [ ] Release triggers on `v*` tags
- [ ] Release builds multi-arch image (amd64 + arm64)
- [ ] Image pushed to `ghcr.io/freema/codeforge:latest` and `ghcr.io/freema/codeforge:v0.x.x`
- [ ] CI uses Go module cache for speed

### Implementation Notes

```yaml
# ci.yaml triggers
on:
  push:
    branches: [main]
  pull_request:

# release.yaml triggers
on:
  push:
    tags: ["v*"]

# Use docker/build-push-action with QEMU for multi-arch
```

### Dependencies

- Task 0.1 (Taskfile targets)

---

## Task 0.5: Logging & Error Handling

**Priority:** P0
**Files:** `internal/logger/logger.go`, error types in relevant packages

### Description

Set up structured JSON logging using Go's built-in `log/slog`. Define error types for consistent error handling across the application.

### Acceptance Criteria

- [ ] `slog` configured with JSON handler for production, text handler for development
- [ ] Log level configurable via config (debug, info, warn, error)
- [ ] Request ID propagated through context and included in all log entries
- [ ] Standard error types: `ErrNotFound`, `ErrValidation`, `ErrUnauthorized`, `ErrInternal`
- [ ] Errors include structured fields (task_id, operation, etc.)

### Implementation Notes

```go
// Use Go 1.21+ slog
func SetupLogger(cfg LoggingConfig) *slog.Logger {
    var handler slog.Handler
    opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
    if cfg.Format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }
    return slog.New(handler)
}

// Context-aware logging
func FromContext(ctx context.Context) *slog.Logger {
    if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
        return l
    }
    return slog.Default()
}
```

### Dependencies

- Task 0.2 (config for log level/format)

---

## Task 0.6: Redis Client

**Priority:** P0
**Files:** `internal/redis/client.go`

### Description

Create Redis client wrapper with connection pooling, health check, and auto-reconnect. Use `github.com/redis/go-redis/v9`.

### Acceptance Criteria

- [ ] Connects to Redis using URL from config
- [ ] Connection pool configured (pool size, timeouts, idle connections)
- [ ] `Ping()` method for health checks
- [ ] `Close()` for graceful shutdown
- [ ] Reconnect on connection loss (built into go-redis)
- [ ] Key prefix support (e.g., `codeforge:` prepended to all keys)
- [ ] Unit tests with mock/stub

### Implementation Notes

```go
// go-redis v9 client setup (from Context7 research)
// Parse Redis URL (redis://user:pass@host:port/db) into Options
opt, err := redis.ParseURL(cfg.URL)  // handles redis:// URLs correctly
// Then override specific settings:
rdb := redis.NewClient(&redis.Options{
    Addr:         opt.Addr,
    Password:     opt.Password,
    DB:           cfg.DB,
    PoolSize:     10,
    MinIdleConns: 5,
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
    MaxRetries:   3,
    MinRetryBackoff: 8 * time.Millisecond,
    MaxRetryBackoff: 512 * time.Millisecond,
})

// Health check
pong, err := rdb.Ping(ctx).Result()
```

**BLPOP for queue** (RPUSH+BLPOP = FIFO, used later in Phase 1):
```go
result, err := rdb.BLPop(ctx, 5*time.Second, "queue:tasks").Result()
// result[0] = key name, result[1] = task ID
```

**Pub/Sub for streaming** (used later in Phase 1):
```go
pubsub := rdb.Subscribe(ctx, "task:{id}:stream")
ch := pubsub.Channel()
for msg := range ch { ... }
```

### Dependencies

- Task 0.2 (Redis config)

---

## Task 0.7: HTTP Server Skeleton

**Priority:** P0
**Files:** `internal/server/server.go`, `internal/server/middleware/auth.go`, `internal/server/middleware/logging.go`, `internal/server/middleware/recovery.go`

### Description

Set up Chi router with middleware stack, route groups, and graceful shutdown. Implement Bearer token auth middleware, request logging middleware, and panic recovery.

### Acceptance Criteria

- [ ] Chi router with `/api/v1` route group
- [ ] Bearer token auth middleware on all API routes
- [ ] Request logging middleware (method, path, status, duration)
- [ ] Panic recovery middleware
- [ ] Request ID middleware (X-Request-ID header)
- [ ] Graceful shutdown on SIGINT/SIGTERM (drain connections, timeout)
- [ ] CORS headers configured
- [ ] Health endpoints excluded from auth

### Implementation Notes

```go
// Chi router setup (from Context7 research)
r := chi.NewRouter()

// Global middleware
r.Use(middleware.RequestID)
r.Use(middleware.RealIP)
r.Use(middleware.Logger)     // or custom slog middleware
r.Use(middleware.Recoverer)
r.Use(middleware.Timeout(60 * time.Second))

// Public routes
r.Get("/health", healthHandler)
r.Get("/ready", readyHandler)

// Protected API routes
r.Route("/api/v1", func(r chi.Router) {
    r.Use(BearerAuthMiddleware(cfg.AuthToken))

    r.Route("/tasks", func(r chi.Router) {
        r.Post("/", createTask)
        r.Get("/{taskID}", getTask)
        r.Post("/{taskID}/instruct", instructTask)
        r.Post("/{taskID}/cancel", cancelTask)
    })

    r.Route("/keys", func(r chi.Router) { ... })
    r.Route("/mcp", func(r chi.Router) { ... })
})

// Graceful shutdown
srv := &http.Server{Addr: ":8080", Handler: r}
go srv.ListenAndServe()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

**Bearer auth middleware:** See Task 1.12 for canonical implementation with `crypto/subtle.ConstantTimeCompare`.
The middleware skeleton in this task is a placeholder — Task 1.12 provides the secure version.

### Dependencies

- Task 0.2 (server config, auth token)
- Task 0.5 (logging)

---

## Task 0.8: Health & Readiness Endpoints

**Priority:** P0
**Files:** `internal/server/handlers/health.go`

### Description

Implement `/health` and `/ready` endpoints. Health checks Redis connectivity and worker status. Readiness indicates the server can accept traffic.

### Acceptance Criteria

- [ ] `GET /health` returns 200 with `{"status": "ok", "redis": "connected", "workers": N}`
- [ ] `GET /health` returns 503 if Redis is down
- [ ] `GET /ready` returns 200 when server is accepting traffic
- [ ] `GET /ready` returns 503 during shutdown
- [ ] Both endpoints excluded from auth middleware
- [ ] Response includes version info

### Implementation Notes

```go
type HealthResponse struct {
    Status  string `json:"status"`           // "ok" | "degraded" | "error"
    Redis   string `json:"redis"`            // "connected" | "disconnected"
    Workers int    `json:"active_workers"`
    Version string `json:"version"`
    Uptime  string `json:"uptime"`
}
```

### Dependencies

- Task 0.6 (Redis client for ping)
- Task 0.7 (HTTP server to register routes)
