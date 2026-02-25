# Phase 10: Multi-Agent Orchestration

## Cíl

Umožnit **pipeline více AI agentů** na jednom tasku. Například:
1. Claude Code napíše kód
2. Codex CLI udělá review a navrhne úpravy
3. Claude Code aplikuje úpravy
4. Výsledek se commitne

## Závislosti

- **Phase 7: Tool System** — agenti potřebují přístup k toolům
- **Phase 9: Multi-CLI** — potřebujeme více CLI runners

## Koncepty

### Agent vs CLI

- **CLI Runner** = konkrétní AI nástroj (Claude Code, Codex, Aider)
- **Agent** = CLI Runner + role + konfigurace + kontext

```go
type Agent struct {
    Name     string   `json:"name"`      // unikátní identifikátor v pipeline
    CLI      string   `json:"cli"`       // "claude-code", "codex", "aider"
    Role     string   `json:"role"`      // "coder", "reviewer", "tester"
    Model    string   `json:"model"`     // model override
    Prompt   string   `json:"prompt"`    // system/role prompt
    Tools    []string `json:"tools"`     // povolené tooly
    MaxTurns int      `json:"max_turns"` // limit
}
```

### Pipeline

Pipeline definuje sekvenci agentů a jak si předávají kontext:

```go
type Pipeline struct {
    Name   string          `json:"name"`
    Agents []PipelineAgent `json:"agents"`
}

type PipelineAgent struct {
    Agent      Agent           `json:"agent"`
    DependsOn  []string        `json:"depends_on,omitempty"`  // jména předchozích agentů
    InputFrom  string          `json:"input_from,omitempty"`  // odkud bere kontext
    Condition  *StepCondition  `json:"condition,omitempty"`   // podmíněné spuštění
}

type StepCondition struct {
    Type  string `json:"type"`   // "has_changes", "review_passed", "always"
    Value string `json:"value"`  // parametr podmínky
}
```

## Architektura

```
┌──────────────────────────────────────────────────────────────┐
│                     TASK REQUEST                              │
│  {                                                            │
│    "repo_url": "...",                                         │
│    "prompt": "Oprav bug v login flow",                        │
│    "pipeline": {                                              │
│      "agents": [                                              │
│        {"name": "coder", "cli": "claude-code",                │
│         "role": "Implementuj opravu"},                        │
│        {"name": "reviewer", "cli": "codex",                   │
│         "role": "Review kódu, navrhni zlepšení",              │
│         "depends_on": ["coder"]},                             │
│        {"name": "fixer", "cli": "claude-code",                │
│         "role": "Aplikuj review feedback",                    │
│         "depends_on": ["reviewer"],                           │
│         "condition": {"type": "has_suggestions"}}             │
│      ]                                                        │
│    }                                                          │
│  }                                                            │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│                PIPELINE ORCHESTRATOR                           │
│                                                               │
│  Step 1: CODER (claude-code)                                  │
│    ├─ Clone repo                                              │
│    ├─ Run: "Oprav bug v login flow"                           │
│    ├─ Result: kód opraven, 3 soubory změněny                  │
│    └─ Output → shared context                                 │
│                                                               │
│  Step 2: REVIEWER (codex)                                     │
│    ├─ Workspace: stejný (se změnami od codera)                │
│    ├─ Run: "Review změn + diff od codera"                     │
│    ├─ Result: "2 suggestions: error handling, naming"         │
│    └─ Output → shared context                                 │
│                                                               │
│  Step 3: FIXER (claude-code) [podmíněný]                      │
│    ├─ Condition: has_suggestions? → ANO                       │
│    ├─ Workspace: stejný                                       │
│    ├─ Run: "Aplikuj: {reviewer suggestions}"                  │
│    └─ Result: finální kód                                     │
│                                                               │
│  → Composite result + all agent outputs                       │
└──────────────────────────────────────────────────────────────┘
```

## Datový model

### Pipeline v tasku

```go
// Rozšíření Task modelu
type Task struct {
    // ... existující pole ...

    Pipeline       *Pipeline        `json:"pipeline,omitempty"`
    CurrentAgent   string           `json:"current_agent,omitempty"`
    AgentResults   []AgentResult    `json:"agent_results,omitempty"`
}

type AgentResult struct {
    AgentName    string       `json:"agent_name"`
    CLI          string       `json:"cli"`
    Status       string       `json:"status"`  // "completed", "failed", "skipped"
    Result       string       `json:"result"`
    Changes      *ChangesSummary `json:"changes,omitempty"`
    Usage        *UsageInfo   `json:"usage,omitempty"`
    Duration     float64      `json:"duration_seconds"`
    StartedAt    time.Time    `json:"started_at"`
    FinishedAt   time.Time    `json:"finished_at"`
}
```

### Redis klíče

| Klíč | Typ | Popis |
|------|-----|-------|
| `task:{id}:pipeline` | Hash | Pipeline definice |
| `task:{id}:agent:{name}:result` | String | Výsledek konkrétního agenta |
| `task:{id}:agent:{name}:state` | Hash | Stav agenta (status, timing) |
| `task:{id}:context` | Hash | Sdílený kontext mezi agenty |

## Sdílený kontext

Agenti v pipeline sdílejí:

1. **Workspace** — stejný git workspace (změny se akumulují)
2. **Shared context** — Redis hash s výsledky předchozích agentů
3. **Diff kontext** — každý agent vidí diff od předchozího

```go
type SharedContext struct {
    TaskPrompt     string                     // originální task prompt
    AgentOutputs   map[string]string          // agent_name → result text
    AccumulatedDiff string                    // git diff od začátku
    Metadata       map[string]string          // libovolná metadata
}
```

### Prompt building pro agenta v pipeline

```
## Task Context
Original task: {task.Prompt}

## Previous Agent Results

### Agent: coder (claude-code)
Status: completed
Result: {truncated result}
Changes: 3 files modified, +142 -38

### Agent: reviewer (codex)
Status: completed
Result: {truncated result}

## Your Role
{agent.Role}

## Current Instruction
{agent.Prompt or task.Prompt}

## Workspace State
The workspace contains changes from previous agents. Use `git diff` to see current state.
```

## Nové soubory

```
internal/pipeline/
  model.go         — Pipeline, PipelineAgent, AgentResult, SharedContext
  orchestrator.go  — Pipeline orchestrátor (sekvenční + DAG execution)
  context.go       — Sdílený kontext management (Redis)
  prompt.go        — Prompt building pro agenty v pipeline
  validator.go     — Validace pipeline definic

internal/server/handlers/
  pipeline.go      — Pipeline-specific endpoints (pokud potřeba)
```

## Tasky

### 10.1 — Datový model
- [ ] Vytvořit `internal/pipeline/model.go` — Pipeline, Agent, AgentResult, SharedContext
- [ ] Rozšířit `Task` model o pipeline pole
- [ ] Rozšířit `CreateTaskRequest` o pipeline definici
- [ ] Validace: pipeline agents mají validní CLI names

### 10.2 — Pipeline Orchestrator
- [ ] Vytvořit `internal/pipeline/orchestrator.go`
- [ ] Sekvenční execution: agent po agentovi dle `depends_on`
- [ ] Podmíněné spuštění: evaluace `StepCondition`
- [ ] Error handling: co dělat když agent uprostřed selže
- [ ] Timeout per agent + celkový pipeline timeout
- [ ] Unit testy

### 10.3 — Shared Context
- [ ] Vytvořit `internal/pipeline/context.go`
- [ ] Redis storage: `task:{id}:context` hash
- [ ] Čtení/zápis agent výsledků do kontextu
- [ ] Akumulace diffů
- [ ] Cleanup po dokončení pipeline

### 10.4 — Prompt Builder
- [ ] Vytvořit `internal/pipeline/prompt.go`
- [ ] Formátování kontextu předchozích agentů
- [ ] Truncation (max context chars per agent)
- [ ] Role-specific prompt prefixes
- [ ] Unit testy

### 10.5 — Executor integrace
- [ ] Upravit `worker/executor.go` — detekce pipeline tasku
- [ ] Pipeline task → delegovat na `pipeline.Orchestrator`
- [ ] Non-pipeline task → beze změn (backward compatible)
- [ ] Stream events: `agent_started`, `agent_completed`, `pipeline_progress`

### 10.6 — Stream Events
- [ ] Nový event type: `"pipeline"`
- [ ] Events: `pipeline_started`, `agent_started`, `agent_completed`, `agent_skipped`, `pipeline_completed`
- [ ] Metadata v eventu: current agent, progress (2/3), agent result summary

### 10.7 — Pre-built Pipelines (Templates)
- [ ] "code-and-review" — coder + reviewer
- [ ] "code-review-fix" — coder + reviewer + fixer
- [ ] "multi-model" — same prompt, dvě CLI, porovnání
- [ ] API: `GET /api/v1/pipelines/templates`
- [ ] Task shortcut: `"pipeline": "code-and-review"` místo plné definice

### 10.8 — Validace a limity
- [ ] Max agentů v pipeline (default: 5)
- [ ] Max celkový timeout pipeline
- [ ] Cyklus detection v depends_on grafu
- [ ] Validace CLI availability pro všechny agenty

### 10.9 — Integration testy
- [ ] Test: 2-agent pipeline (coder → reviewer)
- [ ] Test: podmíněný agent (skipped)
- [ ] Test: agent failure → pipeline stop
- [ ] Test: backward compatibility (task bez pipeline)

## Predefined Roles

```go
var PredefinedRoles = map[string]string{
    "coder": `You are a code implementation agent. Write clean, tested code
              that solves the given task. Focus on correctness and simplicity.`,

    "reviewer": `You are a code review agent. Review the changes in this workspace.
                 Look for: bugs, security issues, performance problems, code style.
                 Provide specific, actionable suggestions.`,

    "tester": `You are a testing agent. Write tests for the changes in this workspace.
               Focus on edge cases and error scenarios. Use the project's existing
               test framework and conventions.`,

    "documenter": `You are a documentation agent. Update documentation based on
                   the code changes. Update README, API docs, inline comments
                   where needed.`,
}
```

## Testovací strategie

### Orchestrator testy (`internal/pipeline/orchestrator_test.go`)

Orchestrator je klíčová logika — musí být důkladně otestovaný s mock runners:

```go
// Mock executor pro pipeline testy (nepotřebuje reálné CLI):
type mockAgentExecutor struct {
    results map[string]*AgentResult  // agent name → result
    errors  map[string]error
}
```

- [ ] `TestOrchestrator_SingleAgent` — 1 agent → execute → complete
- [ ] `TestOrchestrator_TwoAgents_Sequential` — coder → reviewer, oba complete
- [ ] `TestOrchestrator_ThreeAgents_DependsOn` — coder → reviewer → fixer
- [ ] `TestOrchestrator_ConditionTrue` — podmínka splněna → agent běží
- [ ] `TestOrchestrator_ConditionFalse` — podmínka nesplněna → agent skipped
- [ ] `TestOrchestrator_AgentFailure_StopsPipeline` — agent #2 selže → #3 neběží
- [ ] `TestOrchestrator_Timeout_PerAgent` — agent překročí timeout → fail
- [ ] `TestOrchestrator_Timeout_Total` — celkový pipeline timeout
- [ ] `TestOrchestrator_CancellationPropagation` — cancel ctx → všichni agenti se zastaví
- [ ] `TestOrchestrator_EmptyPipeline` — prázdná pipeline → error

### Validace testy (`internal/pipeline/validator_test.go`)

- [ ] `TestValidator_ValidPipeline` — korektní pipeline → no error
- [ ] `TestValidator_DuplicateAgentNames` — dva agenti se stejným jménem → error
- [ ] `TestValidator_CyclicDependency` — A→B→A → error
- [ ] `TestValidator_MissingDependency` — depends_on neexistující agent → error
- [ ] `TestValidator_TooManyAgents` — >5 agentů → error (konfigurovatelné)
- [ ] `TestValidator_InvalidCLI` — neznámý CLI name → error
- [ ] `TestValidator_EmptyAgentName` — prázdné jméno → error

### Shared Context testy (`internal/pipeline/context_test.go` — miniredis)

- [ ] `TestSharedContext_StoreResult` — uloží agent result → přečte zpět
- [ ] `TestSharedContext_MultipleAgents` — výsledky více agentů se akumulují
- [ ] `TestSharedContext_Cleanup` — cleanup po pipeline smaže všechny klíče
- [ ] `TestSharedContext_AccumulateDiff` — diff se přidává, ne přepisuje

### Prompt Builder testy (`internal/pipeline/prompt_test.go`)

- [ ] `TestPromptBuilder_SingleAgent` — jen task prompt, žádný kontext
- [ ] `TestPromptBuilder_WithPreviousResults` — obsahuje předchozí výsledky
- [ ] `TestPromptBuilder_Truncation` — dlouhé výsledky se ořežou
- [ ] `TestPromptBuilder_RolePrefix` — role prompt na začátku
- [ ] `TestPromptBuilder_OutputFormat` — správný markdown formát

### Stream Events testy (miniredis)

- [ ] `TestStreamEvent_PipelineStarted` — event type "pipeline", event "pipeline_started"
- [ ] `TestStreamEvent_AgentStarted` — metadata s agent name a progress
- [ ] `TestStreamEvent_AgentCompleted` — metadata s result summary
- [ ] `TestStreamEvent_AgentSkipped` — metadata s důvodem

### Integration testy (`//go:build integration`)

- [ ] `TestIntegration_Pipeline_CodeAndReview` — mock CLI: coder + reviewer
- [ ] `TestIntegration_Pipeline_BackwardCompat` — task bez pipeline = stávající behavior
- [ ] `TestIntegration_Pipeline_Template` — shortcut "code-and-review" se rozbalí

## Linter checklist

- [ ] Goroutine leaks: žádné goroutiny bez cancel/cleanup
- [ ] Race conditions: `go test -race` musí projít
- [ ] Cyklomatická složitost orchestrátoru ≤15 (rozdělit do metod)
- [ ] `task fmt` + `task lint` MUSÍ projít

## Otevřené otázky

1. **Parallel agents** — povolit paralelní spuštění nezávislých agentů? (komplikuje sdílený workspace)
2. **Agent communication** — mohou agenti přímo komunikovat, nebo jen přes shared context?
3. **Rollback** — pokud pozdější agent selže, rollbacknout změny předchozích?
4. **Cost tracking** — per-agent usage tracking pro billing?
5. **Human-in-the-loop** — možnost pozastavit pipeline pro lidský review mezi agenty?
