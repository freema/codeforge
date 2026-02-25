# Phase 7: Tool System

## Cíl

Vytvořit plugin architekturu, kde CodeForge může při spouštění tasků využívat **externí nástroje** (Sentry, Jira, Chrome, Git, vlastní API). Tool system je **základ** pro všechny budoucí integrace.

## Klíčový koncept

V současné architektuře je Git "hardcoded" v executoru — clone, branch, push jsou pevně zadrátované kroky. Nová architektura:

- **Git = tool** (clone, commit, push, diff, create-pr)
- **Sentry = tool** (get-issue, list-errors, get-stacktrace)
- **Jira = tool** (get-ticket, update-status, add-comment)
- **Chrome = tool** (navigate, screenshot, click, fill)
- **Filesystem = tool** (read, write, search)
- AI agent **volí** které tooly použít na základě promptu

## Architektura

```
┌─────────────────────────────────────────────────┐
│                  TASK REQUEST                     │
│  prompt: "Použij Sentry, najdi bug #123,         │
│           oprav ho v tomto repu"                  │
│  tools: ["sentry", "git"]                        │
│  tool_config:                                     │
│    sentry:                                        │
│      dsn: "https://..."                           │
│      auth_token: "sntrys_..."                     │
└──────────────────────┬──────────────────────────-─┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│               TOOL RESOLVER                      │
│  1. Načti globální tooly (Redis registry)        │
│  2. Načti project tooly                          │
│  3. Načti task-level tooly                       │
│  4. Merge (task > project > global)              │
│  5. Validuj konfigurace                          │
│  6. Vrať resolved tool set                       │
└──────────────────────┬──────────────────────────-─┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│             TOOL → MCP BRIDGE                    │
│  Převeď tool definice na MCP servery:            │
│  - sentry tool → @sentry/mcp-server              │
│  - jira tool → @atlassian/jira-mcp-server        │
│  - chrome tool → @anthropic/chrome-mcp           │
│  - custom tool → user-provided MCP server        │
│                                                   │
│  Generuj .mcp.json pro Claude Code               │
└──────────────────────┬──────────────────────────-─┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│              CLAUDE CODE CLI                      │
│  Spustí se s .mcp.json → má přístup k toolům    │
│  Agent sám volí které tooly kdy zavolat          │
└─────────────────────────────────────────────────┘
```

## Vztah Tool System vs MCP

**MCP (Model Context Protocol)** je low-level protokol — definuje jak AI CLI komunikuje se servery. Stávající `internal/mcp/` to řeší na úrovni konfigurace `.mcp.json`.

**Tool System** je high-level abstrakce nad MCP:
- Uživatel řekne "chci Sentry tool" místo "nastav MCP server @sentry/mcp-server-remote s těmito args"
- CodeForge zná mapování tool → MCP server + potřebná konfigurace
- Validuje credentials, nastavuje env vars, řeší verze
- Přidává metadata (popisy, capabilities, usage tracking)

## Datový model

### Tool Definition

```go
// internal/tools/model.go

type ToolType string

const (
    ToolTypeMCP    ToolType = "mcp"     // mapuje na MCP server
    ToolTypeBuiltin ToolType = "builtin" // built-in implementace (git, filesystem)
    ToolTypeCustom ToolType = "custom"   // user-provided MCP server
)

type ToolDefinition struct {
    Name        string            `json:"name" validate:"required"`
    Type        ToolType          `json:"type" validate:"required"`
    Description string            `json:"description"`
    Version     string            `json:"version"`

    // MCP mapping (pro ToolTypeMCP)
    MCPPackage  string            `json:"mcp_package,omitempty"`  // npm package
    MCPCommand  string            `json:"mcp_command,omitempty"`  // command to run
    MCPArgs     []string          `json:"mcp_args,omitempty"`     // static args

    // Configuration schema
    RequiredConfig []ConfigField  `json:"required_config,omitempty"`
    OptionalConfig []ConfigField  `json:"optional_config,omitempty"`

    // Capabilities
    Capabilities []string         `json:"capabilities,omitempty"` // ["read_issues", "write_comments"]

    CreatedAt   time.Time         `json:"created_at"`
}

type ConfigField struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Type        string `json:"type"`     // "string", "secret", "url", "int"
    EnvVar      string `json:"env_var"`  // kam se mapuje v MCP env
    Sensitive   bool   `json:"sensitive"` // šifrovat v Redis
}
```

### Tool Instance (resolved pro task)

```go
type ToolInstance struct {
    Definition  *ToolDefinition
    Config      map[string]string  // resolved konfigurace
    Enabled     bool
}
```

### Task Request Extension

```go
// Rozšíření CreateTaskRequest
type CreateTaskRequest struct {
    // ... existující pole ...

    Tools      []TaskTool `json:"tools,omitempty"`
}

type TaskTool struct {
    Name   string            `json:"name" validate:"required"`
    Config map[string]string `json:"config,omitempty"` // per-task override
}
```

## Redis klíče

| Klíč | Typ | Popis |
|------|-----|-------|
| `tool:def:{name}` | Hash | Definice toolu (globální) |
| `tool:def:_index` | Set | Seznam všech tool definic |
| `tool:project:{id}:{name}` | Hash | Project-level konfigurace |
| `tool:project:{id}:_index` | Set | Seznam project toolů |
| `tool:usage:{task_id}` | List | Logování použití toolů v tasku |

## Nové soubory

```
internal/tools/
  model.go          — datový model (ToolDefinition, ToolInstance, ConfigField)
  registry.go       — CRUD operace v Redis (registrace, seznam, mazání)
  resolver.go       — resoluce toolů pro task (global → project → task merge)
  bridge.go         — konverze ToolInstance → MCP Server config
  catalog.go        — built-in tool definice (sentry, jira, chrome, git)
  validator.go      — validace tool konfigurací

internal/server/handlers/
  tools.go          — HTTP handlery (POST/GET/DELETE /tools)
```

## API Endpointy

### Tool Definitions (admin)

```
POST   /api/v1/tools              — Registruj nový tool
GET    /api/v1/tools              — Seznam toolů
GET    /api/v1/tools/{name}       — Detail toolu
DELETE /api/v1/tools/{name}       — Smaž tool
PUT    /api/v1/tools/{name}       — Aktualizuj tool
```

### Tool Catalog (read-only)

```
GET    /api/v1/tools/catalog      — Seznam built-in toolů s popisem
```

### Project Tools

```
POST   /api/v1/projects/{id}/tools     — Nastav tool pro projekt
GET    /api/v1/projects/{id}/tools     — Seznam project toolů
DELETE /api/v1/projects/{id}/tools/{n} — Smaž project tool
```

### Task Request (rozšíření)

```json
POST /api/v1/tasks
{
  "repo_url": "https://github.com/user/repo",
  "prompt": "Použij Sentry, najdi bug PROJ-123 a oprav ho",
  "tools": [
    {
      "name": "sentry",
      "config": {
        "auth_token": "sntrys_...",
        "organization": "my-org",
        "project": "my-project"
      }
    },
    {
      "name": "git"
    }
  ]
}
```

## Nová závislost

```bash
go get github.com/alicebob/miniredis/v2  # In-memory Redis pro unit testy
```

## Tasky

### 7.1 — Datový model a typy
- [ ] Vytvořit `internal/tools/model.go` s typy `ToolDefinition`, `ToolInstance`, `ConfigField`, `ToolType`
- [ ] Vytvořit `internal/tools/catalog.go` s built-in definicemi (sentry, jira, chrome, git)
- [ ] Definovat `ConfigField` schéma pro každý built-in tool
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (`internal/tools/model_test.go`):**
- [ ] `TestToolDefinition_JSON_MarshalUnmarshal` — table-driven: marshal → unmarshal round-trip pro každý ToolType
- [ ] `TestToolDefinition_JSON_OmitEmpty` — ověřit že omitempty pole se nevypisují
- [ ] `TestConfigField_SensitiveFlag` — ověřit že sensitive pole se správně serializují
- [ ] `TestToolType_Valid` — validní hodnoty: "mcp", "builtin", "custom"
- [ ] `TestCatalog_AllBuiltins` — ověřit že každý built-in tool má name, description, ≥1 capability

```go
// Příklad test pattern:
func TestToolDefinition_JSON_RoundTrip(t *testing.T) {
    tests := []struct {
        name string
        tool ToolDefinition
    }{
        {name: "mcp tool", tool: ToolDefinition{Name: "sentry", Type: ToolTypeMCP, MCPPackage: "@sentry/mcp"}},
        {name: "builtin tool", tool: ToolDefinition{Name: "git", Type: ToolTypeBuiltin}},
        {name: "custom tool", tool: ToolDefinition{Name: "my-db", Type: ToolTypeCustom}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            data, err := json.Marshal(tt.tool)
            if err != nil { t.Fatalf("Marshal: %v", err) }
            var got ToolDefinition
            if err := json.Unmarshal(data, &got); err != nil { t.Fatalf("Unmarshal: %v", err) }
            if got.Name != tt.tool.Name { t.Errorf("Name = %q, want %q", got.Name, tt.tool.Name) }
        })
    }
}
```

### 7.2 — Tool Registry (Redis)
- [ ] Vytvořit `internal/tools/registry.go` — CRUD operace přes Redis
- [ ] Global tools: `tool:def:{name}`, `tool:def:_index` (HSET + SADD)
- [ ] Project tools: `tool:project:{id}:{name}`, `tool:project:{id}:_index`
- [ ] Šifrování sensitive config polí (reuse `internal/crypto`)
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (`internal/tools/registry_test.go`) — miniredis:**
- [ ] `TestRegistry_CreateGlobal` — vytvoř tool, ověř HGETALL + SISMEMBER v miniredis
- [ ] `TestRegistry_CreateGlobal_Duplicate` — error při duplicitním jménu
- [ ] `TestRegistry_ListGlobal` — vytvoř 3 tooly, list vrátí všechny
- [ ] `TestRegistry_ListGlobal_Empty` — prázdný list, ne nil
- [ ] `TestRegistry_DeleteGlobal` — smaž tool, ověř SREM + DEL
- [ ] `TestRegistry_DeleteGlobal_NotFound` — error pro neexistující tool
- [ ] `TestRegistry_GetGlobal` — get konkrétního toolu
- [ ] `TestRegistry_CreateProject` — project-level tool storage
- [ ] `TestRegistry_SensitiveEncryption` — sensitive config zašifrovaný v Redis, dešifrovaný při čtení

```go
// Redis test pattern (miniredis):
func TestRegistry_CreateGlobal(t *testing.T) {
    s := miniredis.RunT(t)
    rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
    defer rdb.Close()

    cryptoSvc, _ := crypto.NewService("test-key-32-chars-exactly-here!")
    reg := NewRegistry(rdb, cryptoSvc)

    tool := ToolDefinition{Name: "sentry", Type: ToolTypeMCP, Description: "Sentry"}
    err := reg.CreateGlobal(context.Background(), tool)
    if err != nil { t.Fatalf("CreateGlobal: %v", err) }

    // Verify in miniredis directly
    name, err := s.HGet("codeforge:tool:def:sentry", "name")
    if err != nil { t.Fatalf("HGet: %v", err) }
    if name != "sentry" { t.Errorf("name = %q, want %q", name, "sentry") }

    if !s.SIsMember("codeforge:tool:def:_index", "sentry") {
        t.Error("tool not in index set")
    }
}
```

### 7.3 — Tool Resolver
- [ ] Vytvořit `internal/tools/resolver.go` — merging logika (global → project → task)
- [ ] Validace povinných config polí
- [ ] Dešifrování sensitive polí při resoluce
- [ ] `task fmt` + `task lint` MUSÍ projít

**Interface pro testovatelnost:**
```go
// Resolver závisí na abstrakci, ne na konkrétním Registry:
type ToolStore interface {
    ResolveGlobal(ctx context.Context, name string) (*ToolDefinition, error)
    ListGlobal(ctx context.Context) ([]ToolDefinition, error)
    ListProject(ctx context.Context, projectID string) ([]ToolDefinition, error)
}
```

**Testy (`internal/tools/resolver_test.go`) — mock store:**
- [ ] `TestResolver_GlobalOnly` — jen globální tooly, žádný project/task
- [ ] `TestResolver_MergePriority` — task config > project > global
- [ ] `TestResolver_MergePriority_Override` — task override přepíše globální config
- [ ] `TestResolver_RequiredConfigMissing` — error pokud chybí povinné pole
- [ ] `TestResolver_OptionalConfigMissing` — OK bez volitelných polí
- [ ] `TestResolver_UnknownTool` — error pro neregistrovaný tool
- [ ] `TestResolver_EmptyTools` — prázdný tool list → prázdný výsledek

```go
// Mock store pro unit testy (bez Redis):
type mockToolStore struct {
    globals  map[string]ToolDefinition
    projects map[string]map[string]ToolDefinition
}
func (m *mockToolStore) ResolveGlobal(ctx context.Context, name string) (*ToolDefinition, error) {
    if t, ok := m.globals[name]; ok { return &t, nil }
    return nil, ErrToolNotFound
}
```

### 7.4 — Tool → MCP Bridge
- [ ] Vytvořit `internal/tools/bridge.go` — konverze `ToolInstance` → `mcp.Server`
- [ ] Mapování config polí na MCP env vars
- [ ] Integrace s existujícím `internal/mcp/installer.go`
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (`internal/tools/bridge_test.go`):**
- [ ] `TestBridge_MCPTool` — ToolInstance s MCPPackage → mcp.Server se správným command/args/env
- [ ] `TestBridge_BuiltinTool` — builtin tool → správná MCP konverze
- [ ] `TestBridge_CustomTool` — custom tool → user-provided MCP config
- [ ] `TestBridge_EnvVarMapping` — config pole se mapují na správné env vars
- [ ] `TestBridge_SensitiveInEnv` — sensitive config hodnota je v env, ne v args
- [ ] `TestBridge_MultipleTools` — více toolů → více MCP serverů

### 7.5 — HTTP Handlers
- [ ] Vytvořit `internal/server/handlers/tools.go`
- [ ] Endpointy: CRUD pro tool definitions
- [ ] Endpoint: catalog (read-only built-in tools)
- [ ] Endpoint: project tools
- [ ] Validace requestů (go-playground/validator)
- [ ] Registrace rout v `server.go` — Chi route group: `r.Route("/tools", ...)`
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (`internal/server/handlers/tools_test.go`) — httptest + chi:**
- [ ] `TestToolHandler_Create_Success` — POST /tools s valid body → 201
- [ ] `TestToolHandler_Create_InvalidBody` — POST /tools bez name → 400 s validation errors
- [ ] `TestToolHandler_Create_Duplicate` — POST /tools duplicitní → 409
- [ ] `TestToolHandler_List` — GET /tools → 200 s array
- [ ] `TestToolHandler_Get` — GET /tools/{name} → 200
- [ ] `TestToolHandler_Get_NotFound` — GET /tools/{name} → 404
- [ ] `TestToolHandler_Delete` — DELETE /tools/{name} → 204
- [ ] `TestToolHandler_Catalog` — GET /tools/catalog → 200 s built-in tools

```go
// Handler test pattern (Chi + httptest):
func TestToolHandler_Create_Success(t *testing.T) {
    s := miniredis.RunT(t)
    rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
    defer rdb.Close()

    reg := NewRegistry(rdb, nil)
    handler := NewToolHandler(reg)

    r := chi.NewRouter()
    r.Post("/tools", handler.Create)

    body := `{"name":"test-tool","type":"mcp","description":"Test"}`
    req := httptest.NewRequest(http.MethodPost, "/tools", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    r.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
    }
}
```

### 7.6 — Task Request Extension
- [ ] Rozšířit `CreateTaskRequest` o `Tools []TaskTool`
- [ ] Rozšířit `TaskConfig` o tool konfigurace
- [ ] Upravit `Executor.Execute()` — volat tool resolver před MCP setup
- [ ] Backward compatible — tasky bez toolů fungují beze změn
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy:**
- [ ] `TestCreateTaskRequest_WithTools` — JSON decode/encode s tools array
- [ ] `TestCreateTaskRequest_WithoutTools` — existující request funguje beze změn
- [ ] `TestCreateTaskRequest_ToolValidation` — tool bez name → validation error

### 7.7 — Tool Usage Tracking
- [ ] Logování tool použití per task (`tool:usage:{task_id}`) — RPUSH do Redis
- [ ] Rozšířit stream events o tool-related eventy
- [ ] Nový event type: `"tool"` (tool_resolved, tool_configured)
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (miniredis):**
- [ ] `TestToolUsage_Log` — logování vytvoří záznam v Redis listu
- [ ] `TestToolUsage_MultipleTools` — více toolů → více záznamů

### 7.8 — Validator
- [ ] Vytvořit `internal/tools/validator.go`
- [ ] Validace config polí podle schématu (required/optional, type checking)
- [ ] Validace tool existence při task creation
- [ ] Srozumitelné chybové hlášky
- [ ] `task fmt` + `task lint` MUSÍ projít

**Testy (`internal/tools/validator_test.go`):**
- [ ] `TestValidator_RequiredPresent` — all required fields → no error
- [ ] `TestValidator_RequiredMissing` — missing required field → error s field name
- [ ] `TestValidator_TypeURL_Valid` — "https://example.com" → OK
- [ ] `TestValidator_TypeURL_Invalid` — "not-a-url" → error
- [ ] `TestValidator_TypeInt_Valid` — "42" → OK
- [ ] `TestValidator_TypeInt_Invalid` — "abc" → error
- [ ] `TestValidator_TypeSecret_Accepted` — any non-empty string → OK
- [ ] `TestValidator_UnknownTool` — tool not in registry → error

### 7.9 — Integration testy
- [ ] `TestIntegration_ToolLifecycle` — registrace → task s toolem → ověření .mcp.json
- [ ] `TestIntegration_BackwardCompat` — task bez toolů funguje beze změn
- [ ] `TestIntegration_MergePriority` — global + project + task merge
- [ ] `TestIntegration_SensitiveEncryption` — sensitive config encrypted v Redis
- [ ] Build tag: `//go:build integration`

## Linter checklist

- [ ] Všechny errors ošetřeny (`errcheck`)
- [ ] Context propagován (`noctx`)
- [ ] Žádné nepoužité proměnné (`unused`)
- [ ] Cyklomatická složitost ≤15 (`gocyclo`)
- [ ] Konstanty pro opakující se stringy (`goconst`)
- [ ] `gofmt` + `goimports` formátování

## Otevřené otázky

1. **Tool versioning** — jak řešit update MCP serverů? Pinovat verze v definici?
2. **Tool health checks** — kontrolovat dostupnost toolu před spuštěním tasku?
3. **Tool permissions** — omezit které tooly může kdo používat?
4. **Custom tools** — umožnit uživatelům registrovat vlastní MCP servery jako tooly?
