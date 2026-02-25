# Akční plán: Phase 7 + 8 + 9 — dotažení do produkce

> Vytvořeno: 2026-02-25
> Stav: aktivní plán

## Skutečný stav (audit 2026-02-25)

Situace je LEPŠÍ než se zdálo z dokumentace:

| Komponenta | Stav | Soubory |
|-----------|------|---------|
| `internal/tool/git/` | **HOTOVO** | 11 souborů, clone/branch/diff/PR/GitHub/GitLab |
| `internal/tool/runner/` | **HOTOVO** | Claude runner + Codex runner (reálná implementace, ne stub) |
| `internal/tool/mcp/` | **HOTOVO** | SQLite registry, installer (.mcp.json generování) |
| Tool System (Phase 7) | **NEEXISTUJE** | Žádný `internal/tools/` package — chybí registry, resolver, bridge, catalog |
| Stream Normalization | **ČÁSTEČNĚ** | Streamer existuje (workflow + SSE), ale per-CLI normalizace chybí |
| OpenCode runner | **NEEXISTUJE** | |
| Aider runner | **NEEXISTUJE** | |

### Co NECHYBÍ (=funguje a je v produkci):
- Claude Code runner s stream-json parsováním
- Codex runner s JSONL parsováním
- Git operations (clone, branch, commit, diff, PR/MR)
- MCP server registry + installer
- CLI registry pattern (registrace runnerů, default fallback)

### Co CHYBÍ:

#### Phase 7 — Tool System
- `internal/tools/model.go` — ToolDefinition, ToolInstance, ConfigField, ToolType
- `internal/tools/registry.go` — Redis CRUD pro tool definice
- `internal/tools/resolver.go` — merge logika (global → project → task)
- `internal/tools/bridge.go` — ToolInstance → mcp.Server konverze
- `internal/tools/catalog.go` — built-in definice (Sentry, Jira, Git, Browser)
- `internal/tools/validator.go` — config validace
- `internal/server/handlers/tools.go` — HTTP endpoints
- Integrace do executoru (tool resolver před MCP setup)

#### Phase 8 — Built-in Tools
- Definice toolů v katalogu (pouze Go structs + konstanty)
- Bridge mapování per tool (config → env vars)
- Docker compose: Playwright sidecar (volitelné)
- Dokumentace per tool

#### Phase 9 — Multi-CLI
- OpenCode runner implementace
- Aider runner implementace
- Stream normalizer interface + per-CLI implementace
- Docker multi-CLI Dockerfile
- CLI health check endpoint
- CLI+model kompatibilita validace

---

## Plán implementace (pořadí)

### Sprint 1: Tool System foundation (Phase 7.1–7.4)

**Priorita: HIGHEST — blokuje Phase 8**

| # | Task | Soubor(y) | Odhad |
|---|------|-----------|-------|
| 1.1 | Model + typy | `internal/tools/model.go` | 1h |
| 1.2 | Catalog (built-in definice) | `internal/tools/catalog.go` | 1h |
| 1.3 | Registry (Redis CRUD) | `internal/tools/registry.go` + testy | 2h |
| 1.4 | Resolver (merge global→project→task) | `internal/tools/resolver.go` + testy | 2h |
| 1.5 | Bridge (ToolInstance → mcp.Server) | `internal/tools/bridge.go` + testy | 1h |
| 1.6 | Validator | `internal/tools/validator.go` + testy | 1h |

**Výstup:** Package `internal/tools/` kompletní, unit testy, `task test` projde.

### Sprint 2: Tool System HTTP + integrace (Phase 7.5–7.9)

| # | Task | Soubor(y) | Odhad |
|---|------|-----------|-------|
| 2.1 | HTTP handlers (CRUD) | `internal/server/handlers/tools.go` | 2h |
| 2.2 | Route registration | `internal/server/server.go` | 30m |
| 2.3 | Task request extension | `internal/task/model.go`, `service.go` | 1h |
| 2.4 | Executor integrace | `internal/worker/executor.go` | 1h |
| 2.5 | Tool usage tracking | stream events | 30m |
| 2.6 | Integration testy | `tests/integration/` | 1h |

**Výstup:** `POST /api/v1/tasks` s `tools` polem funguje, .mcp.json se generuje s resolved tooly.

### Sprint 3: Built-in Tools (Phase 8)

**Závisí na: Sprint 1+2**

| # | Task | Odhad |
|---|------|-------|
| 3.1 | Sentry tool definice + bridge | 1h |
| 3.2 | Jira tool definice + bridge | 1h |
| 3.3 | Git MCP tool definice + bridge | 1h |
| 3.4 | GitHub API tool definice + bridge | 1h |
| 3.5 | Browser (Playwright) definice + bridge | 1h |
| 3.6 | Custom tool registration flow | 1h |
| 3.7 | Dokumentace per tool | 1h |
| 3.8 | Bridge testy per tool | 2h |

**Výstup:** 5 built-in toolů v katalogu, custom tool registrace, docs.

### Sprint 4: Stream Normalization (Phase 9 — core)

**Nezávisí na Phase 7/8, může paralelně**

| # | Task | Soubor(y) | Odhad |
|---|------|-----------|-------|
| 4.1 | NormalizedEvent + StreamNormalizer interface | `internal/tool/runner/normalizer.go` | 30m |
| 4.2 | ClaudeStreamNormalizer | `internal/tool/runner/normalizer_claude.go` | 1h |
| 4.3 | CodexStreamNormalizer | `internal/tool/runner/normalizer_codex.go` | 1h |
| 4.4 | Integrace do Streamer.EmitCLIOutput() | `internal/worker/stream.go` | 1h |
| 4.5 | Unit testy normalizérů | `*_test.go` | 1h |

**Výstup:** Unified stream format, Claude + Codex normalizace, testy.

### Sprint 5: Nové CLI Runners (Phase 9 — runners)

| # | Task | Soubor(y) | Odhad |
|---|------|-----------|-------|
| 5.1 | OpenCode runner | `internal/tool/runner/opencode.go` | 2h |
| 5.2 | OpenCode normalizer | `internal/tool/runner/normalizer_opencode.go` | 1h |
| 5.3 | Aider runner | `internal/tool/runner/aider.go` | 2h |
| 5.4 | Aider normalizer (plain text) | `internal/tool/runner/normalizer_aider.go` | 1h |
| 5.5 | Docker multi-CLI Dockerfile | `deployments/Dockerfile` | 1h |
| 5.6 | CLI health check endpoint | `handlers/` | 30m |
| 5.7 | CLI+model validace | `internal/tool/runner/` | 1h |
| 5.8 | Unit testy + mock CLI rozšíření | | 2h |

**Výstup:** 4 CLI runners (Claude, Codex, OpenCode, Aider), Docker image se všemi.

### Sprint 6: E2E + dokumentace

| # | Task | Odhad |
|---|------|-------|
| 6.1 | E2E test: task s tooly | 1h |
| 6.2 | E2E test: task s different CLI | 1h |
| 6.3 | API docs update (OpenAPI) | 1h |
| 6.4 | docs/tools.md | 1h |
| 6.5 | docs/multi-cli.md | 1h |

---

## Závislosti mezi sprinty

```
Sprint 1 (tool model/registry/resolver/bridge)
    │
    ├──► Sprint 2 (HTTP + executor integrace)
    │       │
    │       └──► Sprint 3 (built-in tools)
    │               │
    └───────────────┴──► Sprint 6 (E2E + docs)

Sprint 4 (stream normalization) ← nezávislý, paralelní
    │
    └──► Sprint 5 (nové runners)
            │
            └──► Sprint 6 (E2E + docs)
```

**Kritická cesta:** Sprint 1 → 2 → 3 → 6
**Paralelní:** Sprint 4 → 5 (stream + runners)

## Celkový odhad

| Sprint | Odhad | Kumulativně |
|--------|-------|-------------|
| 1 (Tool foundation) | ~8h | 8h |
| 2 (HTTP + integrace) | ~6h | 14h |
| 3 (Built-in tools) | ~9h | 23h |
| 4 (Stream normalization) | ~4.5h | 27.5h |
| 5 (Nové runners) | ~10.5h | 38h |
| 6 (E2E + docs) | ~5h | 43h |

**Celkem: ~43h efektivní práce**

## Rozhodnutí k potvrzení

1. **Package name:** `internal/tools/` (s 's') vs `internal/tool/tools/` (pod existující namespace)?
   - Doporučení: `internal/tools/` — top-level, nezávisí na tool/ namespace
2. **Aider priorita:** implementovat teď nebo odložit? (plain text parsing = highest effort, lowest value)
   - Doporučení: odložit Aider na later, priorita Claude + Codex + OpenCode
3. **Docker multi-CLI:** jeden fat image nebo per-CLI image?
   - Doporučení: jeden image s ARG flags pro volitelnou instalaci
4. **Tool versioning:** pinovat verze MCP serverů v katalogu?
   - Doporučení: ano, `MCPVersion` pole v ToolDefinition

## Další kroky

Začít Sprint 1 — `internal/tools/model.go` + `catalog.go`.
