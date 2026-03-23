# Fáze 3: Preset run přímo vytváří session

**Commit:** `feat: preset run creates session directly`

## Cíl

`POST /workflow-configs/{id}/run` přestane volat orchestrator a místo toho přímo vytvoří session. Vrátí `session_id` místo `run_id`.

## Současný flow

```
workflow_config.go Run()
  → h.runCreator.CreateRun(cfg.Workflow, params)  // orchestrator
    → Redis RPUSH queue:workflows
      → orchestrator BLPOP
        → SessionExecutor.Execute()
          → sessionService.Create()
```

## Cílový flow

```
workflow_config.go Run()
  → lookup workflow definition (registry)
  → render prompt template with params
  → resolve tool key ref
  → sessionService.Create()
  → return session_id
```

## Kroky

### 1. Rozšířit WorkflowConfigHandler dependencies

Soubor: `internal/server/handlers/workflow_config.go`

Handler dnes závisí na:
- `workflow.ConfigStore` — zůstane
- `WorkflowRunCreator` — **nahradit** za:
  - `workflow.Registry` — lookup workflow definice (prompt šablona)
  - `session.Service` (nebo interface `SessionCreator`) — vytvoření session
  - `keys.Registry` — resolve tool key ref

### 2. Přesunout mapovací logiku ze step_session.go

Z `internal/workflow/step_session.go` (řádky 44-147) extrahovat do nové funkce v workflow balíčku:

```go
// BuildSessionRequest builds a CreateSessionRequest from a workflow
// definition, preset params, and key registry.
func BuildSessionRequest(ctx context.Context, def WorkflowDefinition, params map[string]string, keyReg keys.Registry) (*session.CreateSessionRequest, error)
```

Tato funkce:
1. Najde první `session` step v definici
2. Renderuje prompt šablonu s params (template.go)
3. Resolve tool key ref z key registry
4. Složí `session.CreateSessionRequest`

**Neřeší:** WorkspaceTaskRef (to je multi-step feature, tu mažeme), workflow RunID

### 3. Přepsat Run() handler

Soubor: `internal/server/handlers/workflow_config.go`

```go
func (h *WorkflowConfigHandler) Run(w http.ResponseWriter, r *http.Request) {
    // 1. Lookup preset
    cfg, err := h.store.Get(r.Context(), id)
    // 2. Lookup workflow definition (template)
    def, err := h.registry.Get(r.Context(), cfg.Workflow)
    // 3. Build session request
    req, err := workflow.BuildSessionRequest(r.Context(), *def, cfg.Params, h.keys)
    // 4. Apply timeout if configured
    if cfg.TimeoutSeconds > 0 { req.Config.TimeoutSeconds = cfg.TimeoutSeconds }
    // 5. Create session
    sess, err := h.sessions.Create(r.Context(), *req)
    // 6. Return session_id
    writeJSON(w, http.StatusCreated, map[string]interface{}{
        "session_id": sess.ID,
        "config_id":  cfg.ID,
        "config_name": cfg.Name,
    })
}
```

### 4. Přidat compat endpoint `POST /workflows/{name}/run`

Soubor: `internal/server/handlers/workflow.go`

Stávající `RunWorkflow()` volá orchestrator. Přepsat na thin wrapper:

```go
func (h *WorkflowHandler) RunWorkflow(w http.ResponseWriter, r *http.Request) {
    name := chi.URLParam(r, "name")
    var body struct {
        Params map[string]string `json:"params"`
    }
    json.NewDecoder(r.Body).Decode(&body)

    def, err := h.registry.Get(r.Context(), name)
    req, err := workflow.BuildSessionRequest(r.Context(), *def, body.Params, h.keys)
    sess, err := h.sessions.Create(r.Context(), *req)

    writeJSON(w, http.StatusCreated, map[string]interface{}{
        "session_id": sess.ID,
        "workflow_name": name,
    })
}
```

Tím UI nemusí měnit volání ihned — `POST /workflows/{name}/run` funguje dál, jen vrací `session_id` místo `run_id`.

### 5. Upravit response

Oba endpointy (`/workflow-configs/{id}/run` i `/workflows/{name}/run`) vrací:

```json
{"session_id": "...", "config_id": 1, "config_name": "my-sentry-preset"}
```

respektive:

```json
{"session_id": "...", "workflow_name": "sentry-fixer"}
```

UI po run přesměruje na `/sessions/{session_id}` místo `/workflows/runs/{run_id}`.

## Soubory (backend)

| Soubor | Akce |
|--------|------|
| `internal/workflow/preset.go` | **Nový** — `BuildSessionRequest()` funkce |
| `internal/server/handlers/workflow_config.go` | Přepsat `Run()`, změnit dependencies |
| `internal/server/handlers/workflow.go` | Přepsat `RunWorkflow()` na compat wrapper |
| `internal/server/server.go` | Upravit handler inicializaci — nové deps |
| `cmd/codeforge/main.go` | Předat nové deps do server.New() |

## UI contract change (musí jít SPOLEČNĚ s backend změnou)

Backend mění response z `{ run_id, ... }` na `{ session_id, ... }`. UI musí být upraveno současně, jinak se rozbije po kliknutí na Run.

| Soubor | Akce |
|--------|------|
| `codeforge-ui/src/lib/api.ts:171` | `runWorkflow` — přestat vracet `WorkflowRun`, nový response type `{ session_id: string, workflow_name: string }` |
| `codeforge-ui/src/lib/api.ts:197` | `runWorkflowConfig` — přestat vracet `{ run_id }`, nový response type `{ session_id: string, config_id: number, config_name: string }` |
| `codeforge-ui/src/hooks/useWorkflowMutations.ts:23` | `useRunWorkflow` — zůstane, ale s novým response typem |
| `codeforge-ui/src/pages/WorkflowDetail.tsx:127` | Redirect `→ /sessions/${result.session_id}` |
| `codeforge-ui/src/pages/WorkflowList.tsx:77` | Redirect `→ /sessions/${data.session_id}` |
| `codeforge-ui/src/components/SentryFixerRunForm.tsx:133` | Redirect `→ /sessions/${run.session_id}` |

**Pozn.:** SentryTab.tsx:679 (`setFixingIds`) a SentryTab inline run tracking se řeší v 05a — tady jen zajistíme, že run endpointy vrací `session_id` a základní redirecty fungují.

## Ověření

```bash
task build
task test
# POST /workflow-configs/{id}/run → vrátí session_id
# POST /workflows/{name}/run → vrátí session_id (compat)
# Session se objeví v GET /sessions a spustí se normálně
# UI: klik na Run v WorkflowDetail/WorkflowList/SentryFixerRunForm → redirect na /sessions/{id}
```
