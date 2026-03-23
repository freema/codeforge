# Refactoring: Workflow Runtime → Preset Model

## Cílový model (jedna věta)

Workflow/preset jen připraví repo + prompt + tools + provider keys + metadata a přímo vytvoří session. Všechna skutečná práce běží jen v session.

## Co zůstane

- **Workflow definice** jako read-only šablony (builtin sentry-fixer)
- **Workflow configs** jako presety — uložená konfigurace, kterou můžeš spouštět opakovaně (i cronem)
- `POST /workflow-configs/{id}/run` → přímo vrátí `session_id`
- Template rendering pro prompt parametrizaci
- Config store (SQLite `workflow_configs` tabulka)
- Registry pro workflow definice (SQLite `workflows` tabulka)

## Co se maže

- Orchestrator (Redis queue `codeforge:queue:workflows`, BLPOP loop)
- Workflow runs tracking (SQLite `workflow_runs`, `workflow_run_steps` tabulky — **tabulky nechat, jen přestat používat**)
- Step executory: fetch, action, session (mapovací logiku ze session přesunout do handleru)
- Workflow streaming (Redis Pub/Sub `workflow:{runID}:stream`, SSE endpoint)
- API endpointy: `/workflow-runs/*`
- Built-in workflows: github-issue-fixer, gitlab-issue-fixer, knowledge-update (definice)
- UI (3 vrstvy):
  - **Run API contract**: response typy v `api.ts`, `types/workflow.ts`, `types/index.ts`
  - **Workflow runtime UI**: WorkflowRunDetail stránka, route, useWorkflowRuns hook, workflow stream, cancel mutace
  - **Workflow-adjacent UI**: WorkflowList runs tab + cancel UI, WorkflowDetail runs sekce, SentryFixerRunForm redirect, SentryTab (live tracking + run history)

## Co zůstane (compat)

- `POST /workflows/{name}/run` — compat endpoint, uvnitř jen vytvoří session a vrátí `session_id`
- UI workflow stránky (List, Detail, Create) — pracují s presety, ne s runtime (ale vyžadují úpravy — viz 05a)

## Fáze

| # | Fáze | Commit |
|---|------|--------|
| 1 | Přesunout knowledge-update prompty do `internal/prompt/` | `refactor: move knowledge prompts to prompt package` |
| 2 | Osekat builtins jen na sentry-fixer | `refactor: remove non-sentry builtin workflows` |
| 3 | Přepsat oba run endpointy na přímé vytvoření session + UI contract change | `feat: preset run creates session directly` |
| 5a | UI migrace — 3 vrstvy: API contract, runtime UI, workflow-adjacent (List, Detail, SentryTab) | `refactor: migrate UI to session-first model` |
| 4 | Smazat orchestrator, workflow-runs, step executory, streaming | `refactor: remove workflow runtime engine` |
| 5 | Vyčistit main.go wiring a server.go routes | `refactor: remove workflow runtime wiring` |
| 6 | Přepnout MCP/tool setup na fail-closed | `fix: fail session when tool/MCP setup fails` |
| 7 | (Samostatně, poslední) Rename task → session terminologie | `refactor: rename task to session terminology` |
| 8 | Smazat Redis input listener (mrtvý kód) | `refactor: remove unused Redis input listener` |

## DB migrace

**NEMAŽ** již aplikované migrace. Tabulky `workflow_runs`, `workflow_run_steps`, `workflows` nechat ležet. Sloupec `workflow_run_id` v `sessions` tabulce nechat (bude prázdný).

## Rozhodnutí (2026-03-23)

**Nechat beze změny:**
- Session types `plan` a `review` — jsou to prompt šablony, runtime je stejný jako `code`
- Oba review mechanismy — `session_type: "review"` (review cizího PR/MR) vs `POST /sessions/{id}/review` (review vlastních změn z CodeForge) — různé use cases
- Public `/workspaces` endpointy — UI je používá (Settings → WorkspacesTab)
- Built-in tools catalog (sentry, jira, git, github, playwright) — UI je zobrazuje všechny
- CI Action — aktivně udržovaný, 100% core
- Webhook receiver — auto-review na PR/MR, core feature
- `callback_url` — integrační API, funkční
- Codex CLI — nechat i s tichým ignorováním MaxTurns/MaxBudgetUSD/AllowedTools

**Smazat:**
- Redis input listener (`input:sessions`) — žádný producent, mrtvý kód
- `workflow_run_id` field — po smazání orchestratoru (fáze 7)
- `workspace_task_id` → `workspace_session_id` — rename (fáze 7)

**Otevřené rozhodnutí:**
- SentryTab (fáze 5a): varianta A (přepsat na session stream + session IDs) vs varianta B (zjednodušit — smazat inline progress + history, jen spuštění + redirect na session detail)
