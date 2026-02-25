# Testing Conventions for CodeForge

## Princip

Každý task MUSÍ být testovatelný. Žádný kód bez testu neprojde review. Linter a formatter MUSÍ projít.

## Pravidla

### 1. Každý nový soubor má odpovídající `_test.go`

```
internal/tools/registry.go      → internal/tools/registry_test.go
internal/tools/resolver.go      → internal/tools/resolver_test.go
internal/session/service.go     → internal/session/service_test.go
```

### 2. Table-driven testy (povinný pattern)

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:  "valid input",
            input: InputType{...},
            want:  OutputType{...},
        },
        {
            name:    "empty input returns error",
            input:   InputType{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := DoSomething(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("DoSomething() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && got != tt.want {
                t.Errorf("DoSomething() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 3. Pure stdlib testing (žádný testify)

Projekt nepoužívá testify. Zůstáváme u `testing.T` s `t.Errorf`, `t.Fatalf`, `t.Helper()`.

```go
// SPRÁVNĚ:
if got != want {
    t.Errorf("Get() = %q, want %q", got, want)
}

// ŠPATNĚ (nepoužíváme testify):
// assert.Equal(t, want, got)
```

### 4. miniredis pro unit testy Redis logiky

Pro unit testy, které potřebují Redis, používáme `github.com/alicebob/miniredis/v2`. Podporuje všechny operace, které CodeForge používá (Hash, List, Set, Pub/Sub, TTL, Transactions).

```go
import (
    "testing"
    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
)

func TestRegistryCreate(t *testing.T) {
    s := miniredis.RunT(t) // automatický cleanup

    rdb := redis.NewClient(&redis.Options{
        Addr: s.Addr(),
    })
    defer rdb.Close()

    // Testuj registry s in-memory Redis
    registry := NewRegistry(rdb)
    // ...

    // Ověř stav přímo v miniredis
    got, err := s.HGet("tool:def:sentry", "name")
    if err != nil {
        t.Fatalf("HGet() error: %v", err)
    }
    if got != "sentry" {
        t.Errorf("name = %q, want %q", got, "sentry")
    }
}
```

**miniredis podporuje:**
- Hash: HSET, HGET, HGETALL, HDEL, HEXISTS ✓
- List: RPUSH, BLPOP, LPOP, LRANGE, LLEN ✓
- Set: SADD, SMEMBERS, SREM, SCARD ✓
- Pub/Sub: SUBSCRIBE, PUBLISH, PSUBSCRIBE ✓
- Transactions: WATCH, MULTI, EXEC ✓
- TTL: EXPIRE, TTL, PTTL ✓
- Sorted Set: ZADD, ZRANGEBYSCORE (pro rate limiting) ✓

### 5. Interfaces pro testovatelnost

Každá závislost MUSÍ být za interface, aby šla mockovat bez frameworku:

```go
// SPRÁVNĚ — interface v consumer package:
type TokenResolver interface {
    ResolveToken(ctx context.Context, task *Task) (string, error)
}

type Executor struct {
    tokenResolver TokenResolver
    // ...
}

// V testu:
type mockTokenResolver struct {
    token string
    err   error
}

func (m *mockTokenResolver) ResolveToken(ctx context.Context, task *Task) (string, error) {
    return m.token, m.err
}
```

### 6. Test helpers

Sdílené helpery v `internal/testutil/` (pokud potřeba):

```go
// internal/testutil/redis.go
package testutil

import (
    "testing"
    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
)

// SetupRedis creates a miniredis instance and go-redis client for testing.
func SetupRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
    t.Helper()
    s := miniredis.RunT(t)
    rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
    t.Cleanup(func() { rdb.Close() })
    return s, rdb
}
```

### 7. Integration testy

- Build tag: `//go:build integration`
- Spuštění: `task test:integration`
- Vyžadují Docker (Redis, app server)
- Testují celý flow přes HTTP API

### 8. Test coverage

- Cíl: **80%+ coverage** pro nový kód
- Kritické cesty (šifrování, state machine, auth): **95%+**
- `task test` zahrnuje `-cover` flag

### 9. Co testovat u každé komponenty

| Komponenta | Unit testy | Integration testy |
|------------|-----------|-------------------|
| Model (typy, marshaling) | JSON marshal/unmarshal, validace | — |
| Registry (CRUD Redis) | miniredis: Create/Get/List/Delete | Reálný Redis v Docker |
| Resolver (merge logika) | Mock registry, merge priority | — |
| Bridge (konverze) | Input → output mapping | — |
| Handler (HTTP) | httptest.NewRecorder, Chi router | API testy přes HTTP |
| Config (koanf) | env var override, defaults | — |
| State machine | Všechny platné/neplatné přechody | — |
| Prompt builder | Formátování, truncation | — |
| Classifier | Různé typy promptů → typy commitů | — |

## Linter a Formatter

### gofmt

```bash
task fmt   # Runs: gofmt -w .
```

Veškerý kód MUSÍ být formátovaný přes `gofmt`. CI to kontroluje.

### golangci-lint

```bash
task lint  # Runs: golangci-lint run ./...
```

Konfigurace viz `.golangci.yml` v root projektu.

**Hlavní pravidla:**
- `errcheck` — všechny errors musí být ošetřeny
- `govet` — vet checks
- `staticcheck` — statická analýza
- `unused` — žádný nepoužitý kód
- `gosec` — security checks
- `bodyclose` — HTTP body musí být uzavřen
- `noctx` — HTTP requesty musí mít context
- `exportloopref` — loop variable capture
- `goconst` — opakující se stringy → konstanty
- `gocyclo` — cyklomatická složitost (max 15)

**Běžné chyby:**
```go
// ŠPATNĚ — errcheck failure:
json.Unmarshal(data, &result)

// SPRÁVNĚ:
if err := json.Unmarshal(data, &result); err != nil {
    return fmt.Errorf("unmarshal: %w", err)
}

// ŠPATNĚ — noctx:
http.Get(url)

// SPRÁVNĚ:
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
resp, err := http.DefaultClient.Do(req)

// ŠPATNĚ — bodyclose:
resp, _ := http.Get(url)
// missing resp.Body.Close()

// SPRÁVNĚ:
resp, err := http.DefaultClient.Do(req)
if err != nil { return err }
defer resp.Body.Close()
```

### nolint direktivy

Používat POUZE s odůvodněním:

```go
//nolint:errcheck // fire-and-forget logging, error is not actionable
logger.Sync()
```

## Checklist pro každý PR

- [ ] Všechny nové soubory mají `_test.go`
- [ ] Testy pokrývají happy path i error cases
- [ ] Table-driven pattern kde je to vhodné
- [ ] `task test` prochází (unit testy)
- [ ] `task lint` prochází (zero warnings)
- [ ] `task fmt` nic nezměnil (kód je formátovaný)
- [ ] Interfaces pro závislosti (testovatelnost)
- [ ] Žádné hardcoded credentials v testech
- [ ] Integration testy pro Redis-dependent kód
