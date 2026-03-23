# Fáze 8: Smazat Redis input listener

**Commit:** `refactor: remove unused Redis input listener`

## Proč

`input:sessions` Redis kanál nemá žádného producenta v codebase. Je to mrtvý kód. Pokud bude potřeba, vrátí se jako feature s dokumentací a API kontraktem.

## Soubory ke smazání

| Soubor | Akce |
|--------|------|
| `internal/session/listener.go` | **Smazat** celý soubor |

## Wiring v main.go

Soubor: `cmd/codeforge/main.go`

Smazat:
```go
listener := session.NewListener(rdb, sessionService, "input:sessions")  // řádek 213
go listener.Start(appCtx)                                                // řádek 263
```

## Ověření

```bash
task build
task test
task lint
```
