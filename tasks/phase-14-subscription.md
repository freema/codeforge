# Phase 14: Subscription & Multi-User Auth

## Cíl

Přejít z single-tenant (jeden API token) na **multi-user systém** s:
- Per-user API klíči
- Usage tracking (tokeny, tasky, minuty)
- Subscription plány s limity
- Billing integration

## Závislosti

- Žádné přímé závislosti (ale ovlivní téměř vše)

## Současný stav

```go
// internal/server/middleware/auth.go
// Jeden Bearer token pro všechny requesty
// server.auth_token v configu
```

Žádné user identity, žádný usage tracking, žádné limity per user.

## Architektura

```
┌──────────────────────────────────────────────────────────────┐
│                      API REQUEST                              │
│  Authorization: Bearer cf_user_abc123_xxxxxxxx               │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│                   AUTH MIDDLEWARE                              │
│  1. Parse API key (prefix → user ID)                         │
│  2. Validate key exists in Redis                             │
│  3. Check subscription status (active, expired, suspended)   │
│  4. Check rate limits per user                               │
│  5. Check usage limits (tasks/month, tokens/month)           │
│  6. Set user context on request                              │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│                   USAGE TRACKER                               │
│  Per request:                                                 │
│  - Increment task count                                      │
│  - Track tokens used (after task completes)                  │
│  - Track compute time                                        │
│  - Log API call                                              │
└──────────────────────────────────────────────────────────────┘
```

## Datový model

### User

```go
// internal/user/model.go

type User struct {
    ID             string    `json:"id"`
    Email          string    `json:"email"`
    Name           string    `json:"name"`
    Plan           PlanType  `json:"plan"`
    Status         string    `json:"status"`  // active, suspended, expired
    APIKeys        []APIKey  `json:"api_keys"`
    UsageThisMonth *Usage    `json:"usage_this_month"`
    CreatedAt      time.Time `json:"created_at"`
}

type APIKey struct {
    ID        string    `json:"id"`
    Key       string    `json:"-"`           // nikdy nevrátit v API
    KeyHash   string    `json:"-"`           // SHA256 hash pro lookup
    Prefix    string    `json:"prefix"`      // "cf_user_abc123_" (pro identifikaci)
    Name      string    `json:"name"`        // user-friendly název
    Scopes    []string  `json:"scopes"`      // ["tasks:write", "tasks:read", "admin"]
    LastUsed  time.Time `json:"last_used"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at,omitempty"`
}
```

### Subscription Plans

```go
type PlanType string

const (
    PlanFree       PlanType = "free"
    PlanPro        PlanType = "pro"
    PlanTeam       PlanType = "team"
    PlanEnterprise PlanType = "enterprise"
)

type Plan struct {
    Type              PlanType `json:"type"`
    Name              string   `json:"name"`
    TasksPerMonth     int      `json:"tasks_per_month"`      // -1 = unlimited
    TokensPerMonth    int64    `json:"tokens_per_month"`     // -1 = unlimited
    MaxConcurrent     int      `json:"max_concurrent"`
    MaxTaskTimeout    int      `json:"max_task_timeout_min"`
    MaxPipelineAgents int      `json:"max_pipeline_agents"`
    ToolsEnabled      bool     `json:"tools_enabled"`
    ReviewEnabled     bool     `json:"review_enabled"`
    SessionsEnabled   bool     `json:"sessions_enabled"`
    PriceUSD          float64  `json:"price_usd"`
}

var Plans = map[PlanType]Plan{
    PlanFree: {
        Type:              PlanFree,
        Name:              "Free",
        TasksPerMonth:     50,
        TokensPerMonth:    1_000_000,
        MaxConcurrent:     1,
        MaxTaskTimeout:    10,
        MaxPipelineAgents: 2,
        ToolsEnabled:      false,
        ReviewEnabled:     false,
        SessionsEnabled:   false,
        PriceUSD:          0,
    },
    PlanPro: {
        Type:              PlanPro,
        Name:              "Pro",
        TasksPerMonth:     500,
        TokensPerMonth:    20_000_000,
        MaxConcurrent:     3,
        MaxTaskTimeout:    30,
        MaxPipelineAgents: 5,
        ToolsEnabled:      true,
        ReviewEnabled:     true,
        SessionsEnabled:   true,
        PriceUSD:          49,
    },
    PlanTeam: {
        Type:              PlanTeam,
        Name:              "Team",
        TasksPerMonth:     2000,
        TokensPerMonth:    100_000_000,
        MaxConcurrent:     10,
        MaxTaskTimeout:    60,
        MaxPipelineAgents: 10,
        ToolsEnabled:      true,
        ReviewEnabled:     true,
        SessionsEnabled:   true,
        PriceUSD:          199,
    },
    PlanEnterprise: {
        Type:              PlanEnterprise,
        Name:              "Enterprise",
        TasksPerMonth:     -1,
        TokensPerMonth:    -1,
        MaxConcurrent:     50,
        MaxTaskTimeout:    120,
        MaxPipelineAgents: 20,
        ToolsEnabled:      true,
        ReviewEnabled:     true,
        SessionsEnabled:   true,
        PriceUSD:          0, // custom pricing
    },
}
```

### Usage Tracking

```go
type Usage struct {
    UserID        string    `json:"user_id"`
    Period        string    `json:"period"`        // "2025-02"
    TasksCreated  int       `json:"tasks_created"`
    TasksCompleted int      `json:"tasks_completed"`
    TokensInput   int64     `json:"tokens_input"`
    TokensOutput  int64     `json:"tokens_output"`
    ComputeMinutes float64  `json:"compute_minutes"`
    APICallCount  int       `json:"api_call_count"`
}
```

## Redis klíče

| Klíč | Typ | Popis | TTL |
|------|-----|-------|-----|
| `user:{id}` | Hash | User data | No TTL |
| `user:{id}:keys` | Set | API key IDs | No TTL |
| `apikey:{hash}` | Hash | API key data (lookup by hash) | No TTL |
| `user:{id}:usage:{period}` | Hash | Monthly usage counters | 90d |
| `user:{id}:usage:{period}:log` | List | Detailed usage log | 30d |
| `user:email:{email}` | String | Email → user ID mapping | No TTL |

## API Endpointy

### Auth (veřejné)

```
POST /api/v1/auth/register    — Registrace nového uživatele
POST /api/v1/auth/login       — Login (vrátí API key? nebo session?)
```

### User Management (autentizované)

```
GET    /api/v1/me                — Profil aktuálního uživatele
PUT    /api/v1/me                — Update profilu
GET    /api/v1/me/usage          — Aktuální usage
GET    /api/v1/me/usage/{period} — Usage za období
```

### API Key Management

```
POST   /api/v1/me/keys           — Vytvořit nový API key
GET    /api/v1/me/keys           — Seznam klíčů
DELETE /api/v1/me/keys/{id}      — Smazat klíč
PUT    /api/v1/me/keys/{id}      — Update (name, scopes, expiry)
```

### Admin

```
GET    /api/v1/admin/users         — Seznam uživatelů
GET    /api/v1/admin/users/{id}    — Detail uživatele
PUT    /api/v1/admin/users/{id}    — Update (plan, status)
GET    /api/v1/admin/usage         — Agregovaný usage
```

## Tasky

### 14.1 — Datový model
- [ ] Vytvořit `internal/user/model.go` — User, APIKey, Plan, Usage
- [ ] Definovat subscription plány
- [ ] API key formát: `cf_{plan}_{user_short}_{random}`

### 14.2 — User Service
- [ ] Vytvořit `internal/user/service.go`
- [ ] CRUD: Create/Get/Update/Delete user
- [ ] API key: Create/List/Delete/Rotate
- [ ] API key lookup: hash → user (fast)
- [ ] Unit testy

### 14.3 — Auth Middleware Refaktor
- [ ] Rozšířit `middleware/auth.go`:
  - Parse API key → lookup user
  - Check user status (active?)
  - Check plan limits
  - Set user context na request
- [ ] Backward compatible: starý single token stále funguje (admin mode)
- [ ] API key scopes enforcement

### 14.4 — Usage Tracker
- [ ] Vytvořit `internal/usage/tracker.go`
- [ ] Middleware: počítej API calls
- [ ] Post-task hook: trackuj tokeny, compute time
- [ ] Monthly reset (Redis expiry)
- [ ] Usage limit enforcement (403 když překročen)

### 14.5 — HTTP Handlers
- [ ] Vytvořit `internal/server/handlers/auth.go`
- [ ] Vytvořit `internal/server/handlers/users.go`
- [ ] Registration, login, profile, usage, key management
- [ ] Admin endpoints

### 14.6 — Plan Enforcement
- [ ] Check limity při task creation:
  - Tasks/month limit
  - Concurrent tasks limit
  - Task timeout limit
  - Pipeline agents limit
  - Feature gates (tools, review, sessions)
- [ ] Srozumitelné 403/429 responses s upgrade hints

### 14.7 — Task Ownership
- [ ] Přidat `user_id` na Task model
- [ ] Filtrovat tasky per user (GET /tasks vrátí jen moje tasky)
- [ ] Admin: vidí všechny tasky

### 14.8 — Integration testy
- [ ] Test: registration → API key → create task → usage tracked
- [ ] Test: free plan limits → 429 po překročení
- [ ] Test: API key scopes (read-only key nemůže vytvořit task)
- [ ] Test: backward compatibility (single admin token)

## Migrace

Protože nemáme databázi (jen Redis), migrace = "přidej nová data":

1. Existující API token se stane "admin key"
2. Existující tasky dostanou `user_id: "admin"`
3. Nové tasky vyžadují user API key
4. Konfigurační volba: `auth.mode: "single"` vs `"multi"`

## Testovací strategie

### User model testy (`internal/user/model_test.go`)

- [ ] `TestUser_JSON_MarshalUnmarshal` — round-trip serializace
- [ ] `TestAPIKey_JSON_HidesSecret` — Key a KeyHash se neserializují (`json:"-"`)
- [ ] `TestAPIKey_Prefix_Format` — prefix formát: `cf_{plan}_{short_id}_`
- [ ] `TestPlan_AllPlans_Valid` — každý plan má kladné limity (nebo -1)
- [ ] `TestPlan_FreeHasLimits` — free plan má omezení
- [ ] `TestPlan_EnterpriseUnlimited` — enterprise má -1 pro unlimited pole

### User Service testy (`internal/user/service_test.go` — miniredis)

- [ ] `TestUserService_Create` — vytvoř usera, ověř v Redis hash
- [ ] `TestUserService_Create_DuplicateEmail` — error pro duplicitní email
- [ ] `TestUserService_Get` — get existujícího usera
- [ ] `TestUserService_Get_NotFound` — error pro neexistující
- [ ] `TestUserService_Update` — update pole
- [ ] `TestUserService_Delete` — smazání + cleanup
- [ ] `TestUserService_CreateAPIKey` — generuj key, ulož hash, vrať key jednou
- [ ] `TestUserService_CreateAPIKey_HashLookup` — hash → user lookup funguje
- [ ] `TestUserService_ListAPIKeys` — list klíčů (bez secret)
- [ ] `TestUserService_DeleteAPIKey` — smazání klíče
- [ ] `TestUserService_LookupByHash` — SHA256(key) → najdi usera

```go
func TestUserService_CreateAPIKey(t *testing.T) {
    s := miniredis.RunT(t)
    rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
    defer rdb.Close()

    svc := NewUserService(rdb)
    user := User{ID: "u1", Email: "test@test.com", Plan: PlanFree}
    svc.Create(context.Background(), user)

    key, err := svc.CreateAPIKey(context.Background(), "u1", "My Key", []string{"tasks:write"})
    if err != nil { t.Fatalf("CreateAPIKey: %v", err) }
    if key.Key == "" { t.Error("Key is empty") }
    if key.Prefix == "" { t.Error("Prefix is empty") }
    // Key je vrácen jen jednou, poté nedostupný
}
```

### Auth Middleware testy (`internal/server/middleware/auth_test.go` — rozšíření)

- [ ] `TestAuth_ValidAPIKey` — valid API key → 200, user context nastavený
- [ ] `TestAuth_InvalidAPIKey` — neplatný key → 401
- [ ] `TestAuth_ExpiredAPIKey` — expirovaný key → 401
- [ ] `TestAuth_SuspendedUser` — suspended user → 403
- [ ] `TestAuth_InsufficientScope` — key bez write scope na POST → 403
- [ ] `TestAuth_LegacySingleToken` — starý admin token → backward compatible
- [ ] `TestAuth_MissingHeader` — chybí Authorization → 401
- [ ] `TestAuth_WrongFormat` — "Basic" místo "Bearer" → 401

### Usage Tracker testy (`internal/usage/tracker_test.go` — miniredis)

- [ ] `TestTracker_IncrementTaskCount` — HINCRBY user:{id}:usage:{period} tasks_created
- [ ] `TestTracker_IncrementTokens` — HINCRBY tokens_input, tokens_output
- [ ] `TestTracker_GetUsage` — čtení aktuálního usage
- [ ] `TestTracker_MonthlyReset` — nový měsíc → nový period key
- [ ] `TestTracker_TTL` — usage klíč má 90d TTL

### Plan Enforcement testy

- [ ] `TestPlanEnforcement_TaskLimit_Free` — 51. task → 429
- [ ] `TestPlanEnforcement_TaskLimit_Pro` — 501. task → 429
- [ ] `TestPlanEnforcement_Unlimited` — enterprise → nikdy 429
- [ ] `TestPlanEnforcement_ConcurrentLimit` — 2. concurrent task na free → 429
- [ ] `TestPlanEnforcement_FeatureGate_Tools` — free + tools → 403
- [ ] `TestPlanEnforcement_FeatureGate_Review` — free + review → 403
- [ ] `TestPlanEnforcement_UpgradeHint` — 429 response obsahuje upgrade info

### Handler testy (`internal/server/handlers/auth_test.go`)

- [ ] `TestAuthHandler_Register_Success` — POST /auth/register → 201 + API key
- [ ] `TestAuthHandler_Register_DuplicateEmail` — → 409
- [ ] `TestAuthHandler_Register_InvalidEmail` — → 400
- [ ] `TestAuthHandler_Me` — GET /me → 200 + user profil
- [ ] `TestAuthHandler_Usage` — GET /me/usage → 200 + usage data
- [ ] `TestAuthHandler_CreateKey` — POST /me/keys → 201 + key (plaintext jednou)
- [ ] `TestAuthHandler_ListKeys` — GET /me/keys → 200 + keys (bez secrets)
- [ ] `TestAuthHandler_DeleteKey` — DELETE /me/keys/{id} → 204

### Security testy

- [ ] `TestAPIKey_NeverInLogs` — API key se nikdy neloguje
- [ ] `TestAPIKey_NeverInResponse` — API key hash se nevrací v API
- [ ] `TestAPIKey_TimingAttack` — constant-time comparison pro hash

### Integration testy (`//go:build integration`)

- [ ] `TestIntegration_FullLifecycle` — register → key → task → usage tracked
- [ ] `TestIntegration_PlanLimits` — free plan → limit hit → 429
- [ ] `TestIntegration_ScopeEnforcement` — read-only key → POST /tasks → 403
- [ ] `TestIntegration_BackwardCompat` — legacy admin token funguje

## Linter checklist

- [ ] `crypto/rand` pro API key generaci (ne `math/rand`)
- [ ] `crypto/subtle.ConstantTimeCompare` pro key comparison
- [ ] API keys nikdy v logách
- [ ] Passwords/keys nikdy v JSON response
- [ ] `task fmt` + `task lint` MUSÍ projít

## Otevřené otázky

1. **Auth provider** — vlastní registrace vs OAuth (GitHub, Google)?
2. **Billing** — Stripe integrace? Nebo jen tracking bez plateb?
3. **Team management** — organizace, sdílené API klíče, role (admin/member)?
4. **Data isolation** — per-user Redis prefix? Nebo jen `user_id` na záznamech?
5. **PostgreSQL** — je Redis dostatečný pro user management, nebo přidat DB?
6. **ScopeBot integrace** — ScopeBot bude mít vlastní user management. Shared users?
