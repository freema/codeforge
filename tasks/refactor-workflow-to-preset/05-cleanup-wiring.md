# Fáze 5: Vyčistit main.go wiring a server.go routes

**Commit:** `refactor: remove workflow runtime wiring`

## main.go (`cmd/codeforge/main.go`)

### Smazat (řádky 222-265)

```go
// Smazat celý blok:
workflowRunStore := workflow.NewSQLiteRunStore(sqliteDB.Unwrap())   // řádek 224
wfStreamer := workflow.NewStreamer(rdb, ...)                         // řádek 231
fetchExecutor := workflow.NewFetchExecutor(keyRegistry)             // řádek 232
sessionExecutor := workflow.NewSessionExecutor(...)                 // řádek 233
actionExecutor := workflow.NewActionExecutor(prService)             // řádek 234
orchestrator := workflow.NewOrchestrator(...)                       // řádky 236-248
go orchestrator.Start(appCtx)                                      // řádek 265
```

### Ponechat

```go
workflowRegistry := workflow.NewSQLiteRegistry(sqliteDB.Unwrap())   // řádek 223
workflowConfigStore := workflow.NewSQLiteConfigStore(sqliteDB.Unwrap()) // řádek 225
workflow.SeedBuiltins(context.Background(), workflowRegistry)       // řádek 227-229
```

### Upravit server.New() volání (řádek 256)

Odstranit parametry:
- `workflowRunStore` — smazáno
- `orchestrator` (jako `WorkflowRunCreator`) — smazáno
- `orchestrator` (jako `WorkflowRunCanceller`) — smazáno

Přidat parametry (pro preset run handler):
- `sessionService` — už se předává
- `keyRegistry` — už se předává

## server.go (`internal/server/server.go`)

### Upravit New() signaturu

Odstranit parametry:
- `workflowRunStore workflow.RunStore`
- `workflowRunCreator handlers.WorkflowRunCreator`
- `workflowRunCanceller handlers.WorkflowRunCanceller`

### Smazat routes (řádky 96-97, 162-175)

```go
// Smazat SSE stream:
r.Get("/workflow-runs/{runID}/stream", workflowHandler.StreamRun)   // řádek 97

// Smazat celý /workflow-runs route block:
r.Route("/workflow-runs", func(r chi.Router) {                      // řádky 170-175
    r.Get("/", workflowHandler.ListRuns)
    r.Post("/cancel-all", workflowHandler.CancelAllRuns)
    r.Get("/{runID}", workflowHandler.GetRun)
    r.Post("/{runID}/cancel", workflowHandler.CancelRun)
})

// PONECHAT run endpoint jako compat vrstvu (přepsaný v fázi 3):
// r.Post("/{name}/run", workflowHandler.RunWorkflow)  // → vrátí session_id
```

### Upravit handler inicializaci

```go
// Starý:
workflowHandler := handlers.NewWorkflowHandler(workflowRegistry, workflowRunStore,
    workflowRunCreator, workflowRunCanceller, canceller.Cancel, redis)
workflowConfigHandler := handlers.NewWorkflowConfigHandler(workflowConfigStore, workflowRunCreator)

// Nový:
workflowHandler := handlers.NewWorkflowHandler(workflowRegistry)
workflowConfigHandler := handlers.NewWorkflowConfigHandler(
    workflowConfigStore, workflowRegistry, sessionService, keyRegistry)
```

### Ponechat routes

```go
// Workflow šablony CRUD + compat run:
r.Route("/workflows", func(r chi.Router) {
    r.Post("/", workflowHandler.CreateWorkflow)
    r.Get("/", workflowHandler.ListWorkflows)
    r.Get("/{name}", workflowHandler.GetWorkflow)
    r.Delete("/{name}", workflowHandler.DeleteWorkflow)
    r.Post("/{name}/run", workflowHandler.RunWorkflow)  // compat — vrátí session_id
})

// Preset CRUD + run:
r.Route("/workflow-configs", func(r chi.Router) {
    r.Post("/", workflowConfigHandler.Create)
    r.Get("/", workflowConfigHandler.List)
    r.Get("/{id}", workflowConfigHandler.Get)
    r.Delete("/{id}", workflowConfigHandler.Delete)
    r.Post("/{id}/run", workflowConfigHandler.Run)  // → vrátí session_id
})
```

### Vyčistit SSE bypass

```go
// V řádku 197 — odstranit check pro workflow stream:
// Zůstane jen session stream check (sessions/{taskID}/stream)
```

## Config

Zkontrolovat `internal/config/config.go` — pokud existuje `Workflow` config sekce s `ContextTTLHours` a `MaxRunDurationSec`, můžeme ji smazat (orchestrator ji spotřebovává).

## Ověření

```bash
task build
task test
task lint
```
