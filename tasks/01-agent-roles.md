# 01 — Agent Role System

## Problém

CodeForge dnes pracuje s generickými sessions — každá session dostane prompt a běží bez identity. Chybí koncept **specializované role** (plánovač, implementátor, reviewer, tester, security auditor), která by definovala chování, system prompt, omezení a zodpovědnost AI.

## Cíl

Zavést **Agent Role** jako first-class koncept — pojmenovanou, konfigurovatelnou roli s vlastním system promptem, povolenými akcemi a výchozím chováním. Role se přiřazuje session v rámci multi-agent workflow.

## Návrh

### Datový model

```go
// AgentRole definuje specializovanou roli pro AI session
type AgentRole struct {
    Name        string   // unikátní identifikátor: "planner", "implementer", "reviewer", "tester", "security-auditor"
    Description string   // lidský popis role
    SystemPrompt string  // system prompt šablona (Go template s přístupem ke kontextu workflow)
    Capabilities []string // co smí dělat: "read_only", "write_code", "write_tests", "create_pr", "review"
    DefaultCLI   string  // výchozí CLI pro tuto roli (volitelné)
    DefaultModel string  // výchozí model (volitelné)
    Builtin      bool    // built-in vs. user-defined
}
```

### Built-in role (výchozí sada)

| Role | Popis | Capabilities | Read-only? |
|------|-------|-------------|------------|
| `planner` | Analyzuje repo, generuje implementační plán a user stories | `read_only` | ano |
| `implementer` | Implementuje kód podle zadání/plánu | `write_code`, `write_tests` | ne |
| `reviewer` | Reviewuje kód jiného agenta, hledá bugy a quality issues | `read_only`, `review` | ano |
| `tester` | Píše a spouští testy, ověřuje acceptance criteria | `write_tests` | ne |
| `security-auditor` | OWASP audit, hledá zranitelnosti | `read_only`, `review` | ano |
| `devops` | Dockerfiles, CI/CD, infrastruktura | `write_code` | ne |

### Uložení

- Built-in role v kódu (jako `BuiltinWorkflows`)
- Custom role v SQLite (CRUD přes API)
- System prompt jako Go template s přístupem k workflow kontextu (params, předchozí kroky, repo info)

### API

```
GET    /api/v1/roles              — seznam rolí
GET    /api/v1/roles/:name        — detail role
POST   /api/v1/roles              — vytvořit custom roli
PUT    /api/v1/roles/:name        — upravit custom roli
DELETE /api/v1/roles/:name        — smazat custom roli
```

### Integrace se session

- Nové pole `role` v `SessionStepConfig` a `Session` modelu
- Pokud session má roli, její system prompt se prepend před user prompt
- Capabilities role omezují, co CLI smí dělat (např. reviewer nemůže editovat soubory)

## Dotčené soubory

- `internal/role/` — nový package (model, store, builtins, service)
- `internal/session/model.go` — přidat `Role` pole
- `internal/workflow/model.go` — přidat `Role` do `SessionStepConfig`
- `internal/worker/executor.go` — aplikovat system prompt z role
- `internal/server/handlers/` — nový handler pro role API
- `internal/database/` — migrace pro roles tabulku

## Otevřené otázky

- Mají capabilities reálně blokovat CLI operace, nebo jsou jen "soft" (doporučení v promptu)?
- Chceme hierarchii rolí (reviewer > senior-reviewer)?
