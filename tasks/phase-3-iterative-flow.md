# Phase 3 — Iterative Flow (v0.4.0)

> Multi-turn conversations: follow-up instructions on existing workspace.
> If PR was created (Phase 2), follow-up pushes new commits to the same branch.

---

## Task 3.1: POST /tasks/:id/instruct Endpoint

**Priority:** P0
**Files:** `internal/server/handlers/tasks.go`, `internal/task/service.go`

### Description

Accept follow-up instructions for an existing task. The task must be in AWAITING_INSTRUCTION or COMPLETED status. Transitions back to RUNNING for a new iteration.

### Acceptance Criteria

- [ ] `POST /api/v1/tasks/{taskID}/instruct` with JSON body `{"prompt": "..."}`
- [ ] Validates task exists and is in AWAITING_INSTRUCTION, COMPLETED, or PR_CREATED status
- [ ] Creates new iteration record (iteration number, prompt, timestamp)
- [ ] Transitions task to RUNNING
- [ ] Re-enqueues task to `queue:tasks` for worker processing
- [ ] Returns 200 with updated task status
- [ ] Returns 404 if task not found
- [ ] Returns 409 if task is currently RUNNING (conflict)
- [ ] Returns 400 if task is FAILED (must create new task)

### Implementation Notes

```go
type InstructRequest struct {
    Prompt string `json:"prompt" validate:"required"`
}

// Task payload for re-execution includes:
// - New prompt (follow-up instruction)
// - Reference to previous iteration's result
// - Same workspace (no re-clone)
// - Same branch (push new commits)

func (s *TaskService) Instruct(ctx context.Context, taskID string, req InstructRequest) (*Task, error) {
    task, err := s.Get(ctx, taskID)
    if err != nil { return nil, err }

    if task.Status != StatusAwaitingInstruction && task.Status != StatusCompleted && task.Status != StatusPRCreated {
        return nil, ErrInvalidTransition
    }

    // Transition: COMPLETED → AWAITING_INSTRUCTION → RUNNING
    // (two-step transition so state machine stays valid)
    task.Transition(StatusAwaitingInstruction)
    task.Iteration++
    task.CurrentPrompt = req.Prompt
    task.Transition(StatusRunning)

    s.redis.HSet(ctx, "task:"+taskID+":state", ...)
    s.redis.RPush(ctx, "queue:tasks", taskID)  // RPUSH for FIFO

    return task, nil
}
```

### Dependencies

- Task 1.2 (POST /tasks)
- Task 1.1 (state machine — AWAITING_INSTRUCTION transition)

---

## Task 3.2: Workspace Reuse

**Priority:** P0
**Files:** `internal/worker/executor.go`, `internal/workspace/manager.go`

### Description

For follow-up iterations, reuse the existing workspace (cloned repo + previous changes) instead of re-cloning. The worker detects that a workspace already exists and skips the clone step.

### Acceptance Criteria

- [ ] On first iteration: clone fresh, create workspace
- [ ] On subsequent iterations: skip clone, reuse existing workspace
- [ ] Workspace tracked in Redis: `workspace:{taskID}` → `{path, created_at, ttl, size_bytes}`
- [ ] If workspace is missing (deleted by TTL), re-clone
- [ ] Git state: workspace should be on the correct branch from previous iteration
- [ ] Pull latest changes before starting new iteration (in case of manual edits)

### Implementation Notes

```go
func (e *Executor) Execute(ctx context.Context, task *Task) error {
    workspace, err := e.workspaceMgr.Get(task.ID)

    if workspace == nil || !dirExists(workspace.Path) {
        // First iteration or workspace cleaned up: clone fresh
        workspace, err = e.clone(ctx, task)
    } else {
        // Subsequent iteration: reuse workspace
        // Pull latest changes only if a branch was created (PR flow)
        if task.Branch != "" {
            cmd := exec.CommandContext(ctx, "git", "pull", "origin", task.Branch)
            cmd.Dir = workspace.Path
            cmd.Env = gitAskPassEnv(task.Token) // GIT_ASKPASS for auth
            cmd.Run()
        }
        // If no branch (read-only mode), workspace is already up to date
        // from previous iteration — no pull needed
    }

    // Run AI CLI in existing workspace
    result, err := e.runner.Run(ctx, RunOptions{
        Prompt:  task.CurrentPrompt,
        WorkDir: workspace.Path,
    })
    // ...
}
```

### Dependencies

- Task 1.6 (clone service)
- Phase 5 Task 5.1 (workspace manager — can implement basic version here)

---

## Task 3.3: PR/MR Update (if PR exists)

**Priority:** P0
**Files:** `internal/git/github.go`, `internal/git/gitlab.go`, `internal/git/branch.go`

### Description

On follow-up iterations, if a PR was previously created (Phase 2), push new commits to the existing branch and update the PR description. If no PR exists, just update changes_summary — consumer can request PR later.

### Acceptance Criteria

- [ ] If `task.Branch` is set: push new commits to existing branch (no new branch)
- [ ] If `task.PRNumber` is set: update PR body with iteration history
- [ ] If no PR exists: just calculate new changes_summary, consumer decides
- [ ] GitHub: `PATCH /repos/{owner}/{repo}/pulls/{number}`
- [ ] GitLab: `PUT /api/v4/projects/{id}/merge_requests/{iid}`
- [ ] Commit message: `feat(codeforge): iteration {N} — {summary}`

### Implementation Notes

```go
// GitHub PR update
func (g *GitHubClient) UpdatePR(ctx context.Context, owner, repo string, prNumber int, body string) error {
    url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", g.baseURL, owner, repo, prNumber)
    // PATCH with {"body": "updated description"}
}

// Updated PR body template
body := fmt.Sprintf(`## CodeForge Task

### Iteration %d
**Prompt:** %s
**Status:** Completed

### Previous Iterations
%s
`, task.Iteration, task.CurrentPrompt, previousIterations)
```

### Dependencies

- Task 2.3 / 2.4 (PR/MR creation — reuse clients)
- Task 2.3 (branch management — push commits)

---

## Task 3.4: Conversation Context

**Priority:** P0
**Files:** `internal/cli/claude.go`, `internal/worker/executor.go`

### Description

When running follow-up iterations, pass previous prompts and results as context to the AI CLI so it understands the history.

### Acceptance Criteria

- [ ] Build context string from all previous iterations (prompts + summaries)
- [ ] Prepend context to current prompt: "Previous work: ... \n\n Now: {new prompt}"
- [ ] Context size limited to prevent token overflow (configurable max chars, default: 50000)
- [ ] If context too large, truncate oldest iterations first (drop from beginning)
- [ ] Context includes: iteration number, prompt, short summary of result
- [ ] **Summary strategy:** `summary` = first N characters of raw result text (default: 2000 chars), NOT a separate AI summarization call — keeps it simple and fast

### Implementation Notes

```go
func buildContextPrompt(task *Task) string {
    var ctx strings.Builder
    ctx.WriteString("## Previous iterations on this codebase:\n\n")
    for _, iter := range task.Iterations {
        fmt.Fprintf(&ctx, "### Iteration %d\nPrompt: %s\nSummary: %s\n\n",
            iter.Number, iter.Prompt, iter.Summary)
    }
    ctx.WriteString("## Current instruction:\n\n")
    ctx.WriteString(task.CurrentPrompt)
    return ctx.String()
}
```

### Dependencies

- Task 1.7 (Claude executor)
- Task 3.1 (iteration tracking)

---

## Task 3.5: POST /tasks/:id/cancel

**Priority:** P1
**Files:** `internal/server/handlers/tasks.go`, `internal/worker/executor.go`

### Description

Cancel a running task — kill the CLI process, mark as FAILED, clean up.

### Acceptance Criteria

- [ ] `POST /api/v1/tasks/{taskID}/cancel` cancels a running task
- [ ] Cancels the context associated with the task execution
- [ ] CLI process killed (process group kill)
- [ ] Task status set to FAILED with error "cancelled by user"
- [ ] Webhook callback sent with cancelled status
- [ ] Returns 200 on success
- [ ] Returns 409 if task is not currently running
- [ ] Returns 404 if task not found

### Implementation Notes

```go
// Worker pool tracks cancel functions per task
type Pool struct {
    cancels   map[string]context.CancelFunc
    cancelsMu sync.RWMutex
}

func (p *Pool) Cancel(taskID string) error {
    p.cancelsMu.RLock()
    cancel, ok := p.cancels[taskID]
    p.cancelsMu.RUnlock()
    if !ok {
        return ErrTaskNotRunning
    }
    cancel()
    return nil
}
```

### Dependencies

- Task 1.4 (worker pool)
- Task 1.10 (timeout uses same cancel mechanism)

---

## Task 3.6: Task Iteration History

**Priority:** P1
**Files:** `internal/task/model.go`, `internal/task/service.go`

### Description

Track all iterations with timestamps, prompts, results, and duration. Store as part of the task state.

### Acceptance Criteria

- [ ] Each iteration stored as: `{number, prompt, summary, status, started_at, finished_at, duration}`
- [ ] **Primary storage:** Redis list `task:{id}:iterations` (each entry is JSON-serialized Iteration)
- [ ] Task struct's `Iterations []Iteration` is populated by reading from Redis list on demand (NOT cached in task hash)
- [ ] On iteration complete: `RPUSH task:{id}:iterations <json>` — append only
- [ ] `GET /api/v1/tasks/{id}` includes `iteration` count and latest iteration info from task hash
- [ ] Full iteration history available via `GET /api/v1/tasks/{id}?include=iterations` → reads from Redis list
- [ ] TTL on iterations list matches `tasks.state_ttl` (set when task reaches terminal state)

### Implementation Notes

```go
type Iteration struct {
    Number     int        `json:"number"`
    Prompt     string     `json:"prompt"`
    Summary    string     `json:"summary"`
    Status     string     `json:"status"`
    StartedAt  time.Time  `json:"started_at"`
    FinishedAt time.Time  `json:"finished_at"`
    Duration   string     `json:"duration"`
}
```

### Dependencies

- Task 3.1 (instruct endpoint creates iterations)
