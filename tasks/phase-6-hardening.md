# Phase 6 — Production Hardening (v1.0.0)

> Metrics, tracing, security, documentation, testing.

---

## Task 6.1: Rate Limiting

**Priority:** P1
**Files:** `internal/server/middleware/ratelimit.go`

### Description

Per-client rate limiting using sliding window algorithm (Redis ZSET). Prevent abuse of task submission.

### Acceptance Criteria

- [ ] Rate limit on `POST /api/v1/tasks` (configurable: e.g., 10 req/min)
- [ ] Per Bearer token rate limiting (different clients get separate limits)
- [ ] Redis-based sliding window via ZSET (works across multiple instances, no race conditions)
- [ ] Returns 429 Too Many Requests with `Retry-After` header
- [ ] Rate limit config in YAML: `rate_limit.tasks_per_minute: 10`
- [ ] Bypass for health/ready endpoints

### Implementation Notes

```go
// Redis sliding window (ZSET)
// Key: ratelimit:{token_hash}
// Use MULTI/EXEC for atomic check-and-decrement

func (rl *RateLimiter) Allow(ctx context.Context, clientID string) (bool, time.Duration) {
    key := "ratelimit:" + hash(clientID)

    // Sliding window counter using Redis ZSET
    now := time.Now().UnixMilli()
    windowStart := now - int64(rl.window.Milliseconds())

    pipe := rl.redis.Pipeline()
    pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
    pipe.ZCard(ctx, key)
    pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
    pipe.Expire(ctx, key, rl.window)
    results, _ := pipe.Exec(ctx)

    count := results[1].(*redis.IntCmd).Val()
    return count < int64(rl.limit), rl.retryAfter(count)
}
```

### Dependencies

- Task 0.6 (Redis client)
- Task 0.7 (HTTP middleware)

---

## Task 6.2: Prometheus Metrics

**Priority:** P1
**Files:** `internal/metrics/metrics.go`, `internal/server/server.go`

### Description

Expose Prometheus metrics for monitoring task execution, queue depth, worker utilization, and HTTP request performance.

### Acceptance Criteria

- [ ] `GET /metrics` — Prometheus scrape endpoint
- [ ] Metrics:
  - `codeforge_tasks_total` (counter, labels: status)
  - `codeforge_tasks_duration_seconds` (histogram, labels: status, cli)
  - `codeforge_tasks_in_progress` (gauge)
  - `codeforge_queue_depth` (gauge)
  - `codeforge_workers_active` (gauge)
  - `codeforge_workers_total` (gauge)
  - `codeforge_webhook_deliveries_total` (counter, labels: status)
  - `codeforge_http_requests_total` (counter, labels: method, path, status)
  - `codeforge_http_request_duration_seconds` (histogram, labels: method, path)
  - `codeforge_workspace_disk_usage_bytes` (gauge)
- [ ] All metrics registered with descriptive help text
- [ ] Metrics endpoint excluded from auth middleware

### Implementation Notes

```go
// Use prometheus/client_golang
import "github.com/prometheus/client_golang/prometheus"
import "github.com/prometheus/client_golang/prometheus/promhttp"

var (
    tasksTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "codeforge_tasks_total",
            Help: "Total number of tasks processed",
        },
        []string{"status"},
    )
    taskDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "codeforge_tasks_duration_seconds",
            Help:    "Task execution duration in seconds",
            Buckets: []float64{10, 30, 60, 120, 300, 600, 1800},
        },
        []string{"status", "cli"},
    )
)

// Register in init or setup
prometheus.MustRegister(tasksTotal, taskDuration, ...)

// Expose
r.Handle("/metrics", promhttp.Handler())
```

### Dependencies

- Task 0.7 (HTTP server)
- Task 1.4 (worker pool for active/total gauges)

---

## Task 6.3: OpenTelemetry Tracing

**Priority:** P2
**Files:** `internal/tracing/tracing.go`

### Description

Distributed tracing across the full task lifecycle: HTTP request → queue → worker → clone → execute → callback.

### Acceptance Criteria

- [ ] OpenTelemetry SDK initialized with configurable exporter (Jaeger, OTLP)
- [ ] Trace spans for: HTTP handler, queue enqueue/dequeue, git clone, CLI execution, webhook delivery
- [ ] Trace ID propagated through task via `TraceID` field on Task struct (added in Task 1.1), stored in Redis state hash
- [ ] Trace ID included in webhook callback headers (`X-Trace-ID`)
- [ ] Sampling rate configurable
- [ ] Can be disabled via config (`tracing.enabled: false`)

### Implementation Notes

```go
// Use go.opentelemetry.io/otel
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace"
    "go.opentelemetry.io/otel/sdk/trace"
)

// Chi middleware for HTTP tracing
// otelchi: go.opentelemetry.io/contrib/instrumentation/github.com/go-chi/chi/otelchi
r.Use(otelchi.Middleware("codeforge"))
```

### Dependencies

- All previous phases (tracing wraps all components)

---

## Task 6.4: API Documentation (OpenAPI)

**Priority:** P1
**Files:** `api/openapi.yaml`, generation tooling

### Description

OpenAPI 3.0 specification for all API endpoints. Auto-generate from code annotations or maintain manually.

### Acceptance Criteria

- [ ] Complete OpenAPI 3.0 spec covering all endpoints
- [ ] Request/response schemas for all operations
- [ ] Authentication documented (Bearer token)
- [ ] Error response schemas
- [ ] Served at `GET /api/v1/docs` (optional Swagger UI)
- [ ] Spec used for client code generation (TypeScript for ScopeBot)

### Implementation Notes

Option A: Manual YAML spec in `api/openapi.yaml`
Option B: Code-first with `swaggo/swag` annotations

Recommend Option A for accuracy — maintain manually, validate in CI.

### Dependencies

- All Phase 1-4 endpoints implemented

---

## Task 6.5: Integration Tests

**Priority:** P0
**Files:** `tests/integration/`, `tests/docker-compose.test.yaml`

### Description

Full end-to-end tests using Docker Compose with real Redis. Test the complete flow: submit task → clone → execute → callback.

### Acceptance Criteria

- [ ] Docker Compose test environment: CodeForge + Redis + mock callback server
- [ ] Test cases:
  - Submit task and check status transitions
  - Verify webhook callback with HMAC signature
  - Verify Redis Pub/Sub streaming output
  - Task timeout enforcement
  - Cancel running task
  - Follow-up instruction (iteration)
  - Invalid input validation
  - Auth rejection
- [ ] Mock Git repo (local or test fixture)
- [ ] Mock AI CLI (simple echo tool for testing)
- [ ] CI pipeline runs integration tests
- [ ] Test timeout: 5 minutes max

### Implementation Notes

```go
// Mock CLI runner for tests
type MockRunner struct {
    Output string
    Delay  time.Duration
}

func (m *MockRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    time.Sleep(m.Delay)
    return &RunResult{Output: m.Output, ExitCode: 0}, nil
}

// Mock callback server
func startCallbackServer(t *testing.T) (string, chan WebhookPayload) {
    ch := make(chan WebhookPayload, 1)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var payload WebhookPayload
        json.NewDecoder(r.Body).Decode(&payload)
        ch <- payload
        w.WriteHeader(200)
    }))
    return srv.URL, ch
}
```

### Dependencies

- Phase 1 complete (core flow to test)

---

## Task 6.6: Security Audit

**Priority:** P0
**Files:** All files (review), `internal/` (hardening)

### Description

Security review of the entire codebase focusing on token handling, input validation, and command injection prevention.

### Checklist

- [ ] **Command injection**: All `exec.Command` calls use explicit args, no `sh -c`
- [ ] **Token leaks**: Access tokens never in logs, error messages, or API responses
- [ ] **Input validation**: All user input validated (URLs, prompts, config values)
- [ ] **Path traversal**: Workspace paths validated, no `../` escape
- [ ] **HMAC verification**: Constant-time comparison in webhook signature verification
- [ ] **Redis security**: Password auth, no exposed ports in production
- [ ] **Docker security**: Non-root user, minimal base image, no unnecessary packages
- [ ] **Rate limiting**: Prevents resource exhaustion
- [ ] **Timeout enforcement**: All operations have timeouts (HTTP, CLI, webhook)
- [ ] **Error exposure**: Internal errors not exposed to clients (generic messages)
- [ ] **Dependency audit**: `go mod audit` or `govulncheck` in CI

### Dependencies

- All previous phases

---

## Task 6.7: README & Documentation

**Priority:** P0
**Files:** `README.md`, `docs/`

### Description

User-facing documentation: setup guide, API reference, architecture overview, deployment guide.

### Acceptance Criteria

- [ ] `README.md` with: overview, quick start, API examples, config reference
- [ ] `docs/deployment.md` — Docker, Kubernetes, environment variables
- [ ] `docs/api.md` — all endpoints with curl examples
- [ ] `docs/architecture.md` — system design, Redis schema, task lifecycle
- [ ] Contributing guide (`CONTRIBUTING.md`)
- [ ] Changelog (`CHANGELOG.md`)

### Dependencies

- All previous phases (to document)

---

## Task 6.8: Multi-CLI Support

**Priority:** P2
**Files:** `internal/cli/runner.go`, new CLI implementations

### Description

Plugin architecture for supporting AI CLI tools beyond Claude Code. Enable adding new tools without modifying core code.

### Acceptance Criteria

- [ ] `Runner` interface already defined (Task 1.7) — verify it's generic enough
- [ ] CLI tool selected by `config.cli` field in task payload
- [ ] Registry of available CLIs: `map[string]Runner`
- [ ] New CLI implementations can be added by implementing `Runner` interface
- [ ] Example implementations: Claude Code, Aider, Cursor CLI (stubs)
- [ ] CLI availability check at startup (binary exists in PATH)

### Implementation Notes

```go
// CLI registry
type Registry struct {
    runners map[string]Runner
}

func (r *Registry) Register(name string, runner Runner) {
    r.runners[name] = runner
}

func (r *Registry) Get(name string) (Runner, error) {
    runner, ok := r.runners[name]
    if !ok {
        return nil, fmt.Errorf("unknown CLI: %s", name)
    }
    return runner, nil
}

// Registration at startup
registry.Register("claude-code", NewClaudeRunner(cfg))
registry.Register("aider", NewAiderRunner(cfg))  // future
```

### Dependencies

- Task 1.7 (Runner interface)
