# Project Analysis — CodeForge State (2026-03-14)

## Codebase Stats

| Metric | Value |
|---|---|
| Go LOC (main) | 12,952 |
| Go LOC (tests) | 5,717 |
| Test ratio | 0.44x |
| Go files | 142 |
| Packages | ~25 |
| Packages with tests | 14 |
| Packages without tests | 11 |
| SQLite migrations | 11 |
| Direct dependencies | 21 |
| API endpoints | ~38 |

## Architecture Assessment

### Strong Points

- **Clean separation** — server/handlers, worker/executor, task/service layers well defined
- **No hardcoded secrets** — AES-256-GCM encryption, GIT_ASKPASS, json:"-" on sensitive fields
- **Context handling** — proper cancellation, timeouts throughout
- **Graceful shutdown** — signal handling, worker drain in main.go
- **Redis key namespacing** — consistent `codeforge:` prefix
- **State machine** — well-defined task status transitions
- **Multi-CLI** — clean runner interface, Claude + Codex implementations
- **Workflow system** — orchestrator, step executors, builtins, SSE streaming
- **No SQL injection** — parameterized queries everywhere
- **No shell injection** — exec.Command with explicit args, no shell=true
- **Prometheus metrics** — structured, well-named counters and gauges

### Weak Points

- **Test coverage gaps** — 11 packages untested, including security-critical ones (keys, auth middleware)
- **Silent error suppression** — ~36 instances of `_ =` on error returns
- **Schema migration tests missing** — no validation that DB schema matches Go models
- **Review refactor state unclear** — two files deleted, integration needs verification
- **No E2E test suite** — manual testing documented but no automated E2E
- **Handler tests sparse** — only webhook_receiver and tasks have partial coverage

## Package Health Matrix

| Package | Tests | Quality | Notes |
|---|---|---|---|
| `internal/workflow/` | Yes | Excellent | Orchestrator, steps, builtins, integration tests |
| `internal/tools/` | Yes | Excellent | Catalog, registry, resolver, validator, bridge |
| `internal/tool/runner/` | Yes | Good | Normalizers well-tested, runner interface clean |
| `internal/prompt/` | Yes | Good | Template rendering tested |
| `internal/review/` | Partial | OK | Parser + format tested, reviewer.go deleted |
| `internal/task/` | Partial | OK | SQLite store tested, service partially |
| `internal/server/handlers/` | Partial | Weak | Only webhook_receiver + tasks partial |
| `internal/tool/git/` | No | OK | Functional but untested |
| `internal/keys/` | No | Risk | Handles encrypted tokens, zero tests |
| `internal/config/` | No | Risk | All config loading, zero tests |
| `internal/database/` | No | Risk | Migrations, zero tests |
| `internal/server/middleware/` | No | Risk | Auth + rate limiting, zero tests |
| `internal/tool/mcp/` | No | OK | Registry + installer |
| `internal/webhook/` | No | OK | HMAC signing |
| `internal/workspace/` | No | OK | Dir management |
| `internal/crypto/` | No | Medium | AES-256-GCM (used by keys) |

## Dependencies

**go.mod — healthy:**
- Go 1.24.0 (current)
- chi/v5 — router
- koanf/v2 — config
- go-redis/v9 — Redis client
- modernc.org/sqlite — pure Go SQLite
- opentelemetry — tracing
- prometheus — metrics
- All dependencies at recent versions, no known CVEs

## Feature Completeness

| Feature | Status | Notes |
|---|---|---|
| Task CRUD | Done | Full lifecycle |
| Task queue (Redis FIFO) | Done | RPUSH + BLPOP |
| Git clone (GitHub + GitLab) | Done | GIT_ASKPASS auth |
| AI CLI execution | Done | Claude Code + Codex |
| SSE streaming | Done | History replay + live |
| Multi-turn (instruct) | Done | Workspace reuse |
| Code review (user-triggered) | Done* | *Verify after refactor |
| PR review (pr_review task type) | Done | Webhook + manual trigger |
| Review comment posting | Done | GitHub + GitLab APIs |
| PR/MR creation | Done | From task workspace |
| Webhook receiver | Done | GitHub + GitLab events |
| Workflow system | Done | Orchestrator + builtins |
| Knowledge-update workflow | Done | .codeforge/ generation |
| Sentry-fixer workflow | Done | Issue analysis + fix |
| Tool system | Done | Catalog, registry, resolver |
| MCP server management | Done | HTTP + stdio transports |
| Key management | Done | Encrypted storage |
| Prometheus metrics | Done | Basic counters + gauges |
| OpenTelemetry tracing | Done | Configurable |
| Rate limiting | Done | Sliding window (ZSET) |
| Auth middleware | Done | Bearer token |
| CI Action mode | Planned | See tasks/codeforge-ci-action.md |

## Builtin Workflows

| Name | Steps | Description |
|---|---|---|
| `sentry-fixer` | 3 | Analyze Sentry issue → fix code → create PR |
| `knowledge-update` | 3 | Analyze repo → generate .codeforge/ docs → create PR |
| `code-review` | 2 | Review code → post comments |

## Config Sections (from codeforge.example.yaml)

Server, Redis, SQLite, Auth, Workspace, Worker, CLI (Claude + Codex), Logging, Tracing, Metrics, Webhooks, Rate Limit, Code Review, Sentry

All config sections have corresponding implementation in `internal/config/config.go`.

## Risk Areas

1. **Review flow after refactor** — deleted adapter/reviewer, needs E2E verification
2. **Keys encryption** — working but untested, single point of failure for all API tokens
3. **Migration 010** — table recreation pattern, risky for production data
4. **Executor error handling** — 20 silent streaming failures possible

## Recommendations

See companion task files:
- `tasks/tech-debt.md` — executor streaming, schema safety, review verification
- `tasks/test-coverage.md` — test plan for untested packages
- `tasks/codeforge-ci-action.md` — CI Action feature plan

## Priority Order

1. **Verify review flow** (tech-debt P0) — may be broken
2. **Critical tests** (test-coverage P1) — keys, config, middleware
3. **Executor logging** (tech-debt P1) — silent failures
4. **Schema tests** (tech-debt P1) — migration safety
5. **Handler tests** (test-coverage P2) — HTTP contract
6. **CI Action** (new feature) — after stability
7. **Docs update** — README + architecture
