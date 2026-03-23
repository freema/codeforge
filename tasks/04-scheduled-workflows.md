# 04 — Scheduled / Cron Workflows

## Problém

CodeForge workflows jsou dnes spouštěné výhradně přes API volání nebo webhooky. Chybí možnost **periodického automatického spouštění** — např. "každou noc projdi Sentry errory a oprav je" nebo "každý týden spusť security audit".

## Cíl

Přidat podporu pro **plánované spouštění workflows** — uživatel definuje schedule (cron expression), CodeForge automaticky spouští workflow v daných intervalech.

## Návrh

### Datový model

```go
type WorkflowSchedule struct {
    ID           string            // unikátní ID
    WorkflowName string            // název workflow k spuštění
    CronExpr     string            // cron expression (5-field: min hour dom mon dow)
    Params       map[string]string // fixní parametry pro každé spuštění
    Enabled      bool              // aktivní/pozastavený
    MaxConcurrent int              // max souběžných runů z tohoto schedule (default: 1)
    Timezone     string            // IANA timezone (default: UTC)
    LastRunAt    *time.Time        // poslední spuštění
    NextRunAt    *time.Time        // příští plánované spuštění (pre-computed)
    CreatedAt    time.Time
}
```

### Scheduler komponenta

Nová komponenta `internal/scheduler/`:

```
scheduler/
  scheduler.go    — hlavní loop (tick každou minutu, porovnává s NextRunAt)
  store.go        — SQLite persistence pro schedules
  model.go        — WorkflowSchedule struct
```

**Chování:**
1. Při startu serveru scheduler načte všechny enabled schedules
2. Každou minutu porovná aktuální čas s `NextRunAt` všech schedules
3. Pokud `now >= NextRunAt`:
   a. Zkontroluje `MaxConcurrent` (kolik runů z tohoto schedule aktuálně běží)
   b. Pokud pod limitem → spustí workflow (enqueue do workflow queue)
   c. Aktualizuje `LastRunAt` a vypočítá nový `NextRunAt`

**Guard rails:**
- `MaxConcurrent` default 1 — nespouštět nový run pokud předchozí ještě běží
- Minimální interval: 5 minut (zabránit nechtěnému zahlcení)
- Dead letter: pokud workflow 3× po sobě failne, schedule se automaticky pozastaví + alert

### API

```
GET    /api/v1/schedules                — seznam schedules
POST   /api/v1/schedules                — vytvořit schedule
GET    /api/v1/schedules/:id            — detail + historie runů
PUT    /api/v1/schedules/:id            — upravit
DELETE /api/v1/schedules/:id            — smazat
POST   /api/v1/schedules/:id/pause      — pozastavit
POST   /api/v1/schedules/:id/resume     — obnovit
POST   /api/v1/schedules/:id/trigger    — okamžitě spustit (mimo schedule)
```

### Příklady použití

```json
{
  "workflow_name": "sentry-fixer",
  "cron_expr": "0 2 * * *",
  "timezone": "Europe/Prague",
  "params": {
    "sentry_org": "my-org",
    "sentry_project": "backend",
    "repo_url": "https://github.com/my-org/backend",
    "key_name": "my-key"
  }
}
```

```json
{
  "workflow_name": "agile-team",
  "cron_expr": "*/40 * * * *",
  "params": {
    "repo_url": "https://github.com/my-org/project",
    "task_description": "Continue working on the current sprint backlog"
  },
  "max_concurrent": 1
}
```

### Persistence

- SQLite tabulka `workflow_schedules`
- Historie spuštění = existující `workflow_runs` tabulka (přidat `schedule_id` FK)

### Observabilita

- `codeforge_schedules_total{status}` — počet schedules (enabled/paused)
- `codeforge_schedule_runs_total{workflow,status}` — spuštění per workflow
- `codeforge_schedule_auto_paused_total` — automaticky pozastavené (opakovaný failure)
- Log entry při každém schedule triggeru

## Dotčené soubory

- `internal/scheduler/` — nový package
- `internal/database/` — migrace pro `workflow_schedules` tabulku
- `internal/workflow/model.go` — přidat `ScheduleID` do `WorkflowRun`
- `internal/server/handlers/` — nový handler pro schedules API
- `internal/server/server.go` — registrace schedule routes
- `cmd/codeforge/main.go` — start scheduler jako goroutine

## Otevřené otázky

- Chceme podporovat i jednorázové delayed spuštění (run-at timestamp)?
- Jak řešit timezone DST přechody?
- Mají schedule respektovat global rate limit na workflow runs?
