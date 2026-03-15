# Test Coverage — Critical Packages

## Goal

Add tests for critical packages that currently have zero test coverage. Focus on packages handling security, configuration, and data integrity.

## Current State (2026-03-14)

- **Test ratio:** 0.44x (5,717 LOC tests / 12,952 LOC main)
- **Well-tested:** workflow, tools (catalog/resolver/validator), runner normalizers, prompt, review parser
- **Zero tests:** 11 packages + 8 handler files

## Priority 1 — Security & Auth

### `internal/keys/` (3 .go files)

Handles encrypted API tokens (AES-256-GCM). Zero tests.

**Test cases:**
- `TestRegistry_CreateAndGet` — store key, retrieve, verify decryption
- `TestRegistry_List` — list all keys, verify no plaintext tokens in response
- `TestRegistry_Delete` — create + delete + verify gone
- `TestRegistry_DuplicateName` — conflict error on duplicate
- `TestRegistry_ResolveGitHub` — resolve key by repo URL + provider
- `TestRegistry_ResolveGitLab` — same for GitLab
- `TestRegistry_EncryptionRoundtrip` — encrypt → store → load → decrypt matches original

**File:** `internal/keys/registry_test.go`
**Pattern:** table-driven, in-memory SQLite

### `internal/server/middleware/` (4 .go files)

Auth middleware, rate limiting, logging, recovery. Zero tests.

**Test cases (auth.go):**
- `TestAuth_ValidToken` — correct bearer token → 200
- `TestAuth_InvalidToken` — wrong token → 401
- `TestAuth_MissingToken` — no header → 401
- `TestAuth_EmptyToken` — empty bearer → 401

**Test cases (ratelimit.go):**
- `TestRateLimit_UnderLimit` — requests under threshold → 200
- `TestRateLimit_OverLimit` — requests over threshold → 429
- `TestRateLimit_SlidingWindow` — old entries expire, new requests pass

**File:** `internal/server/middleware/auth_test.go`, `ratelimit_test.go`
**Pattern:** httptest, table-driven

### `internal/config/` (1 .go file)

Configuration loading from YAML + env vars. Zero tests.

**Test cases:**
- `TestLoad_DefaultValues` — no config file → sensible defaults
- `TestLoad_YAMLFile` — valid YAML → correct struct
- `TestLoad_EnvOverride` — env var overrides YAML value
- `TestLoad_NestedEnvVar` — `CODEFORGE_REDIS__URL` → nested field
- `TestLoad_InvalidYAML` — malformed YAML → error
- `TestLoad_MissingOptionalFile` — no config file → no error

**File:** `internal/config/config_test.go`
**Pattern:** temp files, env var setup/teardown

## Priority 2 — Data Integrity

### `internal/database/` (2 .go files)

SQLite wrapper + 11 migrations. Zero tests.

**Test cases:**
- `TestMigrations_AllApply` — all 11 migrations run without error
- `TestMigrations_Idempotent` — running twice doesn't fail
- `TestSchema_TasksTable` — verify columns match Task model fields
- `TestSchema_WorkflowsTable` — verify columns match WorkflowDefinition fields
- `TestSchema_KeysTable` — verify columns match Key model fields
- `TestSchema_ToolsTable` — verify columns match Tool model fields

**File:** `internal/database/database_test.go`
**Pattern:** in-memory SQLite, schema introspection via PRAGMA table_info

### Handler tests (8 files without tests)

**Priority order:**
1. `keys.go` — encrypted token CRUD
2. `workflow.go` — workflow management + run triggering
3. `stream.go` — SSE streaming
4. `mcp.go` — MCP server registry
5. `sentry.go` — Sentry proxy
6. `health.go` — health/ready/info
7. `repos.go` — repo listing
8. `workspace.go` — workspace management

**File:** `internal/server/handlers/*_test.go`
**Pattern:** httptest, mock service interfaces, table-driven

## Priority 3 — Nice to Have

### `internal/tool/mcp/` (3 .go files)

MCP server registry + installer. No tests.

- `TestWriteMCPConfig` — generates correct .mcp.json
- `TestSQLiteRegistry_CRUD` — create, get, list, delete servers
- `TestSetup_WithServers` — full setup writes config file

### `internal/webhook/` (1 .go file)

HMAC webhook signing/verification.

- `TestSign_SHA256` — correct HMAC signature
- `TestVerify_Valid` — valid signature passes
- `TestVerify_Invalid` — tampered payload fails
- `TestVerify_WrongSecret` — wrong secret fails

## Verification

```bash
task test              # all unit tests pass
task test:integration  # integration tests pass (Redis)
task lint              # golangci-lint clean
```

## Estimated Effort

| Group | Files | Test Cases | Effort |
|---|---|---|---|
| P1: keys + middleware + config | 3 test files | ~20 cases | ~4h |
| P2: database + handlers | 3-4 test files | ~15 cases | ~4h |
| P3: mcp + webhook | 2 test files | ~8 cases | ~2h |
| **Total** | **8-9 test files** | **~43 cases** | **~10h** |
