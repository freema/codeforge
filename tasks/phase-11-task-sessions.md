# Phase 11: Task Sessions

## Cíl

Implementovat **cross-task paměť** — kontext z předchozích tasků na stejném repozitáři přežívá a je dostupný novým taskům. Agent "ví" co se na projektu dělalo dříve.

## Závislosti

- Žádné přímé závislosti (ale synerguje s Phase 10 multi-agent)

## Motivace

Dnes každý task začíná "od nuly" — agent nemá kontext o:
- Předchozích úpravách na repo
- Architektonických rozhodnutích
- Konvencích projektu
- Minulých chybách a jejich řešeních

Session system řeší toto tím, že udržuje **project memory** — strukturované znalosti nasbírané z předchozích tasků.

## Architektura

```
┌──────────────────────────────────────────────────────────────┐
│                    SESSION STORAGE                             │
│                                                               │
│  session:{repo_hash}:memory     — project knowledge base     │
│  session:{repo_hash}:history    — task history log            │
│  session:{repo_hash}:decisions  — architectural decisions     │
│  session:{repo_hash}:meta       — session metadata            │
│                                                               │
└──────────────────────┬───────────────────────────────────────┘
                       │
         ┌─────────────┴─────────────┐
         │                           │
         ▼                           ▼
┌─────────────────┐         ┌─────────────────┐
│   TASK #1        │         │   TASK #2        │
│   "Add auth"     │         │   "Fix login"    │
│                  │         │                  │
│   → Writes to    │         │   → Reads from   │
│     session      │         │     session      │
│     memory       │         │     memory       │
└─────────────────┘         │   → Knows about  │
                            │     auth system   │
                            └─────────────────┘
```

## Datový model

### Session

```go
// internal/session/model.go

type Session struct {
    ID          string    `json:"id"`            // hash repo URL
    RepoURL     string    `json:"repo_url"`
    ProjectName string    `json:"project_name"`  // extrahováno z URL
    TaskCount   int       `json:"task_count"`
    Memory      *Memory   `json:"memory"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Memory struct {
    // Strukturované znalosti o projektu
    TechStack    []string          `json:"tech_stack"`     // ["Go", "Chi", "Redis"]
    Conventions  []string          `json:"conventions"`    // ["table-driven tests", "slog logging"]
    Architecture string            `json:"architecture"`   // vysokoúrovňový popis
    Decisions    []Decision        `json:"decisions"`      // architektonická rozhodnutí
    Notes        []string          `json:"notes"`          // volné poznámky
    KnownIssues  []string          `json:"known_issues"`   // známé problémy
}

type Decision struct {
    Date        string `json:"date"`
    Description string `json:"description"`
    Reasoning   string `json:"reasoning"`
    TaskID      string `json:"task_id"`  // ze kterého tasku pochází
}

type TaskSummary struct {
    TaskID      string    `json:"task_id"`
    Prompt      string    `json:"prompt"`      // truncated
    Status      string    `json:"status"`
    Changes     string    `json:"changes"`     // "3 files modified"
    Duration    float64   `json:"duration_s"`
    CompletedAt time.Time `json:"completed_at"`
}
```

### Session Identification

```go
// Repo URL → session ID
func SessionID(repoURL string) string {
    // Normalizuj URL (odstraň .git, trailing slash)
    // SHA256 hash → prvních 16 znaků
    normalized := normalizeRepoURL(repoURL)
    hash := sha256.Sum256([]byte(normalized))
    return hex.EncodeToString(hash[:8])
}
```

## Redis klíče

| Klíč | Typ | Popis | TTL |
|------|-----|-------|-----|
| `session:{id}:meta` | Hash | Session metadata (repo_url, project_name, task_count) | 90d |
| `session:{id}:memory` | String | JSON-encoded Memory struct | 90d |
| `session:{id}:history` | List | JSON-encoded TaskSummary items | 90d |
| `session:{id}:decisions` | List | JSON-encoded Decision items | 90d |

## Memory Injection do Promptu

Když task startuje, executor:
1. Vypočítá session ID z repo URL
2. Načte session memory (pokud existuje)
3. Přidá do promptu jako kontext

```
## Project Memory (from previous tasks)

### Tech Stack
Go, Chi router, Redis, slog logging

### Conventions
- Table-driven tests
- Structured logging via slog
- Error handling: typed errors from internal/errors

### Recent Tasks on This Project
1. [2 days ago] "Add user authentication" → completed (5 files modified)
2. [5 days ago] "Setup CI/CD pipeline" → completed (3 files created)

### Architectural Decisions
- 2025-02-10: Chose JWT over sessions for auth (stateless, Redis not needed for auth)
- 2025-02-08: Using Chi router instead of standard mux (middleware support)

### Known Issues
- Login endpoint doesn't rate limit (TODO)

---

## Current Task
{actual task prompt}
```

## Memory Extraction (po dokončení tasku)

Po úspěšném dokončení tasku, extrahujeme znalosti:

### Strategie 1: Heuristická extrakce (jednodušší)
- Parsuj result text, hledej patterns:
  - "I chose X because Y" → Decision
  - "The project uses X" → TechStack
  - "Note: X" → Notes
- Přidej task summary do historie

### Strategie 2: AI summarizace (přesnější, dražší)
- Pošli result text dalšímu AI volání s promptem:
  "Extract project knowledge from this task result..."
- Strukturovaný output → Memory update

### Strategie 3: Hybrid (doporučeno)
- Task summary + changes → vždy (heuristika)
- Memory update → jen pokud task přinesl nové poznatky (detekce)
- Periodická AI summarizace → background job na konsolidaci

## Nové soubory

```
internal/session/
  model.go       — Session, Memory, Decision, TaskSummary
  service.go     — CRUD operace v Redis
  injector.go    — Injection memory do promptu
  extractor.go   — Extrakce znalostí z task výsledků
  identifier.go  — Repo URL → Session ID
```

## API Endpointy

```
GET    /api/v1/sessions                         — Seznam sessions
GET    /api/v1/sessions/{id}                    — Detail session + memory
GET    /api/v1/sessions/{id}/history            — Task historie
PUT    /api/v1/sessions/{id}/memory             — Manuální update memory
DELETE /api/v1/sessions/{id}                    — Smazat session
POST   /api/v1/sessions/{id}/consolidate        — Trigger AI konsolidace memory
```

## Tasky

### 11.1 — Datový model a identifikace
- [ ] Vytvořit `internal/session/model.go`
- [ ] Vytvořit `internal/session/identifier.go` — repo URL normalizace + hashing
- [ ] Unit testy: různé URL formáty → stejný session ID

### 11.2 — Session Service (Redis)
- [ ] Vytvořit `internal/session/service.go`
- [ ] CRUD: Create/Get/Update/Delete session
- [ ] Memory: Get/Update memory
- [ ] History: Append/List task summaries
- [ ] Decisions: Append/List
- [ ] TTL management (90d, refresh on access)
- [ ] Unit testy

### 11.3 — Memory Injector
- [ ] Vytvořit `internal/session/injector.go`
- [ ] Formátování memory do prompt contextu
- [ ] Truncation (max chars pro memory section)
- [ ] Integrace s `Executor.buildPrompt()` — přidej memory před task prompt
- [ ] Opt-out: task config `"session": false` pro vypnutí
- [ ] Unit testy

### 11.4 — Knowledge Extractor
- [ ] Vytvořit `internal/session/extractor.go`
- [ ] Heuristická extrakce: task summary, changes, basic patterns
- [ ] Volat po úspěšném dokončení tasku (v Executor.Execute)
- [ ] Přidat do session history
- [ ] Unit testy

### 11.5 — HTTP Handlers
- [ ] Vytvořit `internal/server/handlers/sessions.go`
- [ ] Endpointy: CRUD sessions, memory, history
- [ ] Registrace rout v `server.go`
- [ ] Validace

### 11.6 — Executor integrace
- [ ] Upravit `Executor.Execute()`:
  - Před run: načti session memory → inject do promptu
  - Po run: extrahuj znalosti → ulož do session
- [ ] Backward compatible: bez session data funguje beze změn
- [ ] Stream event: `session_loaded`, `session_updated`

### 11.7 — Integration testy
- [ ] Test: task #1 vytvoří session → task #2 má kontext
- [ ] Test: session TTL refresh
- [ ] Test: opt-out session
- [ ] Test: memory truncation

## Konfigurace

```yaml
sessions:
  enabled: true
  ttl_days: 90
  max_memory_chars: 10000
  max_history_items: 50
  extraction_strategy: heuristic  # "heuristic", "ai", "hybrid"
```

## Testovací strategie

### Identifier testy (`internal/session/identifier_test.go`)

Klíčové: různé URL formáty pro stejný repo MUSÍ produkovat stejný session ID.

- [ ] `TestSessionID_SameRepo_DifferentFormats` — table-driven:

```go
func TestSessionID_SameRepo_DifferentFormats(t *testing.T) {
    tests := []struct {
        name string
        urls []string
    }{
        {
            name: "github https variants",
            urls: []string{
                "https://github.com/user/repo",
                "https://github.com/user/repo.git",
                "https://github.com/user/repo/",
                "https://github.com/user/repo.git/",
            },
        },
        {
            name: "github ssh vs https",
            urls: []string{
                "https://github.com/user/repo",
                "git@github.com:user/repo.git",
            },
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ids := make(map[string]bool)
            for _, url := range tt.urls {
                ids[SessionID(url)] = true
            }
            if len(ids) != 1 {
                t.Errorf("got %d unique IDs for same repo, want 1", len(ids))
            }
        })
    }
}
```

- [ ] `TestSessionID_DifferentRepos` — různé repos → různé ID
- [ ] `TestSessionID_CaseSensitive` — user/Repo vs user/repo
- [ ] `TestNormalizeRepoURL` — strip .git, trailing slash, lowercase host

### Session Service testy (`internal/session/service_test.go` — miniredis)

- [ ] `TestSessionService_Create` — vytvoř session, ověř v Redis
- [ ] `TestSessionService_Get` — get existující session
- [ ] `TestSessionService_Get_NotFound` — error pro neexistující
- [ ] `TestSessionService_UpdateMemory` — update memory field
- [ ] `TestSessionService_AppendHistory` — append task summary
- [ ] `TestSessionService_GetHistory` — list task summaries (LRANGE)
- [ ] `TestSessionService_AppendDecision` — append decision
- [ ] `TestSessionService_Delete` — smazání + cleanup všech klíčů
- [ ] `TestSessionService_TTL` — TTL se nastaví a refreshne při přístupu
- [ ] `TestSessionService_TTL_Refresh` — Get() refreshne TTL

### Memory Injector testy (`internal/session/injector_test.go`)

- [ ] `TestInjector_NoSession` — žádná session → prompt beze změn
- [ ] `TestInjector_WithMemory` — session s memory → prompt obsahuje "## Project Memory"
- [ ] `TestInjector_WithHistory` — session s historií → prompt obsahuje "Recent Tasks"
- [ ] `TestInjector_Truncation` — paměť delší než max_memory_chars se ořeže
- [ ] `TestInjector_EmptyMemory` — session existuje ale memory je prázdná → nic nepřidávej

### Knowledge Extractor testy (`internal/session/extractor_test.go`)

- [ ] `TestExtractor_TaskSummary` — generuje správný TaskSummary z tasku
- [ ] `TestExtractor_TaskSummary_TruncatedPrompt` — dlouhý prompt se ořeže
- [ ] `TestExtractor_HeuristicPatterns` — "I chose X because Y" → Decision

### Integration testy (`//go:build integration`)

- [ ] `TestIntegration_Session_CrossTask` — task #1 → session created → task #2 má kontext
- [ ] `TestIntegration_Session_OptOut` — task s `session: false` → žádná session interakce

## Linter checklist

- [ ] SHA256 import z `crypto/sha256` (ne insecure hash)
- [ ] TTL management: žádné orphaned klíče bez TTL
- [ ] JSON marshal errors ošetřeny
- [ ] `task fmt` + `task lint` MUSÍ projít

## Otevřené otázky

1. **Memory conflicts** — co když dva paralelní tasky updatují memory současně?
2. **Memory quality** — jak zajistit, že memory neobsahuje špatné informace?
3. **Memory cleanup** — jak detekovat a odstranit zastaralé informace?
4. **Cross-repo sessions** — sdílení memory mezi forky/monorepo?
5. **Session ownership** — per-user sessions vs globální per-repo?
