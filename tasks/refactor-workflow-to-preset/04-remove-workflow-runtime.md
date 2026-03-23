# Fáze 4: Smazat workflow runtime engine

**Commit:** `refactor: remove workflow runtime engine`

## Prerequisite

Fáze 3 musí být hotová — preset run už vytváří session přímo. Orchestrator je mrtvý kód.

## Soubory ke smazání

### Runtime engine
| Soubor | LOC | Důvod |
|--------|-----|-------|
| `internal/workflow/orchestrator.go` | 392 | Celý BLPOP loop, step dispatch, state management |
| `internal/workflow/orchestrator_test.go` | 57 | |
| `internal/workflow/orchestrator_integration_test.go` | 105 | |
| `internal/workflow/step_fetch.go` | 170 | HTTP fetch step — logika patří klientovi |
| `internal/workflow/step_fetch_test.go` | 147 | |
| `internal/workflow/step_action.go` | 82 | PR create action — už existuje v session API |
| `internal/workflow/step_action_test.go` | 92 | |
| `internal/workflow/step_session.go` | 220 | Mapovací logika přesunuta do preset.go (fáze 3) |
| `internal/workflow/streamer.go` | 75 | Workflow-specific Redis Pub/Sub |

### Persistence (workflow runs)
| Soubor | LOC | Důvod |
|--------|-----|-------|
| `internal/workflow/run_store.go` | 13 | RunStore interface |
| `internal/workflow/sqlite_run_store.go` | 221 | SQLite implementace |
| `internal/workflow/sqlite_run_store_test.go` | 178 | |

### Model cleanup
Soubor `internal/workflow/model.go` — **ponechat, ale osekat**:

Smazat:
- `StepType` konstanty (`StepTypeFetch`, `StepTypeAction`) — ponechat jen `StepTypeSession`
- `RunStatus` + všechny konstanty (pending, running, completed, failed, canceled)
- `StepStatus` + všechny konstanty
- `ActionKind` + `ActionCreatePR`
- `FetchConfig` struct
- `ActionConfig` struct
- `WorkflowRun` struct
- `WorkflowRunStep` struct

Ponechat:
- `WorkflowDefinition` struct (šablona)
- `StepDefinition` struct (popis session stepu v šabloně)
- `SessionStepConfig` struct (konfigurace session stepu)
- `ParameterDefinition` struct
- `WorkflowConfig` struct (preset)
- `MarshalMapJSON` / `UnmarshalMapJSON` helpers

### API handler
| Soubor | Akce |
|--------|------|
| `internal/server/handlers/workflow.go` | **Osekat** — smazat `RunWorkflow`, `ListRuns`, `GetRun`, `CancelRun`, `CancelAllRuns`, `StreamRun`. Ponechat `CreateWorkflow`, `ListWorkflows`, `GetWorkflow`, `DeleteWorkflow` (CRUD šablon) |

Smazat z workflow.go:
- `WorkflowRunCreator` interface
- `WorkflowRunCanceller` interface
- Pole `runCreator`, `runCanceller`, `sessionCanceller` z handleru
- `RunWorkflow()` metoda
- `ListRuns()` metoda
- `GetRun()` metoda
- `CancelRun()` metoda
- `CancelAllRuns()` metoda
- `StreamRun()` metoda

Handler bude potřebovat jen: `registry workflow.Registry`

## DB tabulky — NECHAT

**Nemaž migrace ani tabulky:**
- `workflow_runs` — nechat ležet
- `workflow_run_steps` — nechat ležet
- `workflows` — **stále se používá** (ukládá sentry-fixer definici)

## Celkem smazáno

~1 550 LOC produkčního kódu + ~580 LOC testů = **~2 130 LOC**

## Ověření

```bash
task build    # musí projít bez orchestrator importů
task test     # všechny testy musí projít
task lint     # žádné unused importy
```
