# 02 — Multi-Agent Workflow Pipeline

## Problém

Stávající workflow systém řetězí kroky (fetch → session → action), ale všechny session kroky jsou generické — nemají roli, nesledují kontext předchozích agentů, a nedokáží simulovat spolupráci týmu (plánování → implementace → review → testování).

## Cíl

Rozšířit workflow engine o podporu **multi-agent pipelines** — sekvencí sessions s různými rolemi, kde každý krok vidí výstup předchozích agentů a pracuje ve sdíleném workspace.

## Návrh

### Nový workflow step typ: `agent_session`

Rozšíření stávajícího `session` step typu (nebo aliasu) s explicitní vazbou na `AgentRole`:

```go
type AgentSessionConfig struct {
    SessionStepConfig          // embed existující config
    Role             string   // název AgentRole
    DependsOn        []string // step names, jejichž výstup se předá jako kontext
    ContextTemplate  string   // šablona pro sestavení kontextu z outputs předchozích kroků
}
```

### Sdílení workspace mezi kroky

Stávající `WorkspaceTaskRef` již umožňuje jednomu kroku navázat na workspace jiného. Toto rozšířit na **chain** — pipeline kroků sdílejících jeden workspace:

```
planner (vytvoří workspace)
  → implementer (WorkspaceTaskRef: "planner")
    → reviewer (WorkspaceTaskRef: "implementer", read-only)
      → tester (WorkspaceTaskRef: "implementer")
```

### Předávání kontextu mezi agenty

Každý step v pipeline vidí:
1. **Výstupy předchozích kroků** — result text, changes summary, review result
2. **Sdílený workspace** — aktuální stav souborů
3. **Pipeline metadata** — číslo sprintu, story ID, celkový plán

Kontext se sestaví z `DependsOn` references + `ContextTemplate` a prepend před prompt agenta.

### Built-in pipeline: `agile-team`

```yaml
name: agile-team
description: "Full agile cycle — plan, implement, review, test, create PR"
steps:
  - name: plan
    type: agent_session
    role: planner
    prompt: "Analyze the repository and create an implementation plan for: {{.Params.task_description}}"

  - name: implement
    type: agent_session
    role: implementer
    workspace_ref: plan
    depends_on: [plan]
    prompt: |
      Implement the following plan:
      {{.Steps.plan.result}}

  - name: review
    type: agent_session
    role: reviewer
    workspace_ref: implement
    depends_on: [implement]
    prompt: |
      Review the code changes made by the implementer.
      Original plan: {{.Steps.plan.result}}

  - name: fix_review_issues
    type: agent_session
    role: implementer
    workspace_ref: implement
    depends_on: [review]
    condition: "{{if eq .Steps.review.review_result.verdict 'request_changes'}}true{{end}}"
    prompt: |
      Fix the issues found in code review:
      {{.Steps.review.result}}

  - name: test
    type: agent_session
    role: tester
    workspace_ref: implement
    depends_on: [implement, review]
    prompt: |
      Write and run tests for the implemented changes.
      Plan: {{.Steps.plan.result}}

  - name: create_pr
    type: action
    kind: create_pr
    task_step_ref: implement
```

### Conditional steps

Přidat podporu pro podmíněné kroky v pipeline:
- `condition` — Go template expression, krok se přeskočí pokud vyhodnotí na prázdný string
- Use case: opravný krok po review se spustí jen pokud review vrátí `request_changes`

### Paralelní kroky (budoucnost)

V první fázi sekvenční. Do budoucna:
- `parallel_group` — kroky ve skupině běží souběžně
- Use case: reviewer + tester + security-auditor mohou běžet paralelně

## Dotčené soubory

- `internal/workflow/model.go` — rozšířit `StepDefinition` o role, depends_on, condition
- `internal/workflow/orchestrator.go` — kontext předávání, podmíněné kroky
- `internal/workflow/step_session.go` — aplikovat roli, sestavit kontext
- `internal/workflow/builtins.go` — přidat `agile-team` pipeline
- `internal/worker/executor.go` — podpora role system promptu

## Otevřené otázky

- Max délka pipeline? (guard proti nekonečným loops)
- Jak řešit selhání uprostřed pipeline? (retry celého kroku vs. pokračovat)
- Budget management — max_budget_usd pro celý pipeline vs. per-step?
