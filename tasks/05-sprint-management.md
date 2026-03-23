# 05 — Sprint & Backlog Management

## Problém

CodeForge dnes pracuje na úrovni jednotlivých sessions a workflows. Chybí vyšší abstrakce pro řízení **kontinuální autonomní práce** — plánování sprintů, správa backlogu, sledování průběhu a automatické navazování na předchozí práci.

## Cíl

Zavést koncept **projektu se sprinty a backlogem**, který umožní CodeForge autonomně pracovat na rozsáhlejších úkolech po dobu dnů až týdnů — plánovat práci, implementovat stories, trackovat progres a navazovat na předchozí výsledky.

## Návrh

### Datový model

```go
type Project struct {
    ID          string
    Name        string
    RepoURL     string
    Description string    // high-level brief co se má postavit
    Config      ProjectConfig
    CreatedAt   time.Time
}

type ProjectConfig struct {
    ProviderKey     string
    CyclesPerSprint int    // kolik workflow runů = 1 sprint (default: 10)
    DefaultPipeline string // workflow template pro každý cycle (default: "agile-team")
    ScheduleID      string // vazba na schedule pro automatické spouštění
}

type Sprint struct {
    ID        string
    ProjectID string
    Number    int
    Status    string // "active", "completed", "retrospective"
    StartedAt time.Time
    EndedAt   *time.Time
}

type Story struct {
    ID          string
    ProjectID   string
    SprintID    string    // přiřazení do sprintu (může být nil = backlog)
    Title       string
    Description string
    Status      string    // "todo", "in_progress", "review", "testing", "done", "blocked"
    Priority    string    // "critical", "high", "medium", "low"
    AssignedTo  string    // role name (implementer, devops, etc.)
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### Board — stav projektu jako markdown

Inspirace konceptem transparentního, čitelného stavu:

```
.codeforge/board/
  backlog.md       — všechny stories s prioritami
  sprint.md        — aktuální sprint, přiřazené stories, status
  architecture.md  — architektonická rozhodnutí (generuje planner)
  decisions.md     — log důležitých rozhodnutí
```

Board soubory slouží **dvěma účelům**:
1. **Kontext pro agenty** — agent při spuštění čte aktuální stav boardu a ví, na čem má pracovat
2. **Transparentnost pro lidi** — člověk může kdykoliv otevřít markdown a vidět stav projektu

Board soubory žijí v repozitáři (committují se) → verzovaná historie stavu projektu.

### Životní cyklus

```
1. Uživatel vytvoří Project s briefem
2. Planner agent analyzuje brief → generuje stories do backlogu
3. PM agent vybere stories pro sprint → sprint.md
4. Implementer agent pracuje na story → status: in_progress → done
5. Reviewer agent reviewuje → approve/request_changes
6. Po X cyklech → sprint retrospektiva → nový sprint
7. Opakuje se od bodu 3
```

### Integrace se stávajícími koncepty

| Nový koncept | Mapuje se na | Jak |
|-------------|-------------|-----|
| Project | — | Nová entita, drží repo + brief + config |
| Sprint | Scheduled workflow | Schedule spouští pipeline každých N minut |
| Story | Workflow step context | Story se předá agentovi jako kontext v promptu |
| Board | Workspace soubory | `.codeforge/board/` soubory v repo |
| Cycle | Workflow run | Jeden run agile-team pipeline = jeden cycle |

### API

```
POST   /api/v1/projects                      — vytvořit projekt s briefem
GET    /api/v1/projects/:id                   — stav projektu
GET    /api/v1/projects/:id/stories           — seznam stories
GET    /api/v1/projects/:id/sprints           — historie sprintů
GET    /api/v1/projects/:id/sprints/current   — aktuální sprint
POST   /api/v1/projects/:id/pause             — pozastavit autonomní práci
POST   /api/v1/projects/:id/resume            — obnovit
POST   /api/v1/projects/:id/stories           — ručně přidat story
```

### Provider sync (volitelné)

- Stories ↔ GitHub Issues / GitLab Issues (obousměrná synchronizace)
- Sprint stav → GitHub Project / GitLab Board
- Standup → Slack webhook / issue comment

### Prompt šablony

Nové prompt templates pro projekt-aware agenty:

- `sprint_planning.md` — PM agent: přečti backlog, vyber stories pro sprint, přiřaď role
- `story_implementation.md` — Implementer: přečti sprint.md, najdi svou story, implementuj
- `sprint_retrospective.md` — PO + PM: zhodnoť sprint, co se povedlo/nepovedlo, uprioriti backlog

## Dotčené soubory

- `internal/project/` — nový package (model, store, service)
- `internal/database/` — migrace pro projects, sprints, stories tabulky
- `internal/server/handlers/projects.go` — nový handler
- `internal/prompt/templates/` — nové šablony
- `internal/workflow/builtins.go` — sprint-aware pipeline

## Fáze implementace

1. **Fáze 1 — Data model**: Project + Story + Sprint entity, CRUD API, SQLite persistence
2. **Fáze 2 — Board**: Generování a čtení `.codeforge/board/` souborů, kontext pro agenty
3. **Fáze 3 — Sprint lifecycle**: Automatický cycle counter, retrospektiva, nový sprint
4. **Fáze 4 — Provider sync**: Dvousměrná sync stories ↔ GitHub/GitLab issues

## Otevřené otázky

- Jak řešit merge konflikty na board souborech mezi cykly?
- Má agent smět přepisovat stories jiného agenta?
- Chceme board soubory committovat do main nebo feature branche?
- Jak detekovat "stuck" sprint (stories se nehýbou)?
