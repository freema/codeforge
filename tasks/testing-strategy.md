# CodeForge — Testing Strategy

> **Test repo:** https://github.com/freema/openclaw-mcp
> TypeScript MCP server, 1 open issue (#2: 405 Method Not Allowed), no open PRs.

---

## Test Levels Overview

| Level | Kde běží | Redis | Claude Code | Cíl |
|-------|----------|-------|-------------|-----|
| **Unit** | CI (každý PR) | Ne | Ne | Logika, parsing, state machine |
| **Integration** | CI (Docker Compose) | Reálný | Mock CLI | HTTP handlers, Redis queue, Pub/Sub, webhooky |
| **E2E** | Staging / manuálně | Reálný | Reálný | Celý flow proti freema/openclaw-mcp |
| **Smoke** | Po deployi | Reálný | Reálný | Health check + 1 jednoduchý task |

---

## Level 1: Unit Tests

> Běží v CI na každý push/PR. Žádné dependencies. Rychlé (< 30s).

### Co testovat

| Package | Test |
|---------|------|
| `internal/task` | State machine: všechny validní přechody projdou, invalidní vrátí error |
| `internal/task` | Task struct serializace → Redis hash → zpět (round-trip) |
| `internal/task` | Sensitive fields (`AccessToken`, `AIApiKey`) nikdy v JSON výstupu |
| `internal/config` | Výchozí hodnoty, YAML loading, env var override |
| `internal/config` | Validace: chybějící required fields vrátí error |
| `internal/git` | `createAskPassScript()`: vytvoří GIT_ASKPASS helper s tokenem |
| `internal/git` | `sanitizeError()`: token se neobjevuje v error message |
| `internal/git` | Provider detection: github.com → GitHub, gitlab.com → GitLab, custom domain |
| `internal/git` | `CalculateChanges()`: parsování git status/diff output |
| `internal/webhook` | HMAC-SHA256 signature generování a verifikace |
| `internal/webhook` | Exponential backoff delay kalkulace |
| `internal/server/middleware` | Bearer auth: validní token projde, invalidní 401 |
| `internal/server/middleware` | Constant-time comparison (ověřit subtle.ConstantTimeCompare) |
| `internal/cli` | Prompt analyzer fallback: vrátí generické jméno při chybě |
| `internal/worker/stream` | StreamEvent JSON serialization format |

### Příklad

```go
func TestStateTransition_ValidPath(t *testing.T) {
    task := &Task{Status: StatusPending}
    assert.NoError(t, task.Transition(StatusCloning))
    assert.NoError(t, task.Transition(StatusRunning))
    assert.NoError(t, task.Transition(StatusCompleted))
    assert.Equal(t, StatusCompleted, task.Status)
}

func TestStateTransition_InvalidPath(t *testing.T) {
    task := &Task{Status: StatusPending}
    assert.Error(t, task.Transition(StatusCompleted)) // skip cloning → error
}

func TestTokenNeverInJSON(t *testing.T) {
    task := &Task{AccessToken: "ghp_secret123", Prompt: "test"}
    data, _ := json.Marshal(task)
    assert.NotContains(t, string(data), "ghp_secret123")
}
```

---

## Level 2: Integration Tests

> Běží v CI s Docker Compose (CodeForge + Redis). Používá **Mock CLI** místo reálného Claude Code.
> Cíl: ověřit že všechny komponenty fungují dohromady.

### Mock CLI Binary

Klíčový komponent — falešný `claude` binary pro CI:

```
tests/
├── mockcli/
│   └── main.go          # Mock Claude Code CLI
├── integration/
│   ├── task_test.go      # Task CRUD tests
│   ├── worker_test.go    # Worker pool tests
│   ├── stream_test.go    # Streaming tests
│   ├── webhook_test.go   # Webhook delivery tests
│   └── redis_input_test.go # Redis message input tests
├── fixtures/
│   └── mock_repo/        # Fake git repo for clone tests
└── docker-compose.test.yaml
```

#### Mock CLI chování

```go
// tests/mockcli/main.go
// Kompiluje se do `mock-claude` binary
// Chová se jako claude CLI ale bez reálného AI

func main() {
    prompt := flag.String("p", "", "prompt")
    outputFormat := flag.String("output-format", "text", "output format")
    flag.Parse()

    if *outputFormat == "stream-json" {
        // Emituje fake stream-json events na stdout
        events := []string{
            `{"type":"system","subtype":"init","session_id":"test-123"}`,
            `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"README.md"}}]}}`,
            `{"type":"assistant","message":{"content":[{"type":"text","text":"Here is my analysis..."}]}}`,
            `{"type":"result","result":"Analysis complete. This is a TypeScript MCP server...","session_id":"test-123"}`,
        }
        for _, e := range events {
            fmt.Println(e)
            time.Sleep(100 * time.Millisecond) // simuluje streaming delay
        }
    } else {
        fmt.Println("Mock result for prompt:", *prompt)
    }
}
```

#### Mock CLI konfigurace přes env vars

```bash
MOCK_CLI_DELAY=500ms          # zpoždění mezi events
MOCK_CLI_EXIT_CODE=0          # 0=success, 1=failure
MOCK_CLI_MODIFY_FILES=true    # simuluje změny souborů (touch/edit files in workdir)
MOCK_CLI_RESULT_FILE=fixture.txt  # custom output
```

### Integration Test Cases

| # | Test | Popis | Ověřuje |
|---|------|-------|---------|
| I-1 | **Task lifecycle via HTTP** | POST /tasks → GET /tasks/:id (pending) → čekej na completed → GET (result) | HTTP handlers, Redis state, worker processing |
| I-2 | **Task lifecycle via Redis** | RPUSH input:tasks → čekej na input:result:{corr_id} → GET /tasks/:id | Redis input listener, correlation_id |
| I-3 | **Streaming events** | Subscribe na task:{id}:stream, submit task, ověř sekvenci events | Event categories (system, git, cli, stream, result) |
| I-4 | **Completion signal** | Subscribe na task:{id}:done, submit task, ověř one-shot done event | Redis done channel, changes_summary v done event |
| I-5 | **Stream history** | Submit task, po completed přečti task:{id}:history list | History persistence, TTL nastavení |
| I-6 | **Webhook delivery** | Start mock HTTP server, submit task s callback_url, ověř POST přijde | HMAC signature, payload format, headers |
| I-7 | **Webhook retry** | Mock server vrací 500 první 2x, pak 200 | Exponential backoff, retry count |
| I-8 | **Auth rejection** | POST /tasks bez Bearer tokenu | 401 response |
| I-9 | **Auth valid** | POST /tasks s platným tokenem | 201 response |
| I-10 | **Invalid payload** | POST /tasks s chybějícím repo_url | 400 s descriptive error |
| I-11 | **Task not found** | GET /tasks/nonexistent-id | 404 response |
| I-12 | **Worker concurrency** | Submit 5 tasků najednou, workers=3 | Max 3 běží paralelně, další čekají ve frontě |
| I-13 | **Graceful shutdown** | Submit task, pošli SIGTERM uprostřed | Task doběhne, pak server shutdownne |
| I-14 | **Timeout** | Submit task, MOCK_CLI_DELAY=10s, timeout_seconds=2 | Task FAILED, error "timed out", process killed |
| I-15 | **Cancel** | Submit task, hned POST /cancel | Task FAILED, error "cancelled by user" |
| I-16 | **Health endpoint** | GET /health | 200, redis connected, worker count |
| I-17 | **Changes summary** | Mock CLI s MOCK_CLI_MODIFY_FILES=true | changes_summary správně počítá modified/created/deleted |
| I-18 | **Git clone** | Clone z mock_repo fixture (bare git repo) | Clone funguje, workspace vytvořen |

### Docker Compose pro testy

```yaml
# tests/docker-compose.test.yaml
services:
  codeforge:
    build:
      context: ../
      dockerfile: deployments/Dockerfile
      args:
        CLI_BINARY: mock-claude   # use mock instead of real claude
    environment:
      CODEFORGE_REDIS_URL: redis://redis:6379
      CODEFORGE_AUTH_TOKEN: test-token
      CODEFORGE_HMAC_SECRET: test-secret
      CODEFORGE_WORKERS_CONCURRENCY: 3
      CODEFORGE_TASKS_DEFAULT_TIMEOUT: 30
    depends_on: [redis]

  redis:
    image: redis:7-alpine

  # Mock callback server for webhook tests
  callback:
    build:
      context: ./
      dockerfile: Dockerfile.callback
    ports: ["9090:9090"]
```

---

## Level 3: E2E Tests (reálný Claude Code)

> Běží manuálně nebo ve staging prostředí. Používá reálný Claude Code a reálné GitHub API.
> **Target repo:** `freema/openclaw-mcp`
> **Cena:** ~$0.10-2.00 za test run (API credits)

### Prerequisites

- `ANTHROPIC_API_KEY` nastavený
- `GITHUB_TOKEN` s write přístupem k `freema/openclaw-mcp`
- Reálný Redis instance
- CodeForge běží (Docker nebo lokálně)

### E2E Test Cases

| # | Test | Prompt | Ověřuje | Cleanup |
|---|------|--------|---------|---------|
| **E-1** | Knowledge doc (read-only) | "Generate a comprehensive knowledge document about this TypeScript MCP server project. Cover: architecture, key components, dependencies, API surface, and configuration options." | Result je raw text, changes_summary.files_modified = 0, streaming events přišly | Nic (read-only) |
| **E-2** | README update | "Update the README.md: improve the installation section, add a 'Quick Start' section with a code example showing basic usage." | changes_summary.files_modified >= 1, result popisuje co se změnilo | git checkout -- README.md (reset workspace) |
| **E-3** | PR creation | Po E-2: POST /tasks/:id/create-pr {"title":"Test: improve README","target_branch":"main"} | PR vytvořen na GitHub, pr_url vrácena, branch codeforge/* existuje | gh pr close {number} && git push origin --delete {branch} |
| **E-4** | Issue analysis | "Read issue #2 (405 Method Not Allowed) and analyze the root cause. Suggest a fix with specific code changes." | Result referencuje issue #2, navrhuje konkrétní fix | Nic (read-only, pokud nemodifikuje kód) |
| **E-5** | Follow-up instruction | Po E-2: POST /tasks/:id/instruct {"prompt":"Also add a Contributing section to README"} | Workspace reused (no re-clone), new changes, iteration=2 | Reset workspace |
| **E-6** | Redis input | Submit via RPUSH input:tasks (ne HTTP) | Task vytvořen, zpracován, result dostupný | Nic |
| **E-7** | Timeout handling | prompt="Analyze every file in detail", timeout_seconds=10 | Task FAILED, error "timed out" | Nic |
| **E-8** | Multiple deliveries | Task s callback_url + Redis subscribe + HTTP poll | Všechny 3 kanály doručí result | Nic |

### E2E Test Runner Script

```bash
#!/bin/bash
# tests/e2e/run.sh
# Spouští E2E testy postupně, kontroluje výsledky

set -e

CODEFORGE_URL="${CODEFORGE_URL:-http://localhost:8080}"
AUTH_TOKEN="${CODEFORGE_AUTH_TOKEN:-test-token}"
TEST_REPO="https://github.com/freema/openclaw-mcp"

echo "=== E-1: Knowledge Document (read-only) ==="
TASK_ID=$(curl -s -X POST "$CODEFORGE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"repo_url\": \"$TEST_REPO\",
    \"prompt\": \"Generate a knowledge document about this project.\",
    \"config\": {\"timeout_seconds\": 300, \"max_turns\": 5}
  }" | jq -r '.id')

echo "Task ID: $TASK_ID"
echo "Waiting for completion..."

# Poll until completed
while true; do
  STATUS=$(curl -s "$CODEFORGE_URL/api/v1/tasks/$TASK_ID" \
    -H "Authorization: Bearer $AUTH_TOKEN" | jq -r '.status')
  echo "  Status: $STATUS"
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then break; fi
  sleep 5
done

# Verify result
RESULT=$(curl -s "$CODEFORGE_URL/api/v1/tasks/$TASK_ID" \
  -H "Authorization: Bearer $AUTH_TOKEN")
echo "$RESULT" | jq '{status, changes_summary, usage}'

# Assert: read-only = no changes
FILES_MODIFIED=$(echo "$RESULT" | jq '.changes_summary.files_modified // 0')
if [ "$FILES_MODIFIED" -eq 0 ]; then
  echo "✅ E-1 PASSED: No files modified (read-only task)"
else
  echo "❌ E-1 FAILED: Expected 0 modified files, got $FILES_MODIFIED"
fi

echo ""
echo "=== E-2: README Update ==="
# ... similar pattern for each test
```

---

## Level 4: Smoke Tests (po deployi)

> Minimální test po každém deployi do staging/production.
> Cíl: ověřit že server běží, Redis je připojený, worker zpracuje task.

```bash
# 1. Health check
curl -f http://codeforge:8080/health | jq .
# Expected: {"status":"ok","redis":"connected","active_workers":3}

# 2. Submit + complete a minimal task
TASK_ID=$(curl -s -X POST http://codeforge:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"repo_url":"https://github.com/freema/openclaw-mcp","prompt":"What is this project?","config":{"max_turns":1,"timeout_seconds":60}}' \
  | jq -r '.id')

# 3. Wait for completion (max 90s)
# 4. Verify status = "completed" and result is non-empty
```

---

## Testovací data a fixtures

### Mock Git Repo (pro integration testy)

```bash
# tests/fixtures/create_mock_repo.sh
# Vytvoří bare git repo pro clone testy bez přístupu na internet

mkdir -p /tmp/mock-repo && cd /tmp/mock-repo
git init --bare

# Clone, přidej soubory, pushni
git clone /tmp/mock-repo /tmp/mock-repo-work
cd /tmp/mock-repo-work
echo "# Mock Project" > README.md
echo "package main" > main.go
git add -A && git commit -m "init" && git push origin main
```

### Webhook Callback Mock Server

```go
// tests/callback/main.go
// Jednoduchý HTTP server co loguje příchozí webhooky

func main() {
    received := make(chan WebhookPayload, 100)

    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        // Verify HMAC signature
        sig := r.Header.Get("X-Signature-256")
        body, _ := io.ReadAll(r.Body)
        if !verifySignature(body, sig, "test-secret") {
            http.Error(w, "bad signature", 401)
            return
        }
        var payload WebhookPayload
        json.Unmarshal(body, &payload)
        received <- payload
        w.WriteHeader(200)
    })

    // GET /received — vrátí všechny přijaté webhooky (pro test assertions)
    http.HandleFunc("/received", func(w http.ResponseWriter, r *http.Request) {
        // drain channel, return as JSON array
    })

    http.ListenAndServe(":9090", nil)
}
```

---

## CI Pipeline

```yaml
# .github/workflows/ci.yaml
jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: task test        # go test -race -cover ./... (inside Docker)

  integration:
    runs-on: ubuntu-latest
    needs: unit
    services:
      redis:
        image: redis:7-alpine
        ports: ['6379:6379']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: task build:mock-cli      # go build -o bin/mock-claude ./tests/mockcli
      - run: task test:integration   # go test -tags integration ./tests/integration/...
        env:
          CODEFORGE_REDIS_URL: redis://localhost:6379
          CODEFORGE_CLI_PATH: ./bin/mock-claude

  # E2E tests run manually or on release tags only
  e2e:
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/v')
    steps:
      - run: ./tests/e2e/run.sh
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

---

## Shrnutí

| Metrika | Cíl |
|---------|-----|
| Unit test coverage | > 80% pro core packages (task, git, webhook, middleware) |
| Integration tests | Všech 18 test cases zelených v CI |
| E2E tests | 8 test cases, běží manuálně nebo na release |
| CI čas | Unit < 30s, Integration < 2min |
| E2E čas | < 10min (včetně Claude Code execution) |
| E2E cena | < $2 za run |
