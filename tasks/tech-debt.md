# Tech Debt — Executor Streaming, Schema Safety, Review Verify

## Goal

Address technical debt found in project analysis (2026-03-14). Three areas: silent error suppression, unvalidated schema evolution, review refactor completion.

---

## 1. Executor Streaming — Silent Error Suppression

### Problem

`internal/worker/executor.go` has ~20 instances of `_ = e.streamer.Emit*(...)`. If Redis Pub/Sub fails, SSE clients hang without explanation. No logging, no alerting.

### Current Pattern

```go
_ = e.streamer.EmitSystem(ctx, t.ID, "task_timeout", ...)
_ = e.streamer.EmitResult(ctx, t.ID, "task_completed", ...)
_ = e.streamer.EmitGit(ctx, t.ID, "clone_start", ...)
```

### Fix

Replace all `_ =` with warn-level logging:

```go
if err := e.streamer.EmitSystem(ctx, t.ID, "task_timeout", ...); err != nil {
    e.logger.Warn("stream emit failed", "event", "task_timeout", "task_id", t.ID, "err", err)
}
```

**Why not error level?** Stream failures are non-fatal — task execution continues. The result is still stored in Redis. SSE is best-effort delivery.

### Also Fix in Other Files

- `internal/tool/git/github_review.go:123` — JSON unmarshal ignored
- `internal/tool/git/diff.go:100-111` — strconv.Atoi errors suppressed
- `internal/tool/mcp/sqlite_registry.go:60, 108-111, 135-138` — JSON unmarshal + time.Parse ignored
- `internal/workflow/sqlite_registry.go:84-86, 147-149` — JSON unmarshal + time.Parse ignored
- `internal/keys/registry.go:85, 124, 166` — API response unmarshal ignored

### Scope

- ~36 instances across codebase
- Most are streaming (non-fatal) — log at warn
- Some are data parsing (could corrupt) — log at error or handle properly

### Effort: ~2h

---

## 2. Schema Migration Safety

### Problem

11 SQLite migrations exist, but no test validates that the schema matches Go model structs. A migration could add a column that the Go code doesn't Scan, or vice versa — causing runtime crashes.

### Recent Example

`workflow_run_id` column was added in migration `011` but the `Get()` and `List()` methods in `sqlite_store.go` didn't Scan it — causing runtime crashes. Fixed manually but could have been caught by tests.

### Fix

Add schema validation test in `internal/database/database_test.go`:

```go
func TestSchema_MatchesModels(t *testing.T) {
    db := setupTestDB(t) // in-memory SQLite with all migrations

    // Verify tasks table columns match Task struct fields
    columns := getColumns(t, db, "tasks")
    require.Contains(t, columns, "id")
    require.Contains(t, columns, "workflow_run_id")
    require.Contains(t, columns, "review_result_json")
    // ... all fields

    // Verify round-trip: insert → select → scan works
    // This catches Scan mismatches
}
```

### Risky Migrations

| Migration | Risk | Reason |
|---|---|---|
| `010_keys_add_sentry_provider.sql` | High | Table recreation (DROP + CREATE) |
| `011_add_workflow_run_id.sql` | Medium | New column, Scan mismatch already found |
| `006_add_review_result.sql` | Medium | JSON column, nullable |

### Effort: ~2h

---

## 3. Review System — Verify Post-Refactor State

### Problem

Two files were deleted in the current working tree:
- `cmd/codeforge/review_adapter.go` — bridge between task.Service and review.TaskProvider
- `internal/review/reviewer.go` — review service with ReviewTask() method

### Questions to Answer

1. **Does `POST /tasks/:id/review` still work?** — Handler calls service.StartReviewAsync, which queues task. Executor picks it up. But how does executor get review prompt without reviewer.go?

2. **Where is review prompt built now?** — Check executor.go for review-specific logic

3. **Is ReviewResult still populated?** — Check executor.go for ParseReviewOutput calls

4. **Does review posting to GitHub/GitLab still work?** — Check executor.go for PostReviewComments calls

### Verification Steps

```bash
# 1. Check executor has review logic
grep -n "review\|Review" internal/worker/executor.go

# 2. Check handler still calls correct service method
grep -n "review\|Review" internal/server/handlers/tasks.go

# 3. Check review package exports are still used
grep -rn "review\." internal/worker/ internal/server/

# 4. Manual E2E test
# Create task → complete → POST /tasks/:id/review → verify ReviewResult
```

### Possible Outcomes

- **A) Refactor is complete** — review logic moved into executor.go, adapter/reviewer were redundant dead code. Just verify and document.
- **B) Refactor is incomplete** — some review functionality is broken. Fix integration.

### Effort: ~1-2h (investigation + possible fix)

---

## 4. CLI Registry Startup Validation

### Problem

`cmd/codeforge/main.go` registers CLI runners but doesn't validate they're actually available on PATH. If Claude Code isn't installed, tasks silently fail at execution time.

### Fix

```go
// After registering runners, validate availability
for _, name := range []string{"claude", "codex"} {
    if _, err := exec.LookPath(name); err != nil {
        logger.Warn("CLI runner not found on PATH", "cli", name)
    }
}
```

### Effort: ~15min

---

## Summary

| Item | Priority | Effort | Impact |
|---|---|---|---|
| Review system verification | P0 | 1-2h | May be broken |
| Executor streaming logging | P1 | 2h | Silent failures |
| Schema migration tests | P1 | 2h | Runtime crashes |
| CLI startup validation | P2 | 15min | Better error messages |
| Other error suppressions | P2 | 2h | Data integrity |
| **Total** | | **~8h** | |
