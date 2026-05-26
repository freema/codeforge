# Prompt: CodeForge CLI Expansion + Subscription Model

> Tento prompt je určen pro Claude Code. Spusť ho v root adresáři CodeForge projektu.

---

## Kontext

CodeForge je Go HTTP server orchestrující AI-powered code work nad git repozitáři. Aktuálně podporuje 2 CLI runnery (Claude Code, Codex) a autentizaci přes API klíče. Potřebujeme:

1. **Přidat subscription model** (vedle stávajících API klíčů — dual-auth)
2. **Přidat Cursor CLI runner** (`cursor-agent`)
3. **Přidat Claude Agent runner** (`claude --bare`)
4. **Refaktorovat executor** (hardcoded switch → registry metadata)

Stávající API key flow se NESMÍ rozbít. Subscription je additivní vrstva.

Přečti si `CLAUDE.md` v root projektu pro konvence, architekturu a příkazy.

---

## Fáze 1: Refaktoring Executor — Registry Metadata

**Proč nejdřív:** Odstraní hardcoded switch statementy v executoru, usnadní přidání nových runnerů.

### Úkoly:

1. **Rozšiř `internal/tool/runner/runner.go`** — přidej `RunnerMeta` struct:
   ```go
   type RunnerMeta struct {
       NormalizerFactory func() StreamNormalizer
       AIProvider        string // "anthropic", "openai", "cursor"
   }
   ```

2. **Uprav `internal/tool/runner/registry.go`** — `Register()` přijímá i `RunnerMeta`, `Get()` vrací `(Runner, RunnerMeta, error)`.

3. **Uprav `internal/worker/executor.go`** — nahraď hardcoded switch (řádky ~574-579, ~607-612, ~991-997, ~1005-1010) za lookup z registry metadata. Oba switche se vyskytují v `runStep()` i `executeReview()`.

4. **Uprav stávající runnery** (`claude.go`, `codex.go`) — při registraci v `cmd/codeforge/main.go` předej metadata:
   - claude-code: `RunnerMeta{NormalizerFactory: NewClaudeNormalizer, AIProvider: "anthropic"}`
   - codex: `RunnerMeta{NormalizerFactory: NewCodexNormalizer, AIProvider: "openai"}`

5. **Testy** — uprav existující testy v `internal/tool/runner/` aby odpovídaly novému API.

### Ověření:
```bash
task test
task lint
```

---

## Fáze 2: Cursor CLI Runner

### Úkoly:

1. **Config** — v `internal/config/config.go` přidej `CursorConfig` struct do `CLIConfig`:
   ```go
   type CursorConfig struct {
       Path         string   `koanf:"path"`
       DefaultModel string   `koanf:"default_model"`
       Models       []string `koanf:"models"`
   }
   ```
   Defaulty: path `"cursor-agent"`, default_model `""`, models `["composer-2"]`.

2. **Runner** — nový soubor `internal/tool/runner/cursor.go`:
   - Command: `cursor-agent -p <prompt> --output-format stream-json --force --workspace <workDir>`
   - Model flag: `--model <model>` pokud specifikován
   - API key: env var `CURSOR_API_KEY`
   - NDJSON parsing stdout, forwarding events přes `OnEvent`
   - Extract result z `type:result, subtype:success` event
   - Token usage: Cursor neexponuje tokens → `RunResult.InputTokens`/`OutputTokens` = 0
   - Nepodporuje `--mcp-config` — ignoruj `MCPConfigPath` v `RunOptions`
   - Nepodporuje `--max-turns`, `--max-budget-usd` — ignoruj tyto options
   - `AppendSystemPrompt` — prepend k promptu (jako Codex)
   - `AllowedTools` — ignoruj (Cursor nemá tento flag)

3. **Normalizer** — nový soubor `internal/tool/runner/normalizer_cursor.go`:
   - Mapování Cursor stream-json events → `NormalizedEvent`:
     - `"type":"system","subtype":"init"` → `EventSystem`
     - `"type":"assistant"` s `message.content[]` → `EventText`
     - `"type":"tool_call","subtype":"started"` → `EventToolUse`
     - `"type":"tool_call","subtype":"completed"` → `EventToolResult`
     - `"type":"result","subtype":"success"` → `EventResult`

4. **Registrace** — v `cmd/codeforge/main.go`:
   ```go
   cliRegistry.Register("cursor", runner.NewCursorRunner(cfg.CLI.Cursor.Path), runner.RunnerMeta{
       NormalizerFactory: runner.NewCursorNormalizer,
       AIProvider:        "cursor",
   })
   ```
   Přidej "cursor" do `cliConfigs` mapy a `DefaultModels` mapy.

5. **Testy** — unit testy pro runner i normalizer, table-driven, vedle source souborů.

6. **Config example** — uprav `configs/codeforge.example.yaml`:
   ```yaml
   cli:
     cursor:
       path: "cursor-agent"
       default_model: ""
       models: ["composer-2"]
   ```

### Ověření:
```bash
task test
task lint
```

---

## Fáze 3: Claude Agent Runner

### Úkoly:

1. **Runner** — nový soubor `internal/tool/runner/claude_agent.go`:
   - Thin wrapper kolem stávajícího `ClaudeRunner` logiky
   - Přidá `--bare` flag (skip auto-discovery hooks, skills, plugins, MCP, CLAUDE.md)
   - Stejná binárka jako claude-code (reuse `cfg.CLI.ClaudeCode.Path`)
   - Stejný output formát → reuse `ClaudeNormalizer`

2. **Registrace** — v `cmd/codeforge/main.go`:
   ```go
   cliRegistry.Register("claude-agent", runner.NewClaudeAgentRunner(cfg.CLI.ClaudeCode.Path), runner.RunnerMeta{
       NormalizerFactory: runner.NewClaudeNormalizer,
       AIProvider:        "anthropic",
   })
   ```

3. **Testy** — unit testy, table-driven.

### Ověření:
```bash
task test
task lint
```

---

## Fáze 4: Subscription Model (Dual-Auth)

### Úkoly:

1. **SQLite migrace** — nový soubor `internal/database/migrations/002_tenants.sql`:
   ```sql
   CREATE TABLE tenants (
       id TEXT PRIMARY KEY,
       name TEXT NOT NULL,
       slug TEXT UNIQUE NOT NULL,
       tier TEXT NOT NULL DEFAULT 'free',
       api_token_hash TEXT NOT NULL,
       max_sessions_per_day INTEGER NOT NULL DEFAULT 10,
       max_concurrent_sessions INTEGER NOT NULL DEFAULT 2,
       max_budget_usd_per_session REAL NOT NULL DEFAULT 1.0,
       allowed_clis TEXT NOT NULL DEFAULT '["claude-code"]',
       allowed_models TEXT,
       created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
       updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
   );

   CREATE TABLE key_pool (
       id TEXT PRIMARY KEY,
       provider TEXT NOT NULL,
       encrypted_token TEXT NOT NULL,
       weight INTEGER NOT NULL DEFAULT 1,
       active BOOLEAN NOT NULL DEFAULT 1,
       created_at DATETIME DEFAULT CURRENT_TIMESTAMP
   );

   CREATE TABLE usage_logs (
       id TEXT PRIMARY KEY,
       tenant_id TEXT NOT NULL REFERENCES tenants(id),
       session_id TEXT NOT NULL,
       cli TEXT NOT NULL,
       model TEXT,
       input_tokens INTEGER DEFAULT 0,
       output_tokens INTEGER DEFAULT 0,
       estimated_cost_usd REAL DEFAULT 0,
       created_at DATETIME DEFAULT CURRENT_TIMESTAMP
   );

   CREATE INDEX idx_usage_tenant_date ON usage_logs(tenant_id, created_at);
   ```

2. **Tenant package** — nový `internal/tenant/`:
   - `model.go` — Tenant struct, tier constants
   - `store.go` — SQLite CRUD (Create, Get, GetByTokenHash, List, Update, Delete)
   - `service.go` — business logic (ValidateSessionLimits, TrackUsage)
   - `keypool.go` — key pool management, weighted round-robin selection per-provider

3. **Config** — v `internal/config/config.go` přidej:
   ```go
   type SubscriptionConfig struct {
       Enabled bool                    `koanf:"enabled"`
       Tiers   map[string]TierConfig   `koanf:"tiers"`
   }
   type TierConfig struct {
       SessionsPerDay     int      `koanf:"sessions_per_day"`
       ConcurrentSessions int      `koanf:"concurrent_sessions"`
       MaxBudgetUSD       float64  `koanf:"max_budget_usd"`
       CLIs               []string `koanf:"clis"`
   }
   ```

4. **Auth middleware** — nový `internal/server/middleware/tenant_auth.go`:
   - Extrahuj Bearer token z `Authorization` header
   - SHA-256 hash → lookup v `tenants` tabulce
   - Pokud nalezen → přidej tenant do request context
   - Pokud nenalezen a žádný API key → 401
   - Koexistuje se stávajícím auth — pokud request má API key, middleware ho pustí dál beze změny

5. **Key resolver rozšíření** — v `internal/keys/resolver.go`:
   - Nový krok v resolution chain (krok 2): pokud request context obsahuje tenanta → vyber managed klíč z key_pool pro daného providera
   - Stávající kroky (inline key, SQLite registry, env var) beze změny

6. **Usage tracking** — v `internal/worker/executor.go`:
   - Po dokončení session: pokud context obsahuje tenanta → zapiš do `usage_logs`
   - Redis counter: `INCR usage:{tenant_id}:daily:{YYYY-MM-DD}`

7. **Admin API** — nový `internal/server/handlers/tenant.go`:
   - `POST /api/v1/admin/tenants` — vytvoří tenanta, vrátí Bearer token (jednorázově, plain text)
   - `GET /api/v1/admin/tenants` — list tenantů
   - `GET /api/v1/admin/tenants/{tenantID}` — detail tenanta
   - `GET /api/v1/admin/tenants/{tenantID}/usage` — usage stats (s query param `?period=7d`)
   - `PATCH /api/v1/admin/tenants/{tenantID}` — update tier/limits
   - `DELETE /api/v1/admin/tenants/{tenantID}` — smazat tenanta
   - `POST /api/v1/admin/key-pool` — přidat managed klíč
   - `GET /api/v1/admin/key-pool` — list klíčů v poolu
   - `DELETE /api/v1/admin/key-pool/{keyID}` — odebrat klíč

8. **Routes** — v `internal/server/server.go` přidej admin route group `/api/v1/admin/` s příslušnými handlery.

9. **Config example** — uprav `configs/codeforge.example.yaml`:
   ```yaml
   subscriptions:
     enabled: false
     tiers:
       free:
         sessions_per_day: 10
         concurrent_sessions: 2
         max_budget_usd: 1.0
         clis: ["claude-code"]
       pro:
         sessions_per_day: 100
         concurrent_sessions: 10
         max_budget_usd: 10.0
         clis: ["claude-code", "codex", "cursor", "claude-agent"]
       enterprise:
         sessions_per_day: -1
         concurrent_sessions: 50
         max_budget_usd: 50.0
         clis: ["claude-code", "codex", "cursor", "claude-agent"]
   ```

10. **Testy** — unit testy pro tenant store, service, keypool, middleware, handlery.

### Ověření:
```bash
task test
task lint
```

---

## Fáze 5: UI aktualizace

### Úkoly:

1. **CLI dropdown** — `web/src/components/NewSession.tsx`:
   - Nové CLI ("cursor", "claude-agent") se zobrazí automaticky z `GET /api/v1/cli` — ověř, že UI je dynamické a nepotřebuje hardcoded seznam.
   - Pokud jsou v UI hardcoded CLI názvy nebo ikony, přidej cursor a claude-agent.

2. **Typy** — `web/src/types/`:
   - Pokud existuje CLI enum/type, rozšiř o nové hodnoty.
   - Přidej typy pro tenant admin API pokud se bude dělat admin UI.

3. **Admin stránka (volitelné)** — pokud je čas, přidej jednoduchou admin stránku pro tenant management:
   - List tenantů s usage
   - Create tenant form
   - Jinak stačí API-only (admin přes curl/Postman).

### Ověření:
```bash
task ui:typecheck
task ui:lint
```

---

## Fáze 6: E2E testování

### A) API E2E testy

Spusť dev prostředí (`task dev`) a otestuj přes `curl` nebo Go integration testy:

1. **CLI endpoints:**
   ```bash
   # Ověř že nové CLI se zobrazují
   curl -s http://localhost:8080/api/v1/cli | jq '.[] | .name'
   # Očekávaný výstup: "claude-code", "codex", "cursor", "claude-agent"
   ```

2. **Tenant CRUD (pokud subscriptions enabled):**
   ```bash
   # Vytvoř tenanta
   curl -s -X POST http://localhost:8080/api/v1/admin/tenants \
     -H "Content-Type: application/json" \
     -d '{"name": "Test Tenant", "slug": "test-tenant", "tier": "pro"}' | jq .

   # List tenantů
   curl -s http://localhost:8080/api/v1/admin/tenants | jq .

   # Usage stats
   curl -s http://localhost:8080/api/v1/admin/tenants/{id}/usage?period=7d | jq .
   ```

3. **Session creation s API key (regression):**
   ```bash
   # Stávající flow MUSÍ fungovat beze změny
   curl -s -X POST http://localhost:8080/api/v1/sessions \
     -H "Content-Type: application/json" \
     -d '{"repo_url": "https://github.com/test/repo", "prompt": "test prompt", "config": {"cli": "claude-code"}}' | jq .status
   ```

4. **Session creation s Bearer token (subscription):**
   ```bash
   curl -s -X POST http://localhost:8080/api/v1/sessions \
     -H "Authorization: Bearer <tenant-token>" \
     -H "Content-Type: application/json" \
     -d '{"repo_url": "https://github.com/test/repo", "prompt": "test prompt", "config": {"cli": "cursor"}}' | jq .status
   ```

### B) UI E2E testy přes Chrome MCP

Otevři Chrome na `http://localhost:5173` (Vite dev server) a proveď tyto testy pomocí Chrome MCP nástrojů:

1. **CLI selection dropdown:**
   - Naviguj na stránku vytvoření nové session (`/sessions/new` nebo odpovídající route)
   - Ověř, že CLI dropdown obsahuje: "claude-code", "codex", "cursor", "claude-agent"
   - Vyber "cursor" → ověř, že model dropdown se aktualizuje na Cursor modely
   - Vyber "claude-agent" → ověř, že model dropdown ukazuje Anthropic modely

2. **Session creation form:**
   - Vyplň formulář s novým CLI (cursor nebo claude-agent)
   - Odešli → ověř, že session se vytvoří a zobrazí v seznamu
   - Ověř, že zvolený CLI je vidět v session detailu

3. **Stávající flow (regression):**
   - Vytvoř session s claude-code → ověř, že funguje stejně jako předtím
   - Ověř, že model dropdown pro claude-code ukazuje správné modely

4. **Admin stránka (pokud implementována):**
   - Naviguj na admin sekci
   - Ověř CRUD operace pro tenanty
   - Ověř zobrazení usage statistik

---

## Fáze 7: Commit & Push

Po úspěšném dokončení všech fází a testů:

```bash
# Zkontroluj stav
git status
git diff --stat

# Přidej soubory
git add internal/tool/runner/cursor.go \
        internal/tool/runner/cursor_test.go \
        internal/tool/runner/normalizer_cursor.go \
        internal/tool/runner/normalizer_cursor_test.go \
        internal/tool/runner/claude_agent.go \
        internal/tool/runner/claude_agent_test.go \
        internal/tool/runner/runner.go \
        internal/tool/runner/registry.go \
        internal/config/config.go \
        internal/database/migrations/002_tenants.sql \
        internal/tenant/ \
        internal/server/middleware/tenant_auth.go \
        internal/server/handlers/tenant.go \
        internal/server/server.go \
        internal/keys/resolver.go \
        internal/worker/executor.go \
        cmd/codeforge/main.go \
        configs/codeforge.example.yaml \
        web/src/

# Finální kontrola
task test
task lint
task ui:typecheck
task ui:lint

# Commit
git commit -m "feat: add Cursor + Claude Agent runners, subscription model, executor registry metadata

- Add CursorRunner with stream-json parsing and CursorNormalizer
- Add ClaudeAgentRunner (--bare mode, thin wrapper over ClaudeRunner)
- Refactor executor hardcoded CLI switches to registry-based metadata
- Add dual-auth: subscription model alongside existing API keys
- Add tenant model, key pool, usage tracking, admin API
- Add SQLite migration 002_tenants.sql (tenants, key_pool, usage_logs)
- Update config with CursorConfig and SubscriptionConfig
- UI: verify new CLIs appear in dropdown dynamically

Co-Authored-By: Claude <noreply@anthropic.com>"

# Push
git push origin main
```

---

## Poznámky

- **Všechno se vyvíjí v Dockeru** — `task dev` pro hot reload
- **Nikdy nepoužívej reálná jména repozitářů v testech**
- **Nikdy nebuilduj/pushuj Docker images lokálně** — vždy CI pipeline
- **Cursor token usage:** RunResult.InputTokens/OutputTokens budou 0 — Cursor je neexponuje
- **Claude Agent billing:** Od 15.6.2026 separátní "Agent SDK credit" track — dokumentuj v konfigu
- **Stávající API key flow se NESMÍ rozbít** — vždy testuj regression
