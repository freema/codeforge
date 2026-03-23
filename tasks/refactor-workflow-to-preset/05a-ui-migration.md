# Fáze 5a: UI migrace na session-first

**Prerequisite:** Fáze 3 hotová (oba run endpointy vrací `session_id`, základní redirecty fungují)

## Princip

UI zachová koncept "workflow" pro uživatele, ale interně přejde na session. Tři vrstvy změn: API contract cleanup, workflow runtime UI, workflow-adjacent komponenty.

---

## Vrstva 1: Run API contract cleanup

### types

| Soubor | Akce |
|--------|------|
| `src/types/workflow.ts` | Smazat `WorkflowRun`, `WorkflowRunStep`, `RunStatus`, `StepStatus` typy. Přidat nový response type pro run endpoint (`{ session_id: string, ... }`) |
| `src/types/index.ts:24` | Smazat re-exporty: `RunStatus`, `StepStatus`, `WorkflowRunStep`, `WorkflowRun` |
| `src/types/session.ts` | Smazat `workflow_run_id` field ze `Session` interface |

### api.ts

| Soubor | Řádky | Akce |
|--------|-------|------|
| `src/lib/api.ts:171` | `runWorkflow` | Změnit return type z `WorkflowRun` na `{ session_id: string, workflow_name: string }` |
| `src/lib/api.ts:173-184` | `listWorkflowRuns`, `getWorkflowRun`, `cancelWorkflowRun`, `cancelAllWorkflowRuns` | **Smazat** celý Workflow Runs blok |
| `src/lib/api.ts:197` | `runWorkflowConfig` | Změnit return type z `{ run_id }` na `{ session_id, config_id, config_name }` |
| `src/lib/api.ts:1-25` | importy | Odstranit `WorkflowRun` z importu |

---

## Vrstva 2: Workflow runtime UI (smazat)

| Soubor | Akce |
|--------|------|
| `src/pages/WorkflowRunDetail.tsx` | **Smazat** celý soubor (267 řádků) |
| `src/App.tsx:28-31` | Smazat route `/workflows/runs/:runId` → `WorkflowRunDetail`, smazat import. Volitelně přidat redirect `/workflows/runs/*` → `/sessions` pro staré bookmarky |
| `src/hooks/useWorkflowRuns.ts` | **Smazat** celý soubor (103 řádků) — `useWorkflowRuns`, `useWorkflowRun`, `useWorkflowRunStream` |
| `src/lib/stream.ts:115-201` | Smazat `WorkflowStreamHandlers` interface a `connectToWorkflowRunStream()` funkci |
| `src/hooks/useWorkflowMutations.ts:34-59` | Smazat `useCancelWorkflowRun` a `useCancelAllWorkflowRuns`. Ponechat `useRunWorkflow` (s novým response typem) a `useDeleteWorkflow` |

---

## Vrstva 3: Workflow-adjacent UI (musí se upravit, NE "beze změny")

### WorkflowList (`src/pages/WorkflowList.tsx`)

Má dvoutabový layout (`configs | runs`). Po refactoru zůstane jen configs/presets.

| Řádky | Co | Akce |
|-------|-----|------|
| 4 | `import { useWorkflowRuns }` | Smazat |
| 6-8 | `import { useCancelWorkflowRun, useCancelAllWorkflowRuns }` | Smazat |
| 12 | `import type { WorkflowRun, RunStatus }` | Smazat |
| 14-15 | `type Tab = "configs" \| "runs"` | Smazat — zůstane jen configs |
| 17-42 | `runStatusColors` | Smazat |
| 63-68 | `useWorkflowRuns()`, `useCancelWorkflowRun()`, `useCancelAllWorkflowRuns()` | Smazat |
| 77 | `void navigate(\`/workflows/runs/${data.run_id}\`)` | Změnit na `void navigate(\`/sessions/${data.session_id}\`)` |
| 92-94 | `activeRuns` filter | Smazat |
| 106-119 | `filteredRuns` memo | Smazat |
| 134-150 | Cancel All button (runs tab header) | Smazat |
| 162-194 | Tabs UI (`configs \| runs`) | Smazat — žádné taby, rovnou configs |
| 196-212 | Search placeholder pro runs | Zjednodušit (jen configs search) |
| 374-436 | Celá Runs tab sekce | **Smazat** |
| 441-514 | `RunRow` komponenta | **Smazat** |
| 516-525 | `formatTimeAgo` helper | Smazat (pokud nikde jinde nepoužívaný) |

### WorkflowDetail (`src/pages/WorkflowDetail.tsx`)

| Řádek | Co | Akce |
|-------|-----|------|
| 70 | Destructure `runs, cancelRun, cancelAllRuns` | Smazat |
| 127 | `void navigate(\`/workflows/runs/${run.id}\`)` | Už přepsáno v fázi 3 |
| spodní část | Runs tabulka s cancel UI | **Smazat** |
| importy | `useWorkflowRuns` | Smazat |

### SentryFixerRunForm (`src/components/SentryFixerRunForm.tsx`)

| Řádek | Co | Akce |
|-------|-----|------|
| 133 | `void navigate(\`/workflows/runs/${run.id}\`)` | Už přepsáno v fázi 3 na `→ /sessions/${run.session_id}` |

### SentryTab (`src/components/SentryTab.tsx`) — ROZHODNUTÍ NUTNÉ

SentryTab má vlastní workflow run runtime UX (~1460 řádků). Závislosti:

| Řádky | Co |
|-------|-----|
| 17-18 | `import { useWorkflowRuns, useWorkflowRunStream }` |
| 23 | `import type { WorkflowRun }` |
| 625 | `fixingIds: Map<string, string>` — mapuje issueId → runId |
| 647 | `useWorkflowRuns("sentry-fixer")` — fetches run history |
| 679 | `setFixingIds(...set(issue.id, run.id))` — ukládá run ID po fixu |
| 971-972 | `InlineRunStatus` — `useWorkflowRunStream(runId)` pro live progress |
| 1389 | Další `useWorkflowRunStream(runId)` v batch fix UI |
| 1446-1459 | `RunHistorySection` — linky na `/workflows/runs/${run.id}` |

**Varianta A — Přepsat na session-first:**
- `fixingIds` mapuje issueId → sessionId
- `InlineRunStatus` používá session stream místo workflow run stream
- `RunHistorySection` linkuje na `/sessions/{id}` místo `/workflows/runs/{id}`
- `useWorkflowRuns("sentry-fixer")` nahradit za query na sessions (filtr dle workflow origin?)

**Varianta B — Zjednodušit:**
- Smazat `InlineRunStatus` (inline live progress) — po fixu redirect na session detail
- Smazat `RunHistorySection` — uživatel vidí historii v session list
- Ponechat jen fix spuštění + redirect na session
- Výrazně jednodušší, ale ztráta inline UX

**TODO: Rozhodnout A nebo B.**

### Session stránky

| Soubor | Řádky | Akce |
|--------|-------|------|
| `src/pages/SessionList.tsx:296-308` | Workflow run link badge | **Smazat** |
| `src/pages/SessionDetail.tsx:204-216` | Workflow run link | **Smazat** |

---

## Deploy ordering

**KRITICKÉ:** Fáze 3 (backend) + UI contract change musí jít SPOLEČNĚ. Potom 05a, potom 04+05.

```
Fáze 3 backend + UI contract (03)  ← musí jít společně
  ↓
Fáze 5a UI cleanup                 ← odstraní vše co závisí na workflow runs
  ↓
Fáze 4+5 backend runtime removal   ← teprve teď je safe smazat BE routes
```

## Ověření

```bash
# UI dev server
cd codeforge-ui && npm run dev

# TypeScript compile check — žádné broken importy
cd codeforge-ui && npx tsc --noEmit

# Otestovat:
# - WorkflowList: žádný runs tab, run → redirect na session detail
# - WorkflowDetail: žádná runs sekce, run → redirect na session detail
# - SentryFixerRunForm: run → redirect na session detail
# - SentryTab: dle varianty A/B
# - SessionList: žádné workflow run badge
# - SessionDetail: žádný workflow run link
# - /workflows/runs/xxx → redirect na /sessions (nebo 404)
# - žádné console errors, žádné broken importy
```
