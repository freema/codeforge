# CodeForge — Multi-Agent & Continuous Autonomy Roadmap

## Vize

Rozšířit CodeForge ze single-session orchestrátoru na platformu schopnou řídit **týmy specializovaných AI agentů**, kteří spolupracují na komplexních úkolech v rámci sprintů, vzájemně si reviewují kód a běží kontinuálně bez lidského zásahu.

## Hlavní iniciativy

| # | Iniciativa | Soubor | Priorita | Závislosti |
|---|-----------|--------|----------|------------|
| 1 | Agent Role System | `01-agent-roles.md` | P0 | — |
| 2 | Multi-Agent Workflow Pipeline | `02-multi-agent-pipeline.md` | P0 | #1 |
| 3 | Cross-Review Pattern | `03-cross-review.md` | P1 | #2 |
| 4 | Scheduled / Cron Workflows | `04-scheduled-workflows.md` | P1 | #2 |
| 5 | Sprint & Backlog Management | `05-sprint-management.md` | P2 | #1, #2, #4 |

## Principy

- **Stavět na existující infrastruktuře** — session, workflow, worker pool, streaming, Git integrace. Nepřepisovat, rozšiřovat.
- **Opt-in komplexita** — single-session use case musí zůstat jednoduchý. Multi-agent je nadstavba.
- **Human-in-the-loop zůstává výchozí** — plná autonomie je volitelný režim, ne default.
- **Transparentnost** — veškerá komunikace mezi agenty musí být čitelná a auditovatelná.
