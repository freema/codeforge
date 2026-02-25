# Phase 8: Built-in Tools

## Cíl

Implementovat konkrétní built-in tool integrace, které CodeForge nabídne "z krabice". Každý tool = MCP server, který CodeForge automaticky nakonfiguruje.

## Závislosti

- **Phase 7: Tool System** — registr, resolver, bridge musí existovat

## Tool: Sentry

### Účel
AI agent se podívá do Sentry, najde konkrétní bug/issue, přečte stacktrace a opraví ho v kódu.

### MCP Server
- **Package:** `@sentry/mcp-server` (npm, v0.29.0+)
- **Repository:** [github.com/getsentry/sentry-mcp](https://github.com/getsentry/sentry-mcp)
- **Typ:** Dva transporty — Remote HTTP a Stdio

### Transport: Remote HTTP (SaaS Sentry — doporučeno)
```json
{
  "mcpServers": {
    "sentry": {
      "type": "http",
      "url": "https://mcp.sentry.dev/mcp"
    }
  }
}
```
- Autentizace přes OAuth (browser flow)
- Nic se neinstaluje lokálně

### Transport: Stdio (self-hosted Sentry)
```json
{
  "mcpServers": {
    "sentry": {
      "type": "stdio",
      "command": "npx",
      "args": ["@sentry/mcp-server@latest"],
      "env": {
        "SENTRY_ACCESS_TOKEN": "<token>",
        "SENTRY_HOST": "sentry.example.com",
        "EMBEDDED_AGENT_PROVIDER": "anthropic",
        "ANTHROPIC_API_KEY": "<key>"
      }
    }
  }
}
```

### Env Variables
| Proměnná | Povinná | Popis |
|----------|---------|-------|
| `SENTRY_ACCESS_TOKEN` | Ano (stdio) | Sentry User Auth Token (scopes: org:read, project:read/write, team:read/write, event:write) |
| `SENTRY_HOST` | Self-hosted | Hostname (ne celá URL), např. `sentry.example.com` |
| `EMBEDDED_AGENT_PROVIDER` | Pro AI search | `"openai"` nebo `"anthropic"` — auto-detection je deprecated |
| `ANTHROPIC_API_KEY` | Pokud provider=anthropic | Pro AI-powered search tools |

### Dostupné nástroje (16+)
| Tool | Popis |
|------|-------|
| `find_organizations` | Seznam organizací |
| `find_projects` / `list_projects` | Seznam projektů v organizaci |
| `search_issues` | AI-powered vyhledávání issues (vyžaduje LLM provider) |
| `search_events` | AI-powered vyhledávání eventů (vyžaduje LLM provider) |
| `get_issue_details` | Detail konkrétního issue |
| `get_event` | Analýza konkrétního Sentry eventu |
| `list_tags` | Tagy asociované s issues/eventy |
| `analyze_with_seer` | Sentry AI agent (Seer) — root cause analýza |
| `create_project` | Vytvoření projektu |
| `create_team` | Vytvoření týmu |
| `list_dsns` | DSN (Data Source Name) pro projekt |
| `create_metric_alert` | Metrický alert rule |
| `create_project_alert_rule` | Projektový alert rule s conditions/actions/filters |

### Konfigurace v CodeForge

```go
var SentryTool = ToolDefinition{
    Name:        "sentry",
    Type:        ToolTypeMCP,
    Description: "Access Sentry error tracking — read issues, stacktraces, error details, AI root cause analysis",
    MCPPackage:  "@sentry/mcp-server",
    RequiredConfig: []ConfigField{
        {Name: "auth_token", Description: "Sentry User Auth Token", Type: "secret", EnvVar: "SENTRY_ACCESS_TOKEN", Sensitive: true},
    },
    OptionalConfig: []ConfigField{
        {Name: "host", Description: "Self-hosted Sentry hostname", Type: "string", EnvVar: "SENTRY_HOST"},
        {Name: "agent_provider", Description: "LLM provider for AI search (anthropic|openai)", Type: "string", EnvVar: "EMBEDDED_AGENT_PROVIDER"},
    },
    Capabilities: []string{"find_organizations", "find_projects", "search_issues", "search_events", "get_issue_details", "get_event", "analyze_with_seer", "create_metric_alert"},
}
```

### Use Cases
- "Podívej se na Sentry issue PROJ-123, najdi příčinu a oprav ji"
- "Zkontroluj Sentry, jestli nejsou nové unresolved errors v posledních 24h"
- "Oprav top 3 nejčastější chyby ze Sentry"

### Tasky

- [x] 8.1.1 — Ověřit aktuální Sentry MCP server package a verzi
- [ ] 8.1.2 — Definovat `SentryTool` v `catalog.go`
- [ ] 8.1.3 — Bridge: mapování config → MCP env vars
- [ ] 8.1.4 — Dokumentace: jak získat Sentry auth token
- [ ] 8.1.5 — Integration test: Sentry tool → .mcp.json konfigurace

---

## Tool: Jira

### Účel
AI agent čte Jira tickety, rozumí požadavkům, aktualizuje stav a přidává komentáře.

### Dostupné MCP Servery

Existují tři hlavní varianty:

#### 1. Official Atlassian Remote MCP Server (cloud-hosted)
- **URL:** `https://mcp.atlassian.com/v1/sse`
- **Repository:** [github.com/atlassian/atlassian-mcp-server](https://github.com/atlassian/atlassian-mcp-server)
- **Auth:** OAuth 2.1 (browser flow) — **nevhodné pro headless/Docker prostředí**
- **Omezení:** Pouze Jira Cloud, vyžaduje browser pro login
- **25 toolů:** Jira + Confluence (getJiraIssue, editJiraIssue, createJiraIssue, transitionJiraIssue, searchJiraIssuesUsingJql, addCommentToJiraIssue, getConfluencePage, createConfluencePage, searchConfluenceUsingCql...)

```json
{
  "mcpServers": {
    "atlassian": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://mcp.atlassian.com/v1/sse"]
    }
  }
}
```

#### 2. sooperset/mcp-atlassian (community, Python — DOPORUČENO pro CodeForge)
- **Package:** `mcp-atlassian` (PyPI) / Docker: `mcp/atlassian`
- **Repository:** [github.com/sooperset/mcp-atlassian](https://github.com/sooperset/mcp-atlassian)
- **Verze:** v0.13.0 (leden 2026)
- **Auth:** API token (Cloud), Personal Access Token (Server/DC), OAuth 2.0 (Cloud)
- **Transport:** stdio (default), SSE, streamable-http
- **30+ Jira toolů + Confluence toolů**

```json
{
  "mcpServers": {
    "atlassian": {
      "command": "uvx",
      "args": ["mcp-atlassian"],
      "env": {
        "JIRA_URL": "https://your-company.atlassian.net",
        "JIRA_USERNAME": "your.email@company.com",
        "JIRA_API_TOKEN": "your_api_token"
      }
    }
  }
}
```

#### 3. @aashari/mcp-server-atlassian-jira (community, Node.js)
- **Package:** `@aashari/mcp-server-atlassian-jira` (npm, v3.3.0)
- **Auth:** API token (env vars: ATLASSIAN_SITE_NAME, ATLASSIAN_USER_EMAIL, ATLASSIAN_API_TOKEN)
- **Omezení:** Pouze Jira Cloud, bez Confluence

### Env Variables (sooperset/mcp-atlassian)

| Proměnná | Povinná | Popis |
|----------|---------|-------|
| `JIRA_URL` | Ano | Jira instance URL (např. `https://company.atlassian.net`) |
| `JIRA_USERNAME` | Cloud | User email pro Cloud API token auth |
| `JIRA_API_TOKEN` | Cloud | Jira API token ([vytvořit zde](https://id.atlassian.com/manage-profile/security/api-tokens)) |
| `JIRA_PERSONAL_TOKEN` | Server/DC | Personal Access Token pro self-hosted Jira |
| `JIRA_SSL_VERIFY` | Ne | `false` pro self-signed certs |
| `JIRA_PROJECTS_FILTER` | Ne | Omezit na konkrétní projekty (např. `PROJ,DEV`) |
| `READ_ONLY_MODE` | Ne | `true` pro read-only přístup (disable write operace) |
| `ENABLED_TOOLS` | Ne | Povolit jen specifické tooly (např. `jira_search,jira_get_issue`) |

### Dostupné Jira nástroje (sooperset/mcp-atlassian, 30 toolů)

**Read operace:**
| Tool | Popis |
|------|-------|
| `jira_search` | Vyhledávání issues pomocí JQL |
| `jira_get_issue` | Detail konkrétního issue |
| `jira_get_all_projects` | Seznam všech projektů |
| `jira_get_project_issues` | Issues v projektu |
| `jira_get_worklog` | Worklogy pro issue |
| `jira_get_transitions` | Dostupné přechody stavu |
| `jira_search_fields` | Dostupná Jira pole |
| `jira_get_agile_boards` | Agile boardy |
| `jira_get_board_issues` | Issues na boardu |
| `jira_get_sprints_from_board` | Sprinty na boardu |
| `jira_get_sprint_issues` | Issues ve sprintu |
| `jira_get_issue_link_types` | Typy linků mezi issues |
| `jira_batch_get_changelogs` | Batch changelogy (Cloud only) |
| `jira_get_user_profile` | Profil uživatele |
| `jira_download_attachments` | Stažení příloh |
| `jira_get_project_versions` | Verze projektu |

**Write operace:**
| Tool | Popis |
|------|-------|
| `jira_create_issue` | Vytvoření nového issue |
| `jira_update_issue` | Aktualizace existujícího issue |
| `jira_delete_issue` | Smazání issue |
| `jira_batch_create_issues` | Batch vytvoření issues |
| `jira_add_comment` | Přidání komentáře |
| `jira_transition_issue` | Přechod do nového stavu |
| `jira_add_worklog` | Přidání worklogu |
| `jira_link_to_epic` | Připojení k epicu |
| `jira_create_sprint` | Vytvoření sprintu |
| `jira_update_sprint` | Aktualizace sprintu |
| `jira_create_issue_link` | Vytvoření linku mezi issues |
| `jira_remove_issue_link` | Odstranění linku |
| `jira_create_version` | Vytvoření verze projektu |
| `jira_batch_create_versions` | Batch vytvoření verzí |

### Konfigurace v CodeForge

```go
var JiraTool = ToolDefinition{
    Name:        "jira",
    Type:        ToolTypeMCP,
    Description: "Access Jira project management — read/create/update issues, transitions, sprints, comments, worklogs",
    MCPPackage:  "mcp-atlassian",  // PyPI package (uvx runner)
    MCPCommand:  "uvx",
    MCPArgs:     []string{"mcp-atlassian"},
    RequiredConfig: []ConfigField{
        {Name: "url", Description: "Jira instance URL", Type: "url", EnvVar: "JIRA_URL"},
        {Name: "username", Description: "Jira user email (Cloud)", Type: "string", EnvVar: "JIRA_USERNAME"},
        {Name: "api_token", Description: "Jira API token", Type: "secret", EnvVar: "JIRA_API_TOKEN", Sensitive: true},
    },
    OptionalConfig: []ConfigField{
        {Name: "personal_token", Description: "Personal Access Token (Server/DC)", Type: "secret", EnvVar: "JIRA_PERSONAL_TOKEN", Sensitive: true},
        {Name: "projects_filter", Description: "Limit to specific projects (e.g., PROJ,DEV)", Type: "string", EnvVar: "JIRA_PROJECTS_FILTER"},
        {Name: "read_only", Description: "Disable write operations", Type: "bool", EnvVar: "READ_ONLY_MODE"},
        {Name: "enabled_tools", Description: "Enable only specific tools", Type: "string", EnvVar: "ENABLED_TOOLS"},
    },
    Capabilities: []string{"jira_search", "jira_get_issue", "jira_create_issue", "jira_update_issue", "jira_transition_issue", "jira_add_comment", "jira_get_agile_boards", "jira_get_sprint_issues"},
}
```

### Doporučení

**Pro CodeForge: `sooperset/mcp-atlassian`** protože:
1. Funguje přes stdio — kompatibilní s `.mcp.json` architekturou
2. Auth přes env vars (API token) — funguje headless v Dockeru
3. Podporuje Cloud i Server/Data Center
4. Nejkompletnější sada nástrojů (30+ Jira + Confluence)
5. Docker image k dispozici (`mcp/atlassian`)
6. Security features: `JIRA_PROJECTS_FILTER`, `ENABLED_TOOLS`, `READ_ONLY_MODE`

### Use Cases
- "Přečti ticket PROJ-456, implementuj požadované změny"
- "Zkontroluj aktuální sprint, najdi bugy s prioritou High a oprav je"
- "Po dokončení aktualizuj status ticketu na Done a přidej komentář"
- "Vytvoř nový bug ticket pro nalezený problém"

### Tasky

- [x] 8.2.1 — Prozkoumat dostupné Jira MCP servery (Atlassian official vs community)
- [ ] 8.2.2 — Definovat `JiraTool` v `catalog.go`
- [ ] 8.2.3 — Bridge: mapování config → MCP env vars (uvx runner)
- [ ] 8.2.4 — Dokumentace: jak získat Jira API token
- [ ] 8.2.5 — Integration test

---

## Tool: Chrome / Browser Automation

### Účel
AI agent ovládá Chrome — naviguje na stránky, pořizuje screenshoty, interaguje s UI. Užitečné pro testování, scraping, vizuální debugging.

### Dostupné MCP Servery (srovnání)

| | `@playwright/mcp` | `chrome-devtools-mcp` | `@modelcontextprotocol/server-puppeteer` |
|---|---|---|---|
| **Maintainer** | Microsoft | Google Chrome team | Anthropic (archivováno) |
| **Verze** | 0.0.68 | 0.17.1 | 2025.5.12 |
| **Aktivně udržován** | Ano (velmi) | Ano (velmi) | Ne (archivováno) |
| **Počet nástrojů** | 21 core + 11 optional | 26 | ~7 |
| **Token usage** | ~13.7k (6.8%) | ~18.0k (9.0%) | ~5k |
| **Prohlížeče** | Chromium, Firefox, WebKit | Pouze Chrome | Pouze Chrome |
| **Selektory** | Accessibility tree (UID) | Accessibility tree (UID) | CSS selektory (křehké) |
| **CDP endpoint** | `--cdp-endpoint` | `--browser-url` | Via launch options |
| **Performance traces** | Ne | Ano (Core Web Vitals) | Ne |
| **Network inspekce** | Základní | Hluboká (request/response body) | Ne |
| **Emulace** | Ne | Ano (CPU, síť, geo, color scheme) | Ne |
| **Docker image** | Oficiální MCR image | Ne | Ne |

#### 1. @playwright/mcp (Microsoft — DOPORUČENO pro automatizaci)
- **npm:** `@playwright/mcp` (v0.0.68)
- **Repository:** [github.com/microsoft/playwright-mcp](https://github.com/microsoft/playwright-mcp)
- **Docker:** `mcr.microsoft.com/playwright/mcp`
- Nejlepší pro **cross-browser testování a UI automatizaci**
- Nižší token overhead, official Docker image

**Core nástroje (21):**
`browser_navigate`, `browser_click`, `browser_type`, `browser_fill_form`, `browser_select_option`, `browser_hover`, `browser_drag`, `browser_press_key`, `browser_file_upload`, `browser_handle_dialog`, `browser_evaluate`, `browser_run_code`, `browser_snapshot`, `browser_take_screenshot`, `browser_console_messages`, `browser_network_requests`, `browser_tabs`, `browser_resize`, `browser_wait_for`, `browser_close`, `browser_install`

**Optional (via `--caps=pdf,vision,testing`):**
`browser_pdf_save`, `browser_mouse_click_xy`, `browser_generate_locator`, `browser_verify_element_visible`, `browser_verify_text_visible`...

```json
{
  "mcpServers": {
    "playwright": {
      "command": "npx",
      "args": ["@playwright/mcp@latest", "--headless"]
    }
  }
}
```

**CDP připojení k existujícímu Chrome:**
```json
{
  "mcpServers": {
    "playwright": {
      "command": "npx",
      "args": ["@playwright/mcp@latest", "--cdp-endpoint", "http://127.0.0.1:9222"]
    }
  }
}
```

**Docker sidecar:**
```bash
docker run -d -i --rm --init --pull=always \
  --entrypoint node \
  --name playwright \
  -p 8931:8931 \
  mcr.microsoft.com/playwright/mcp \
  cli.js --headless --browser chromium --no-sandbox --port 8931
```

#### 2. chrome-devtools-mcp (Google — pro debugging/performance)
- **npm:** `chrome-devtools-mcp` (v0.17.1)
- **Repository:** [github.com/ChromeDevTools/chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp)
- Nejlepší pro **deep debugging, Core Web Vitals, network inspekci**

**26 nástrojů ve 6 kategoriích:**
- **Interakce:** `take_snapshot`, `take_screenshot`, `click`, `hover`, `fill`, `fill_form`, `drag`, `press_key`, `upload_file`, `wait_for`, `handle_dialog`
- **Správa stránek:** `navigate_page`, `new_page`, `close_page`, `select_page`, `list_pages`, `resize_page`
- **JavaScript:** `evaluate_script`
- **Emulace:** `emulate` (CPU throttling, síťové podmínky, geolokace, viewport, color scheme)
- **Síť/konzole:** `list_network_requests`, `get_network_request`, `list_console_messages`, `get_console_message`
- **Performance:** `performance_start_trace`, `performance_stop_trace`, `performance_analyze_insight`

```json
{
  "mcpServers": {
    "chrome-devtools": {
      "command": "npx",
      "args": ["chrome-devtools-mcp@latest", "--headless", "--viewport=1280x720"]
    }
  }
}
```

#### 3. @modelcontextprotocol/server-puppeteer (archivováno — NEPOUŽÍVAT)
Původní referenční implementace, nahrazena Playwright MCP a Chrome DevTools MCP. Archivováno, neudržováno.

### Konfigurace v CodeForge

```go
// Playwright MCP — primární browser tool
var PlaywrightTool = ToolDefinition{
    Name:        "browser",
    Type:        ToolTypeMCP,
    Description: "Cross-browser automation — navigate, screenshot, click, fill forms, test UI (Playwright)",
    MCPPackage:  "@playwright/mcp",
    RequiredConfig: []ConfigField{},
    OptionalConfig: []ConfigField{
        {Name: "cdp_endpoint", Description: "Chrome DevTools Protocol URL", Type: "url", EnvVar: "PLAYWRIGHT_MCP_CDP_ENDPOINT"},
        {Name: "headless", Description: "Run in headless mode (default: true)", Type: "bool"},
        {Name: "browser", Description: "Browser to use: chromium, firefox, webkit", Type: "string"},
        {Name: "viewport", Description: "Viewport size (e.g., 1280x720)", Type: "string"},
        {Name: "capabilities", Description: "Extra capabilities: pdf, vision, testing", Type: "string"},
    },
    Capabilities: []string{"browser_navigate", "browser_click", "browser_type", "browser_fill_form", "browser_take_screenshot", "browser_evaluate", "browser_snapshot", "browser_network_requests"},
}

// Chrome DevTools MCP — volitelný pro performance debugging
var ChromeDevToolsTool = ToolDefinition{
    Name:        "chrome-devtools",
    Type:        ToolTypeMCP,
    Description: "Chrome debugging — performance traces, Core Web Vitals, network inspection, emulation",
    MCPPackage:  "chrome-devtools-mcp",
    RequiredConfig: []ConfigField{},
    OptionalConfig: []ConfigField{
        {Name: "browser_url", Description: "Chrome DevTools URL", Type: "url"},
        {Name: "headless", Description: "Run in headless mode", Type: "bool"},
        {Name: "viewport", Description: "Viewport size (e.g., 1280x720)", Type: "string"},
    },
    Capabilities: []string{"take_snapshot", "take_screenshot", "navigate_page", "evaluate_script", "performance_start_trace", "performance_analyze_insight", "list_network_requests", "emulate"},
}
```

### Docker integrace — doporučení

**Varianta A: Docker sidecar (DOPORUČENO)**
```yaml
# docker-compose.yaml
services:
  playwright:
    image: mcr.microsoft.com/playwright/mcp
    entrypoint: ["node", "cli.js", "--headless", "--browser", "chromium", "--no-sandbox", "--port", "8931"]
    ports:
      - "8931:8931"
```
CodeForge se připojí přes `--cdp-endpoint http://playwright:9222` nebo SSE na portu 8931.

**Varianta B: Remote CDP** — uživatel poskytne CDP URL na existující Chrome instance.

### Use Cases
- "Otevři localhost:3000, udělej screenshot a popiš co vidíš"
- "Naviguj na login stránku, vyplň formulář, otestuj happy path"
- "Zkontroluj responsivitu na mobilních velikostech"
- "Spusť performance trace a analyzuj Core Web Vitals"
- "Najdi broken links na webu"

### Tasky

- [x] 8.3.1 — Prozkoumat Chrome MCP servery (Playwright MCP, Puppeteer MCP, DevTools MCP)
- [ ] 8.3.2 — Rozhodnout deployment model (sidecar Playwright — doporučeno)
- [ ] 8.3.3 — Definovat `PlaywrightTool` + `ChromeDevToolsTool` v `catalog.go`
- [ ] 8.3.4 — Docker compose: Playwright sidecar container
- [ ] 8.3.5 — Bridge: mapování config + CDP container discovery
- [ ] 8.3.6 — Integration test: browser tool → navigate + screenshot

---

## Tool: Git (refaktoring)

### Účel
Refaktorovat existující hardcoded Git operace v executoru na tool pattern. Git se stane standardním toolem, který agent může volat.

### Implementace
Na rozdíl od ostatních toolů, Git má dvě možnosti: MCP server pro agenta nebo builtin tool pro interní operace.

### Dostupné Git MCP Servery

| | `mcp-server-git` (Official) | `@cyanheads/git-mcp-server` | `github/github-mcp-server` |
|---|---|---|---|
| **Jazyk** | Python | TypeScript | Go |
| **Package** | PyPI | npm | Docker/binary |
| **Počet toolů** | 12 | 28 | 60+ (GitHub API) |
| **git status/diff/log** | Ano | Ano | Ne (API-based) |
| **git push/pull** | Ne | Ano | Ne (API-based) |
| **git clone** | Ne | Ano | Ne |
| **git merge/rebase** | Ne | Ano | Ne |
| **git blame** | Ne | Ne | Ne |
| **git stash/tag/worktree** | Ne | Ano | Ne |
| **Security sandbox** | Ne | Ano (GIT_BASE_DIR) | OAuth scopes |
| **Issues/PR integrace** | Ne | Ne | Ano (full GitHub API) |

#### 1. mcp-server-git (Official MCP reference, Python)
- **Package:** `mcp-server-git` (PyPI, v2026.1.14)
- **Repository:** [modelcontextprotocol/servers/tree/main/src/git](https://github.com/modelcontextprotocol/servers/tree/main/src/git)
- **12 toolů:** `git_status`, `git_diff_unstaged`, `git_diff_staged`, `git_diff`, `git_commit`, `git_add`, `git_reset`, `git_log`, `git_create_branch`, `git_checkout`, `git_show`, `git_branch`
- Read-heavy, minimální write (add, commit, reset, create_branch, checkout)

```json
{
  "mcpServers": {
    "git": {
      "command": "uvx",
      "args": ["mcp-server-git", "--repository", "/path/to/repo"]
    }
  }
}
```

#### 2. @cyanheads/git-mcp-server (community, TypeScript — nejkompletnější)
- **Package:** `@cyanheads/git-mcp-server` (npm)
- **Repository:** [github.com/cyanheads/git-mcp-server](https://github.com/cyanheads/git-mcp-server)
- **28 toolů** — kompletní pokrytí: clone, commit, push, pull, merge, rebase, cherry-pick, stash, tag, worktree, remote...
- Transport: stdio + HTTP
- Security: `GIT_BASE_DIR` sandbox, JWT/OAuth auth
- Safety: explicitní potvrzení pro destruktivní operace

```json
{
  "mcpServers": {
    "git": {
      "command": "npx",
      "args": ["@cyanheads/git-mcp-server"],
      "env": {
        "GIT_BASE_DIR": "/workspaces",
        "GIT_SIGN_COMMITS": "false"
      }
    }
  }
}
```

#### 3. github/github-mcp-server (GitHub API — doplňkový)
- **Package:** Docker `ghcr.io/github/github-mcp-server` (Go binary)
- **Repository:** [github.com/github/github-mcp-server](https://github.com/github/github-mcp-server)
- **Účel:** GitHub API přístup (issues, PRs, actions, code security) — NE lokální git operace
- 60+ nástrojů v 18 toolsetech (repos, issues, pull_requests, actions, code_security, discussions, notifications...)
- Staré `@modelcontextprotocol/server-github` npm je **deprecated** od dubna 2025

```json
{
  "mcpServers": {
    "github": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "ghcr.io/github/github-mcp-server"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "<token>"
      }
    }
  }
}
```

### Dvě úrovně:
1. **Git MCP tool** — AI agent má přístup přes MCP k Git operacím (diff, log, status, blame)
2. **Git internal** — CodeForge executor stále dělá clone/push/PR (ne agent)
3. **GitHub MCP tool** (volitelný) — AI agent čte issues, PRs, code security alerts

### Konfigurace v CodeForge

```go
// Git MCP — pro agenta (čtení historie, diff, status)
var GitTool = ToolDefinition{
    Name:        "git",
    Type:        ToolTypeMCP,
    Description: "Git operations — status, diff, log, branch, commit (via MCP server)",
    MCPPackage:  "@cyanheads/git-mcp-server",
    RequiredConfig: []ConfigField{},
    OptionalConfig: []ConfigField{
        {Name: "base_dir", Description: "Restrict operations to directory tree", Type: "string", EnvVar: "GIT_BASE_DIR"},
        {Name: "sign_commits", Description: "Enable commit signing", Type: "bool", EnvVar: "GIT_SIGN_COMMITS"},
    },
    Capabilities: []string{"git_status", "git_diff", "git_log", "git_show", "git_branch", "git_commit", "git_add", "git_checkout"},
}

// GitHub API tool (volitelný)
var GitHubTool = ToolDefinition{
    Name:        "github",
    Type:        ToolTypeMCP,
    Description: "GitHub API — issues, pull requests, actions, code security, discussions",
    MCPPackage:  "ghcr.io/github/github-mcp-server",  // Docker image
    MCPCommand:  "docker",
    MCPArgs:     []string{"run", "-i", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "ghcr.io/github/github-mcp-server"},
    RequiredConfig: []ConfigField{
        {Name: "token", Description: "GitHub Personal Access Token", Type: "secret", EnvVar: "GITHUB_PERSONAL_ACCESS_TOKEN", Sensitive: true},
    },
    OptionalConfig: []ConfigField{
        {Name: "toolsets", Description: "Enabled toolsets (repos,issues,pull_requests,actions...)", Type: "string", EnvVar: "GITHUB_TOOLSETS"},
        {Name: "read_only", Description: "Read-only mode", Type: "bool", EnvVar: "GITHUB_READ_ONLY"},
    },
    Capabilities: []string{"get_file_contents", "list_commits", "search_issues", "create_pull_request", "list_actions_workflows"},
}
```

### Tasky

- [x] 8.4.1 — Prozkoumat Git MCP servery (official, cyanheads, GitHub)
- [ ] 8.4.2 — Definovat `GitTool` + `GitHubTool` v `catalog.go`
- [ ] 8.4.3 — Rozhodnout scope: co zůstane v executoru vs co jde do MCP toolu
- [ ] 8.4.4 — Refaktor: executor Git operace abstrahovat za tool interface
- [ ] 8.4.5 — Integration test

---

## Tool: Custom (User-provided)

### Účel
Uživatelé mohou registrovat vlastní MCP servery jako tooly.

```json
POST /api/v1/tools
{
  "name": "my-database",
  "type": "custom",
  "description": "Access to production database (read-only)",
  "mcp_package": "@company/db-mcp-server",
  "mcp_args": ["--readonly"],
  "required_config": [
    {"name": "connection_string", "type": "secret", "env_var": "DB_CONNECTION", "sensitive": true}
  ]
}
```

### Tasky

- [ ] 8.5.1 — Custom tool registration endpoint
- [ ] 8.5.2 — Validace custom tool definic
- [ ] 8.5.3 — Security: sandboxing custom MCP serverů
- [ ] 8.5.4 — Dokumentace: jak vytvořit custom tool

---

## Testovací strategie

Každý built-in tool potřebuje tyto testy:

### Unit testy (per tool, `internal/tools/catalog_test.go`)

```go
func TestCatalog_SentryTool(t *testing.T) {
    tool := SentryTool
    // Povinná pole
    if tool.Name == "" { t.Error("Name is empty") }
    if tool.Type != ToolTypeMCP { t.Errorf("Type = %q, want mcp", tool.Type) }
    if tool.MCPPackage == "" { t.Error("MCPPackage is empty") }
    if tool.Description == "" { t.Error("Description is empty") }
    // Povinný config
    if len(tool.RequiredConfig) == 0 { t.Error("RequiredConfig is empty") }
    // auth_token musí být sensitive
    for _, f := range tool.RequiredConfig {
        if f.Name == "auth_token" && !f.Sensitive {
            t.Error("auth_token should be sensitive")
        }
    }
}
```

- [ ] `TestCatalog_SentryTool` — validace definice, required fields, sensitivity
- [ ] `TestCatalog_JiraTool` — validace definice, required fields
- [ ] `TestCatalog_ChromeTool` — validace definice
- [ ] `TestCatalog_GitTool` — validace definice
- [ ] `TestCatalog_AllToolsHaveUniqueNames` — žádné duplicity v katalogu
- [ ] `TestCatalog_AllSensitiveFieldsHaveEnvVar` — každé sensitive pole má EnvVar

### Bridge testy (per tool, `internal/tools/bridge_test.go`)

- [ ] `TestBridge_Sentry_MCP` — SentryTool + config → korektní mcp.Server s SENTRY_AUTH_TOKEN env
- [ ] `TestBridge_Jira_MCP` — JiraTool + config → korektní mcp.Server s JIRA_* env vars
- [ ] `TestBridge_Chrome_MCP` — ChromeTool + config → korektní mcp.Server
- [ ] `TestBridge_Git_Builtin` — GitTool → specifické handling pro builtin type

### Integration testy (`//go:build integration`)

- [ ] `TestIntegration_Sentry_MCPJson` — vytvoř task se Sentry toolem → ověř .mcp.json v workspace
- [ ] `TestIntegration_MultipleTools` — task s Sentry + Git → oba v .mcp.json

## Linter checklist

- [ ] Všechny tool definice jsou `var`, ne `const` (pro modifikovatelnost)
- [ ] Žádné hardcoded package verze v Go kódu (verze v config/env)
- [ ] `task fmt` + `task lint` MUSÍ projít

## Priorita toolů

| Tool | Priorita | Důvod |
|------|----------|-------|
| Sentry | Vysoká | Jasný use case, `@sentry/mcp-server` (v0.29.0+) s 16+ nástroji |
| Git | Vysoká | Refaktoring existujícího kódu; `@cyanheads/git-mcp-server` (28 toolů) nebo `mcp-server-git` (12 toolů) |
| GitHub | Vysoká | Doplněk ke Git; `github/github-mcp-server` (60+ toolů, issues, PRs, actions) |
| Jira | Střední | `sooperset/mcp-atlassian` (v0.13.0, 30+ toolů, Cloud + Server/DC, Docker image) |
| Browser | Střední | `@playwright/mcp` (21 toolů, Docker sidecar, cross-browser) |
| Chrome DevTools | Nízká | `chrome-devtools-mcp` (26 toolů, performance tracing) — volitelný doplněk k Playwright |
| Custom | Nízká | Framework first, custom later |
