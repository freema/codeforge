# CodeForge Roadmap — Overview

## Vision

CodeForge se vyvíjí z jednoúčelového AI CLI runneru na **univerzální orchestrační platformu**, kde:
- **Vše je tool** — Git, Sentry, Jira, Chrome, filesystem, databáze
- **Více agentů spolupracuje** — Claude Code píše kód, Codex dělá review, Sentry diagnostikuje bugy
- **Task má paměť** — kontexty přežívají mezi úlohami na stejném repozitáři
- **Uživatelé mají svůj prostor** — subscription, API klíče, usage tracking

## Fáze

| Fáze | Název | Závislosti | Popis |
|------|-------|------------|-------|
| **7** | [Tool System](./phase-7-tool-system.md) | — | Plugin architektura, registr toolů, runtime resoluce |
| **8** | [Built-in Tools](./phase-8-builtin-tools.md) | Phase 7 | Sentry, Jira, Chrome, Git-as-tool implementace |
| **9** | [Multi-CLI Support](./phase-9-multi-cli.md) | — | OpenCode, Codex, Aider runners |
| **10** | [Multi-Agent Orchestration](./phase-10-multi-agent.md) | Phase 7, 9 | Pipeline agentů, DAG workflow, sdílený kontext |
| **11** | [Task Sessions](./phase-11-task-sessions.md) | — | Cross-task paměť, project memory |
| **12** | [Code Review](./phase-12-code-review.md) | Phase 9, 10 | Automatický review změn jiným modelem |
| **13** | [Enhanced PR Messages](./phase-13-enhanced-pr.md) | — | Strukturované PR popisy, conventional commits |
| **14** | [Subscription & Multi-User Auth](./phase-14-subscription.md) | — | Per-user auth, usage tracking, plány |

## Priorita implementace

### Tier 1 — Základ (implementovat první)
1. **Phase 7: Tool System** — základ pro vše ostatní
2. **Phase 9: Multi-CLI Support** — rozšíření CLI registry
3. **Phase 13: Enhanced PR Messages** — rychlý win, malý rozsah

### Tier 2 — Klíčové funkce
4. **Phase 8: Built-in Tools** — Sentry, Jira, Chrome, Git
5. **Phase 11: Task Sessions** — project memory
6. **Phase 10: Multi-Agent Orchestration** — pipeline agentů

### Tier 3 — Pokročilé
7. **Phase 12: Code Review** — závisí na multi-agent + multi-CLI
8. **Phase 14: Subscription & Multi-User Auth** — monetizace

## Architektonické principy

1. **Registry pattern** — každý nový koncept (tool, CLI, agent) má svůj registr
2. **Interface-first** — definice rozhraní před implementací
3. **Redis-native** — vše komunikuje přes Redis (tools, agents, sessions)
4. **Backward compatible** — nové fáze neruší existující API
5. **Opt-in complexity** — jednoduchý task funguje beze změn, pokročilé funkce jsou volitelné

## Quality Gate — POVINNÉ pro každý PR

Viz [Testing Conventions](./testing-conventions.md) pro detaily.

```bash
task fmt           # gofmt + goimports — MUSÍ nic nezměnit
task lint          # golangci-lint — MUSÍ projít s 0 errors
task test          # unit testy — MUSÍ projít
task test:integration  # integration testy — MUSÍ projít
```

### Nové závislosti pro testování

```bash
go get github.com/alicebob/miniredis/v2  # In-memory Redis pro unit testy
```

### Klíčové linter rules (`.golangci.yml`)

- `errcheck` — všechny errors ošetřit
- `gosec` — security checks
- `gocyclo` — max complexity 15
- `goconst` — opakující se stringy → konstanty
- `noctx` — HTTP requesty s contextem
- `bodyclose` — HTTP body uzavřen

### Testovací pattern

- **Unit testy**: table-driven, pure stdlib `testing`, miniredis pro Redis
- **Interfaces**: každá závislost za interface (mock bez frameworku)
- **Integration testy**: `//go:build integration`, reálný Redis v Dockeru
- **Coverage cíl**: 80%+ pro nový kód, 95%+ pro security-critical

## Soubory

| Soubor | Popis |
|--------|-------|
| `tasks/roadmap-overview.md` | Tento soubor — přehled roadmapu |
| `tasks/testing-conventions.md` | Testovací konvence, patterns, checklist |
| `tasks/phase-{7-14}-*.md` | Detailní task breakdowns per fáze |
| `.golangci.yml` | Linter konfigurace |
