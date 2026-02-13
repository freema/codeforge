# Phase 1 — Core Task Runner (v0.2.0)

> Minimum viable task execution: submit, clone, run, stream, report.
> Default flow is **read-only** — no branches, no commits, no PRs.

---

## Task 1.1: Task Model & State Machine

**Priority:** P0
**Files:** `internal/task/model.go`, `internal/task/state.go`

### Description

Define the Task struct with all fields (including fields needed for later phases), task status enum, and a state machine that enforces valid transitions. Persist task state in Redis hashes.

### Acceptance Criteria

- [ ] Task struct with all fields (see below — designed for full lifecycle up front)
- [ ] Status enum: PENDING, CLONING, RUNNING, COMPLETED, FAILED, AWAITING_INSTRUCTION, CREATING_PR, PR_CREATED
- [ ] State machine validates transitions (see valid transitions below)
- [ ] Task serializes to/from Redis hash (`task:{id}:state`)
- [ ] Task ID is UUID v4
- [ ] Sensitive fields (`AccessToken`, `AIApiKey`) never returned in API responses (`json:"-"`)
- [ ] Sensitive fields stored in Redis **encrypted** (AES-256-GCM via CryptoService from Task 4.2)
- [ ] Redis hash fields: `encrypted_access_token`, `encrypted_ai_api_key` (plaintext NEVER in Redis)
- [ ] On task load: decrypt in memory for execution, never expose via API
- [ ] Unit tests for all valid and invalid state transitions

### Implementation Notes

```go
type TaskStatus string

const (
    StatusPending             TaskStatus = "pending"
    StatusCloning             TaskStatus = "cloning"
    StatusRunning             TaskStatus = "running"
    StatusCompleted           TaskStatus = "completed"
    StatusFailed              TaskStatus = "failed"
    StatusAwaitingInstruction TaskStatus = "awaiting_instruction"
    StatusCreatingPR          TaskStatus = "creating_pr"
    StatusPRCreated           TaskStatus = "pr_created"
)

// Valid transitions — default flow is simple: PENDING → CLONING → RUNNING → COMPLETED
// PR states are optional, triggered by explicit signal
var validTransitions = map[TaskStatus][]TaskStatus{
    StatusPending:             {StatusCloning, StatusFailed},
    StatusCloning:             {StatusRunning, StatusFailed},
    StatusRunning:             {StatusCompleted, StatusFailed},
    StatusCompleted:           {StatusAwaitingInstruction, StatusCreatingPR},
    StatusFailed:              {},  // terminal for this iteration
    StatusAwaitingInstruction: {StatusRunning, StatusFailed},
    StatusCreatingPR:          {StatusPRCreated, StatusFailed},
    StatusPRCreated:           {StatusAwaitingInstruction, StatusCompleted},
}

// Full Task struct — fields for all phases defined up front
type Task struct {
    ID          string     `json:"id"`
    Status      TaskStatus `json:"status"`
    RepoURL     string     `json:"repo_url"`
    ProviderKey string     `json:"provider_key,omitempty"`
    AccessToken string     `json:"-"` // NEVER in API responses; stored ENCRYPTED in Redis
    Prompt      string     `json:"prompt"`
    CallbackURL string     `json:"callback_url,omitempty"`
    Config      *TaskConfig `json:"config,omitempty"`

    // Result fields
    Result         string          `json:"result,omitempty"`
    Error          string          `json:"error,omitempty"`
    ChangesSummary *ChangesSummary `json:"changes_summary,omitempty"`
    Usage          *UsageInfo      `json:"usage,omitempty"`

    // Iteration tracking (Phase 3)
    Iteration     int         `json:"iteration"`
    CurrentPrompt string      `json:"current_prompt,omitempty"` // latest prompt (may differ from original)
    Iterations    []Iteration `json:"iterations,omitempty"`     // full history for context building

    // Git integration (Phase 2) — populated only when PR is requested
    Branch   string `json:"branch,omitempty"`
    PRNumber int    `json:"pr_number,omitempty"`
    PRURL    string `json:"pr_url,omitempty"`

    // Observability (Phase 6)
    TraceID string `json:"trace_id,omitempty"` // OpenTelemetry trace ID, stored in Redis state

    // Timestamps
    CreatedAt  time.Time  `json:"created_at"`
    StartedAt  *time.Time `json:"started_at,omitempty"`
    FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type ChangesSummary struct {
    FilesModified int    `json:"files_modified"`
    FilesCreated  int    `json:"files_created"`
    FilesDeleted  int    `json:"files_deleted"`
    DiffStats     string `json:"diff_stats"` // e.g., "+142 -38"
}

type UsageInfo struct {
    InputTokens    int `json:"input_tokens"`
    OutputTokens   int `json:"output_tokens"`
    DurationSeconds int `json:"duration_seconds"`
}

type TaskConfig struct {
    TimeoutSeconds int         `json:"timeout_seconds,omitempty"`
    CLI            string      `json:"cli,omitempty"`           // default: "claude-code"
    AIModel        string      `json:"ai_model,omitempty"`
    AIApiKey       string      `json:"-"`                       // NEVER in responses; stored ENCRYPTED in Redis
    MaxTurns       int         `json:"max_turns,omitempty"`
    TargetBranch   string      `json:"target_branch,omitempty"` // for PR creation
    MaxBudgetUSD   float64     `json:"max_budget_usd,omitempty"` // cost control via --max-budget-usd
    MCPServers     []MCPServer `json:"mcp_servers,omitempty"`
}
```

### Dependencies

- Phase 0 complete (Task 0.6 Redis client)
- Task 4.2 (CryptoService for encrypting sensitive fields in Redis) — **pull forward to Phase 1** as shared infrastructure

---

## Task 1.2: POST /tasks Endpoint

**Priority:** P0
**Files:** `internal/server/handlers/tasks.go`, `internal/task/service.go`

### Description

Accept task submission via HTTP POST, validate payload, create task in Redis, enqueue to worker queue, return task ID.

### Acceptance Criteria

- [ ] `POST /api/v1/tasks` accepts JSON payload (see project-plan.md §2.4)
- [ ] Validates required fields using `github.com/go-playground/validator/v10`: `repo_url`, `prompt`
- [ ] Token resolution: `access_token` (inline) → `provider_key` (registry) → env var fallback
- [ ] Generates UUID v4 task ID
- [ ] Creates task in Redis with status=PENDING
- [ ] Enqueues task ID to `queue:tasks` via **RPUSH** (FIFO with BLPOP)
- [ ] Returns 201 with `{"id": "...", "status": "pending", "created_at": "..."}`
- [ ] Returns 400 for invalid input with descriptive errors
- [ ] Returns 401 for missing/invalid Bearer token
- [ ] Prompt size limit: 100KB max (configurable)

### Implementation Notes

```go
// Validation: github.com/go-playground/validator/v10
// Usage: validate := validator.New(); err := validate.Struct(req)
type CreateTaskRequest struct {
    RepoURL     string      `json:"repo_url" validate:"required,url"`
    ProviderKey string      `json:"provider_key,omitempty"`
    AccessToken string      `json:"access_token,omitempty"`
    Prompt      string      `json:"prompt" validate:"required,max=102400"`
    CallbackURL string      `json:"callback_url,omitempty" validate:"omitempty,url"`
    Config      *TaskConfig `json:"config,omitempty"`
}
// Note: correlation_id is Redis-input-only (Task 1.5), NOT part of HTTP request schema.

func (s *TaskService) Create(ctx context.Context, req CreateTaskRequest) (*Task, error) {
    task := &Task{
        ID:        uuid.New().String(),
        Status:    StatusPending,
        RepoURL:   req.RepoURL,
        Prompt:    req.Prompt,
        Iteration: 1,
        CreatedAt: time.Now().UTC(),
    }
    s.redis.HSet(ctx, "task:"+task.ID+":state", ...)
    s.redis.RPush(ctx, s.cfg.QueueName, task.ID)  // RPUSH for FIFO
    return task, nil
}
```

### Dependencies

- Task 1.1 (Task model)
- Task 0.7 (HTTP server with routes)

---

## Task 1.3: GET /tasks/:id Endpoint

**Priority:** P0
**Files:** `internal/server/handlers/tasks.go`

### Description

Retrieve task status, result, and changes_summary from Redis by task ID.

### Acceptance Criteria

- [ ] `GET /api/v1/tasks/{taskID}` returns full task state
- [ ] Returns 200 with task JSON (status, result, changes_summary, usage, timestamps)
- [ ] Returns 404 if task ID doesn't exist
- [ ] Never exposes sensitive fields (access tokens, API keys)
- [ ] If task has result, include from `task:{id}:result`
- [ ] Include `changes_summary` if available (files modified/created/deleted)

### Implementation Notes

```go
taskID := chi.URLParam(r, "taskID")
fields, err := s.redis.HGetAll(ctx, "task:"+taskID+":state").Result()
if len(fields) == 0 {
    return ErrNotFound
}
// Also fetch result if completed
result, _ := s.redis.Get(ctx, "task:"+taskID+":result").Result()
```

### Dependencies

- Task 1.1 (Task model)
- Task 1.2 (tasks must exist)

---

## Task 1.4: Redis Queue Consumer (Worker Pool)

**Priority:** P0
**Files:** `internal/worker/pool.go`

### Description

Worker pool that consumes task IDs from Redis queue using **BLPOP** (paired with RPUSH for FIFO). Dispatches to executor goroutines. Configurable concurrency.

### Acceptance Criteria

- [ ] Pool starts N worker goroutines (N = config `workers.concurrency`)
- [ ] Each worker calls **BLPOP** on `queue:tasks` with 5s timeout (RPUSH+BLPOP = FIFO)
- [ ] On receiving task ID, loads task from Redis and passes to executor
- [ ] Detects iteration: if task has `iteration > 1`, skips clone step (workspace reuse)
- [ ] Respects context cancellation for graceful shutdown
- [ ] Workers drain cleanly: finish current task before stopping
- [ ] Logs worker start/stop and task pickup
- [ ] Tracks active worker count (for health endpoint)
- [ ] If task loading fails (deleted from Redis), log warning and skip

### Implementation Notes

```go
func (p *Pool) worker(ctx context.Context, id int) {
    defer p.wg.Done()
    for {
        // BLPOP blocks until item available or timeout
        result, err := p.redis.BLPop(ctx, 5*time.Second, p.queueName).Result()
        if err != nil {
            if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
                if ctx.Err() != nil { return } // shutting down
                continue
            }
            slog.Error("queue pop failed", "worker", id, "error", err)
            continue
        }
        taskID := result[1] // result[0] = key name
        p.execute(ctx, taskID)
    }
}
```

> **FIFO guarantee:** RPUSH adds to tail, BLPOP removes from head → first in, first out.

### Dependencies

- Task 0.6 (Redis client)
- Task 1.1 (Task model)

---

## Task 1.5: Redis Message Input

**Priority:** P0 *(upgraded from P1 — Redis is the primary input channel for ScopeBot)*
**Files:** `internal/task/listener.go`

### Description

Primary task submission for Redis-connected consumers (ScopeBot). External systems RPUSH a JSON task payload to a configurable Redis list. A listener goroutine validates the payload and creates the task.

### Acceptance Criteria

- [ ] Listens on configurable Redis key (default: `input:tasks`)
- [ ] BLPOP to consume incoming payloads (same FIFO pattern)
- [ ] Validates incoming JSON payload (same schema as HTTP POST)
- [ ] Creates task in Redis and enqueues to `queue:tasks`
- [ ] Returns task ID by setting Redis key `input:result:{correlation_id}` (so submitter can read it)
- [ ] Invalid payloads logged and discarded (not retried)
- [ ] Graceful shutdown support
- [ ] Publishes system event: `{"type":"system","event":"task_received","data":{"source":"redis"}}`

### Implementation Notes

```go
// ScopeBot submits task via Redis:
// RPUSH input:tasks '{"repo_url":"...","prompt":"...","correlation_id":"abc"}'
//
// CodeForge creates task and writes back:
// SET input:result:abc '{"task_id":"uuid","status":"pending"}' EX 300
```

This is the same BLPOP pattern as the worker pool, but instead of executing tasks, it creates them. The `correlation_id` lets the submitter know the task ID.

### Dependencies

- Task 1.1 (Task model)
- Task 1.2 (reuse task creation logic from service layer)

---

## Task 1.6: Git Clone Service

**Priority:** P0
**Files:** `internal/git/clone.go`

### Description

Clone a Git repository into a workspace directory using an access token. Support GitHub and GitLab HTTPS URLs. Shallow clone by default.

### Acceptance Criteria

- [ ] Clones repo via HTTPS with token auth using `GIT_ASKPASS` helper (NOT URL-embedded token)
- [ ] GitHub format: `GIT_ASKPASS` script echoes token (works with PAT and fine-grained tokens)
- [ ] GitLab format: same `GIT_ASKPASS` approach (works for SaaS and self-hosted)
- [ ] Token NEVER stored in `.git/config` — `GIT_ASKPASS` keeps it in memory only
- [ ] Shallow clone (`--depth 1`) by default, full clone via config flag
- [ ] Clone into specified workspace directory
- [ ] Checkout target branch if specified (default: repo default branch)
- [ ] Token resolved via resolver: inline → registry → env var (Task 4.3, env fallback for now)
- [ ] Token NEVER appears in logs (sanitize clone URLs in log output)
- [ ] Returns cloned directory path
- [ ] Timeout support via context
- [ ] Publishes stream events: `{"type":"git","event":"clone_started"}`, `{"type":"git","event":"clone_completed"}`

### Implementation Notes

```go
func (s *CloneService) Clone(ctx context.Context, opts CloneOptions) (string, error) {
    args := []string{"clone", "--depth", "1"}
    if opts.Branch != "" {
        args = append(args, "--branch", opts.Branch)
    }
    args = append(args, opts.RepoURL, opts.DestDir)

    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = opts.DestDir
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // Token via GIT_ASKPASS — never stored in .git/config
    askpass := createAskPassScript(opts.Token)
    defer os.Remove(askpass) // cleanup temp script
    cmd.Env = append(os.Environ(),
        "GIT_ASKPASS="+askpass,
        "GIT_TERMINAL_PROMPT=0",
    )

    if err := cmd.Run(); err != nil {
        return "", sanitizeError(err, opts.Token) // NEVER leak token
    }
    return opts.DestDir, nil
}

// createAskPassScript creates a temp script that echoes the token.
// Git calls this script for username (ignored) and password (returns token).
func createAskPassScript(token string) string {
    f, _ := os.CreateTemp("", "codeforge-askpass-*.sh")
    f.WriteString("#!/bin/sh\necho '" + shellEscape(token) + "'\n")
    f.Close()
    os.Chmod(f.Name(), 0700)
    return f.Name()
}
```

> **Security:** `GIT_ASKPASS` ensures the token is never written to `.git/config`.
> The clone URL stays clean (`https://github.com/org/repo.git`), so the token
> can't leak via `git remote -v` or workspace inspection. Same script is reused
> for push operations (Task 2.3).

### Dependencies

- Phase 0 complete (Redis for key lookup)
- Task 1.1 (Task model)

---

## Task 1.7: Claude Code Executor

**Priority:** P0
**Files:** `internal/cli/runner.go`, `internal/cli/claude.go`

### Description

Run Claude Code CLI against a cloned workspace using `--output-format stream-json` for full streaming visibility. Capture structured events, support timeouts. Define a `Runner` interface for pluggable CLI tools.

### Acceptance Criteria

- [ ] `Runner` interface: `Run(ctx, opts) (*RunResult, error)`
- [ ] Claude Code spawns: `claude -p "<prompt>" --output-format stream-json --verbose --permission-mode bypassPermissions`
- [ ] Claude Code CLI version pinned in Dockerfile (e.g., `@anthropic-ai/claude-code@1.x.x`) and documented in README
- [ ] CLI version also configurable via config: `cli.claude_code.version` (for upgrades without rebuild)
- [ ] Supports `--model` flag from task config (`config.ai_model`)
- [ ] Supports `--max-turns` flag from task config
- [ ] Supports `--max-budget-usd` for cost control (optional)
- [ ] Reads stdout line-by-line — each line is a JSON event from `stream-json`
- [ ] Passes each JSON event to streaming callback (for Redis Pub/Sub)
- [ ] Collects final result text from stream events
- [ ] Context-based timeout kills the process (process group kill)
- [ ] AI API key passed via `ANTHROPIC_API_KEY` environment variable
- [ ] Returns `RunResult` with output, exit code, duration, usage stats

### Implementation Notes

```go
type Runner interface {
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}

type RunOptions struct {
    Prompt    string
    WorkDir   string
    Model     string
    APIKey    string
    MaxTurns  int
    OnEvent   func(event json.RawMessage) // each stream-json line
}

type RunResult struct {
    Output     string
    ExitCode   int
    Duration   time.Duration
    InputTokens  int
    OutputTokens int
}

func (c *ClaudeRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    args := []string{
        "-p", opts.Prompt,
        "--output-format", "stream-json",
        "--verbose",
        "--permission-mode", "bypassPermissions",
    }
    if opts.Model != "" {
        args = append(args, "--model", opts.Model)
    }
    if opts.MaxTurns > 0 {
        args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
    }

    cmd := exec.CommandContext(ctx, c.binaryPath, args...)
    cmd.Dir = opts.WorkDir
    cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+opts.APIKey)
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // process group for clean kill

    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    cmd.Start()

    // Read stream-json: each line is a complete JSON object
    scanner := bufio.NewScanner(stdout)
    scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large events
    var lastResult string
    for scanner.Scan() {
        line := scanner.Bytes()
        if opts.OnEvent != nil {
            opts.OnEvent(json.RawMessage(line))
        }
        // Extract final result text from stream events
        lastResult = extractResultText(line)
    }

    err := cmd.Wait()
    return &RunResult{
        Output:   lastResult,
        ExitCode: cmd.ProcessState.ExitCode(),
        Duration: time.Since(startTime),
    }, err
}
```

### Dependencies

- Task 1.6 (workspace must be cloned first)

---

## Task 1.8: Redis Pub/Sub Streaming with Event Categories

**Priority:** P0
**Files:** `internal/worker/stream.go`, `internal/redis/pubsub.go`

### Description

Publish structured events to Redis Pub/Sub channel `task:{id}:stream`. Events are categorized into groups: system, git, cli, stream, result. Raw Claude Code `stream-json` output is forwarded as-is in the `stream` category.

### Acceptance Criteria

- [ ] All events published to `task:{id}:stream` as JSON
- [ ] Event format: `{"type":"<group>","event":"<name>","data":{...},"ts":"<ISO8601>"}`
- [ ] Event groups: `system`, `git`, `cli`, `stream`, `result`
- [ ] `stream` events contain raw Claude Code stream-json output (every line forwarded)
- [ ] `cli` events extracted from stream-json: detect `content_block_start` where `content_block.type == "tool_use"` → emit `cli.tool_call` with tool name and input
- [ ] Tool call detection: `content_block_start` with `content_block.type == "tool_use"` contains `name` (e.g., "Read", "Write", "Bash") and `id`
- [ ] Tool input accumulation: subsequent `content_block_delta` events with `delta.type == "input_json_delta"` contain partial JSON (concatenate until `content_block_stop`)
- [ ] Final result text: accumulate `content_block_delta` events where `delta.type == "text_delta"` → `.delta.text`
- [ ] Stream end: `message_stop` event signals completion; `message_delta` contains `stop_reason` and `usage` (token counts)
- [ ] Completion signal: separate `PUBLISH task:{id}:done` with final status + changes_summary
- [ ] Result stored in `SET task:{id}:result` (raw text)
- [ ] All events also persisted to `task:{id}:history` list (dual-write via pipeline)
- [ ] History list gets TTL matching workspace TTL
- [ ] **TTL policy for task data:**
  - `task:{id}:state` → TTL = `tasks.state_ttl` (default: 7 days) — set on completion/failure
  - `task:{id}:result` → TTL = `tasks.result_ttl` (default: 7 days) — set when result is stored
  - `task:{id}:history` → TTL = `tasks.workspace_ttl` (default: 24h) — ephemeral stream data
  - Running tasks have NO TTL (TTL set only on terminal states)
  - Config keys: `tasks.state_ttl`, `tasks.result_ttl` (seconds, added to Task 0.2)

### Implementation Notes

```go
type StreamEvent struct {
    Type  string          `json:"type"`  // system, git, cli, stream, result
    Event string          `json:"event"` // event name
    Data  json.RawMessage `json:"data"`  // event-specific payload
    TS    string          `json:"ts"`    // ISO 8601 timestamp
}

func (s *Streamer) Emit(ctx context.Context, taskID string, evt StreamEvent) error {
    evt.TS = time.Now().UTC().Format(time.RFC3339Nano)
    data, _ := json.Marshal(evt)
    msg := string(data)

    // Dual-write: Pub/Sub + History list (atomic pipeline)
    pipe := s.redis.Pipeline()
    pipe.Publish(ctx, "task:"+taskID+":stream", msg)
    pipe.RPush(ctx, "task:"+taskID+":history", msg)
    _, err := pipe.Exec(ctx)
    return err
}

// Completion signal — separate channel so consumers don't need to parse every event
func (s *Streamer) EmitDone(ctx context.Context, taskID string, status string, summary *ChangesSummary) error {
    data, _ := json.Marshal(map[string]interface{}{
        "task_id":         taskID,
        "status":          status,
        "changes_summary": summary,
    })
    pipe := s.redis.Pipeline()
    pipe.Publish(ctx, "task:"+taskID+":done", string(data))
    pipe.Expire(ctx, "task:"+taskID+":history", s.historyTTL)
    _, err := pipe.Exec(ctx)
    return err
}

// Called from executor for each Claude Code stream-json line:
func (s *Streamer) EmitCLIOutput(ctx context.Context, taskID string, rawEvent json.RawMessage) error {
    return s.Emit(ctx, taskID, StreamEvent{
        Type:  "stream",
        Event: "output",
        Data:  rawEvent,
    })
}
```

### Dependencies

- Task 0.6 (Redis Pub/Sub)
- Task 1.7 (CLI OnEvent callback)

---

## Task 1.9: Changes Summary Calculation

**Priority:** P0 *(new task — needed for consumer to decide on PR)*
**Files:** `internal/git/diff.go`

### Description

After CLI execution completes, calculate what changed in the workspace: files modified, created, deleted, and diff stats. This `changes_summary` is included in the task result so the consumer can decide whether to request a PR.

### Acceptance Criteria

- [ ] Runs `git diff --shortstat` and `git status --porcelain` in workspace after CLI completes
- [ ] Covers both staged and unstaged changes: `git diff --shortstat` (unstaged) + `git diff --cached --shortstat` (staged)
- [ ] For untracked files (new files by Claude): `git status --porcelain` detects `??` entries
- [ ] Parses output into `ChangesSummary` struct
- [ ] Handles case where nothing changed (all zeros)
- [ ] Stored as part of task state in Redis
- [ ] Included in GET /tasks/:id response
- [ ] Included in webhook callback payload
- [ ] Included in Redis `task:{id}:done` completion signal

### Implementation Notes

```go
func CalculateChanges(ctx context.Context, workDir string) (*ChangesSummary, error) {
    // git status --porcelain shows all changes
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    cmd.Dir = workDir
    out, _ := cmd.Output()

    var modified, created, deleted int
    for _, line := range strings.Split(string(out), "\n") {
        if len(line) < 3 { continue }
        switch line[0:2] {
        case " M", "MM": modified++
        case "??", " A", "A ": created++
        case " D", "D ": deleted++
        }
    }

    // git diff --shortstat for +/- summary (unstaged)
    cmd2 := exec.CommandContext(ctx, "git", "diff", "--shortstat")
    cmd2.Dir = workDir
    unstaged, _ := cmd2.Output()

    // git diff --cached --shortstat for staged changes
    cmd3 := exec.CommandContext(ctx, "git", "diff", "--cached", "--shortstat")
    cmd3.Dir = workDir
    staged, _ := cmd3.Output()

    // --shortstat output: "3 files changed, 142 insertions(+), 38 deletions(-)"
    // Parse and combine both outputs
    ins, del := parseShortStat(string(unstaged))
    ins2, del2 := parseShortStat(string(staged))
    diffStats := fmt.Sprintf("+%d -%d", ins+ins2, del+del2)

    return &ChangesSummary{
        FilesModified: modified,
        FilesCreated:  created,
        FilesDeleted:  deleted,
        DiffStats:     diffStats,
    }, nil
}
```

### Dependencies

- Task 1.7 (runs after CLI completes)
- Task 1.1 (ChangesSummary struct)

---

## Task 1.10: Task Timeout Enforcement

**Priority:** P0
**Files:** `internal/worker/executor.go`

### Description

Enforce per-task timeouts. If CLI execution exceeds timeout, kill the process and mark task as FAILED.

### Acceptance Criteria

- [ ] Timeout from task config, fallback to default from config
- [ ] Cannot exceed `max_timeout` from config
- [ ] Uses Go `context.WithTimeout` to propagate cancellation
- [ ] On timeout: kill process group (not just main process)
- [ ] Task status set to FAILED with error "task timed out after Xs"
- [ ] Stream publishes timeout event: `{"type":"system","event":"task_timeout"}`
- [ ] Webhook callback sent with failure status

### Implementation Notes

```go
timeout := task.Config.TimeoutSeconds
if timeout == 0 {
    timeout = s.cfg.Tasks.DefaultTimeout
}
if timeout > s.cfg.Tasks.MaxTimeout {
    timeout = s.cfg.Tasks.MaxTimeout
}

ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeout)*time.Second)
defer cancel()

// exec.CommandContext sends SIGKILL on context cancellation
// Process group kill via SysProcAttr set in Task 1.7
```

### Dependencies

- Task 1.7 (CLI executor)

---

## Task 1.11: Webhook Callback

**Priority:** P0
**Files:** `internal/webhook/sender.go`

### Description

Send task results to the callback URL via HTTP POST with HMAC-SHA256 signature. Includes changes_summary. Retry with exponential backoff.

### Acceptance Criteria

- [ ] POST to `callback_url` with JSON body containing task result + changes_summary
- [ ] Header `X-Signature-256: sha256=<hex>` with HMAC-SHA256 of body
- [ ] Header `X-CodeForge-Event: task.completed` or `task.failed`
- [ ] Header `X-Request-ID` from original request
- [ ] Retry with exponential backoff: 1s, 5s, 25s (configurable base + max retries)
- [ ] Skip if no `callback_url` in task
- [ ] Log webhook delivery status (success/failure/retry)
- [ ] Timeout per webhook request (10s)

### Implementation Notes

```go
type WebhookPayload struct {
    TaskID         string          `json:"task_id"`
    Status         string          `json:"status"`
    Result         string          `json:"result,omitempty"`
    Error          string          `json:"error,omitempty"`
    ChangesSummary *ChangesSummary `json:"changes_summary,omitempty"`
    Usage          *UsageInfo      `json:"usage,omitempty"`
    FinishedAt     time.Time       `json:"finished_at"`
}

func (s *Sender) Send(ctx context.Context, callbackURL string, payload WebhookPayload) error {
    body, _ := json.Marshal(payload)

    mac := hmac.New(sha256.New, []byte(s.secret))
    mac.Write(body)
    sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

    for attempt := 0; attempt <= s.maxRetries; attempt++ {
        req, _ := http.NewRequestWithContext(ctx, "POST", callbackURL, bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("X-Signature-256", sig)
        req.Header.Set("X-CodeForge-Event", "task."+payload.Status)

        resp, err := s.client.Do(req)
        if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
            return nil
        }
        // Exponential backoff: 1s, 5s, 25s
        delay := time.Duration(math.Pow(5, float64(attempt))) * time.Second
        time.Sleep(delay)
    }
    return fmt.Errorf("webhook delivery failed after %d attempts", s.maxRetries)
}
```

### Dependencies

- Task 0.2 (webhook config: secret, retry settings)

---

## Task 1.12: Bearer Token Auth Middleware

**Priority:** P0
**Files:** `internal/server/middleware/auth.go`

### Description

Middleware that validates `Authorization: Bearer <token>` header against configured token.

### Acceptance Criteria

- [ ] Rejects requests without Authorization header (401)
- [ ] Rejects requests with invalid token (401)
- [ ] Passes requests with valid Bearer token
- [ ] Constant-time comparison to prevent timing attacks (`crypto/subtle`)
- [ ] Applied to `/api/v1/*` routes only (not `/health`, `/ready`)
- [ ] Error response: `{"error": "unauthorized", "message": "..."}`

### Implementation Notes

```go
func BearerAuthMiddleware(expected string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte(`{"error":"unauthorized"}`))
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Dependencies

- Task 0.7 (HTTP server skeleton)

---

## Task 1.13: Graceful Shutdown

**Priority:** P0
**Files:** `cmd/codeforge/main.go`

### Description

Clean shutdown sequence: stop accepting new HTTP requests, drain worker pool, close Redis connection.

### Acceptance Criteria

- [ ] Listens for SIGINT and SIGTERM
- [ ] Stops HTTP server (with 30s drain timeout)
- [ ] Signals workers to stop accepting new tasks
- [ ] Waits for in-flight tasks to complete (with timeout)
- [ ] Closes Redis connection
- [ ] Logs shutdown progress
- [ ] Exit code 0 on clean shutdown, 1 on forced

### Implementation Notes

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

slog.Info("shutting down...")

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
httpServer.Shutdown(ctx)

workerPool.Stop()   // finish in-flight, reject new
redisListener.Stop() // stop Redis input listener
redisClient.Close()

slog.Info("shutdown complete")
```

### Dependencies

- Task 0.7 (HTTP server)
- Task 1.4 (Worker pool)
- Task 1.5 (Redis listener)
- Task 0.6 (Redis client)
