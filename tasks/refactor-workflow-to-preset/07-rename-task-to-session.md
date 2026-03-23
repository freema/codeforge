# Fáze 7: Rename task → session terminologie

**Commit:** `refactor: rename task to session terminology`

**POSLEDNÍ FÁZE** — dělat až po stabilizaci předchozích fází.

## Rozsah

### Kód (Go)

| Změna | Soubor | Detail |
|-------|--------|--------|
| `TaskTool` → `SessionTool` | `internal/tools/model.go` | Struct + 15+ referencí across codebase |
| `workspace_task_id` → `workspace_session_id` | `internal/session/model.go` | JSON tag + Go field `WorkspaceTaskID` → `WorkspaceSessionID` |
| `taskID` → `sessionID` | `internal/workspace/manager.go` | Parametry funkcí (interní, ne API) |
| Komentáře "task" → "session" | `internal/server/handlers/sessions.go` | "Canceller can cancel a running task" apod. |
| `TaskStepRef` | `internal/workflow/model.go` | Pokud přežil z fáze 4, přejmenovat |

### API (breaking change)

| Změna | Dopad |
|-------|-------|
| `{taskID}` route param | Všechny `/sessions/{taskID}/*` routes — **zvážit aliasing** |
| `workspace_task_id` JSON field | Request/response field — **breaking pro API klienty** |

### Dokumentace

| Soubor | Akce |
|--------|------|
| `README.md` | Přepsat na session-first model |
| `docs/api.md` | Nahradit všechny `/api/v1/tasks` → `/api/v1/sessions`, "Tasks" → "Sessions" |
| `docs/architecture.md` | Systematický replace "task" → "session" |
| `api/openapi.yaml` | Aktualizovat schémata a paths |

### UI

| Soubor | Akce |
|--------|------|
| `src/pages/SessionList.tsx` | Odstranit `workflow_run_id` linky |
| `src/pages/SessionDetail.tsx` | Odstranit `workflow_run_id` link |
| `src/types/session.ts` | Rename `workspace_task_id` → `workspace_session_id` |

## Poznámky

- **Metriky** (`TasksInProgress`, `TaskDuration`, `TasksTotal`) — nechat. Rename metrik rozbije dashboardy.
- **DB sloupce** — nemigrovat. `workflow_run_id` v sessions tabulce nechat.
- **Route param `{taskID}`** — ideálně přejmenovat na `{sessionID}`, ale zvážit jestli to stojí za breaking change. Chi router to bere jako pattern variable, UI a klienti to nevidí v URL.

## Ověření

```bash
task build
task test
task lint
# Prohledat codebase: grep -r "task" --include="*.go" | grep -v _test | grep -vi "context\|Taskfile\|tasks/"
```
