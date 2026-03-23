# Fáze 6: Přepnout MCP/tool setup na fail-closed

**Commit:** `fix: fail session when tool/MCP setup fails`

## Problém

V `internal/worker/executor.go` funkce `setupMCP()` je fail-open:

```go
// Současný stav — session pokračuje BEZ nástrojů:
if err != nil {
    log.Warn("tool resolution failed (continuing without tools)", "error", err)
}
if err := e.mcpInstaller.Setup(...); err != nil {
    log.Warn("MCP setup failed (continuing without MCP)", "error", err)
    return ""
}
```

Pokud user zvolí Sentry tool a setup selže, CLI běží bez Sentry přístupu. Uživatel netuší, že tool nefunguje. Falešný dojem funkčnosti.

## Cílový stav

Pokud session **explicitně** požaduje tools (v `Config.Tools`), a tool setup selže → session failne.

## Kroky

### 1. Upravit setupMCP() v executor.go

```go
func (e *Executor) setupMCP(ctx context.Context, t *session.Session, workDir string) (string, error) {
    // ... resolve tools ...
    if err != nil {
        // Fail-closed: pokud session požaduje tools a resolve selže
        if len(t.Config.Tools) > 0 {
            return "", fmt.Errorf("tool resolution failed: %w", err)
        }
        // Pokud žádné tools nebyly požadovány, jen warning
        log.Warn("tool resolution failed (no tools requested, continuing)", "error", err)
    }

    // ... setup MCP ...
    if err := e.mcpInstaller.Setup(...); err != nil {
        if len(taskMCPServers) > 0 {
            return "", fmt.Errorf("MCP setup failed: %w", err)
        }
        log.Warn("MCP setup failed (no servers configured, continuing)", "error", err)
        return "", nil
    }
}
```

### 2. Zajistit, že executor.Execute() propaguje error

Zkontrolovat, že error z `setupMCP()` vede na `handleRunError()` → session status = `failed`.

## Soubory

| Soubor | Akce |
|--------|------|
| `internal/worker/executor.go` | Přepsat `setupMCP()` na fail-closed pro explicitně požadované tools |

## Pravidlo

- **Session požaduje tools** (`Config.Tools` neprázdné) + setup selže → **FAIL**
- **Session nepožaduje tools** + resolve selže → warning, pokračuj (backward compat)
- **MCP servers nakonfigurované** + install selže → **FAIL**
- **Žádné MCP servers** → pokračuj normálně

## Ověření

```bash
task test
# Manuálně: vytvořit session se Sentry toolem + nevalidním klíčem → musí failnout
# Manuálně: vytvořit session bez tools → musí projít
```
