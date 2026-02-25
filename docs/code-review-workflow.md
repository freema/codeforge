# Code Review Workflow

CodeForge includes a built-in `code-review` workflow that chains two steps:

1. **execute_task** — an AI agent (default: Claude Code) implements changes
2. **code_review** — a different AI agent (default: Codex) reviews the changes in the same workspace

The review agent runs `git diff HEAD~1` to see exactly what the first agent changed, then produces a structured review with severity levels and a final verdict (PASS / WARN / FAIL).

## How It Works

```
POST /api/v1/workflows/code-review/run
     │
     ▼
┌─────────────┐    workspace    ┌──────────────┐
│ execute_task │───────────────▶│ code_review  │
│ (claude-code)│   reused via   │   (codex)    │
│              │ WorkspaceTaskID│              │
└─────────────┘                 └──────────────┘
     │                               │
     ▼                               ▼
  Task result                  Review result
  (code changes)               (PASS/WARN/FAIL)
```

### Workspace Reuse

The `code_review` step references `execute_task` via `WorkspaceTaskRef`. When the workflow engine creates the review task, it resolves the referenced step's `task_id` and sets `WorkspaceTaskID` on the new task. The executor then skips cloning and runs the review CLI directly in the first task's workspace directory.

### Two-Phase Prompt Rendering

The code-review prompt template (`internal/prompt/templates/code_review.md`) uses Go `text/template` with a `{{.OriginalPrompt}}` variable. At startup, this is pre-rendered with `{{.Params.prompt}}` as the original prompt value. At workflow runtime, the workflow template engine resolves `{{.Params.prompt}}` to the actual user prompt. This avoids template escaping issues.

## API Usage

### Start a Code Review Workflow

```bash
curl -X POST http://localhost:8080/api/v1/workflows/code-review/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "params": {
      "repo_url": "https://github.com/owner/repo.git",
      "prompt": "Add input validation to the /api/v1/users endpoint",
      "provider_key": "my-github-key",
      "access_token": "ghp_..."
    }
  }'
```

**Response:**
```json
{
  "run_id": "wfr-abc123",
  "workflow_name": "code-review",
  "status": "pending",
  "created_at": "2026-02-25T10:00:00Z"
}
```

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `repo_url` | yes | — | Repository URL to clone |
| `prompt` | yes | — | Task description for the coding agent |
| `provider_key` | no | — | Key name for git credentials |
| `access_token` | no | — | Direct git access token |
| `source_branch` | no | — | Branch to clone/checkout |
| `cli` | no | `claude-code` | CLI runner for the coding step |
| `review_cli` | no | `codex` | CLI runner for the review step |

### Check Run Status

```bash
curl http://localhost:8080/api/v1/workflow-runs/$RUN_ID \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "id": "wfr-abc123",
  "workflow_name": "code-review",
  "status": "completed",
  "steps": [
    {
      "step_name": "execute_task",
      "step_type": "task",
      "status": "completed",
      "result": {
        "task_id": "task-111",
        "status": "completed",
        "result": "Added input validation..."
      }
    },
    {
      "step_name": "code_review",
      "step_type": "task",
      "status": "completed",
      "result": {
        "task_id": "task-222",
        "status": "completed",
        "result": "## Code Review\n\n...PASS"
      }
    }
  ]
}
```

### Stream Events (SSE)

```bash
curl -N http://localhost:8080/api/v1/workflow-runs/$RUN_ID/stream \
  -H "Authorization: Bearer $TOKEN"
```

## Review Output Format

The review agent produces structured output:

```
## Code Review

### Issues

1. **[warning]** `src/handlers/users.go:42` — Missing error check on `json.Unmarshal`.
   Fix: Add `if err != nil { return err }` after unmarshal.

2. **[info]** `src/handlers/users.go:55` — Consider using a constant for max name length.

### Verdict: PASS

No critical issues found. Two minor suggestions for improvement.
```

Severity levels:
- **critical** — must fix before merging
- **warning** — should fix, but not a blocker
- **info** — suggestion or style improvement

Verdicts:
- **PASS** — no issues or only info-level findings
- **WARN** — has warning-level findings but no blockers
- **FAIL** — has critical findings that must be fixed

## Internal Architecture

### Prompt Package (`internal/prompt/`)

```
internal/prompt/
  prompt.go                 # Render(name, data) + CodeReviewData struct
  prompt_test.go            # Unit tests
  templates/
    code_review.md          # Review prompt template
```

Templates are embedded via `//go:embed templates/*.md` and rendered with Go `text/template`.

### Key Types

```go
// internal/task/model.go
type TaskConfig struct {
    // ... existing fields ...
    WorkspaceTaskID string `json:"workspace_task_id,omitempty"`
}

// internal/workflow/model.go
type TaskStepConfig struct {
    // ... existing fields ...
    CLI              string `json:"cli,omitempty"`
    AIModel          string `json:"ai_model,omitempty"`
    SourceBranch     string `json:"source_branch,omitempty"`
    WorkspaceTaskRef string `json:"workspace_task_ref,omitempty"`
}
```

### Workspace Reuse Flow (executor.go)

```
1. Task has Config.WorkspaceTaskID set
2. Executor looks up referenced task's workspace via workspaceMgr.Get()
3. If workspace path exists on disk → use it, skip clone
4. No new workspace entry is created — review task is ephemeral
```

## E2E Testing

The code-review workflow can be tested E2E using the mock CLI infrastructure.

### Prerequisites

```bash
# Build mock CLI + start E2E environment
task dev:e2e:detach
```

### Manual E2E Test

```bash
# 1. Start the workflow
RUN=$(curl -s -X POST http://localhost:8080/api/v1/workflows/code-review/run \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{
    "params": {
      "repo_url": "https://github.com/owner/repo.git",
      "prompt": "Add hello world function",
      "cli": "claude-code",
      "review_cli": "claude-code"
    }
  }' | jq -r '.run_id')

echo "Run ID: $RUN"

# 2. Poll status until completed
watch -n2 "curl -s http://localhost:8080/api/v1/workflow-runs/$RUN \
  -H 'Authorization: Bearer dev-token' | jq '.status, .steps[]?.status'"

# 3. Get full results
curl -s http://localhost:8080/api/v1/workflow-runs/$RUN \
  -H "Authorization: Bearer dev-token" | jq .
```

> **Note:** For E2E testing, set both `cli` and `review_cli` to `claude-code` since the mock CLI only simulates Claude Code. In production, use different runners (e.g. `claude-code` + `codex`).

### Automated E2E Test

Add to `tests/e2e/e2e_test.go`:

```go
func TestE2ECodeReviewWorkflow(t *testing.T) {
    repoDir := createTestRepo(t, "code-review")

    // Start workflow
    resp := apiRequest(t, "POST", "/api/v1/workflows/code-review/run", map[string]interface{}{
        "params": map[string]string{
            "repo_url":   "file://" + repoDir,
            "prompt":     "Add a hello world function",
            "cli":        "claude-code",  // mock CLI
            "review_cli": "claude-code",  // mock CLI (no codex mock)
        },
    })
    if resp.StatusCode != http.StatusCreated {
        b, _ := io.ReadAll(resp.Body)
        t.Fatalf("start workflow: expected 201, got %d: %s", resp.StatusCode, b)
    }

    var runResult map[string]interface{}
    decodeJSON(t, resp, &runResult)
    runID := runResult["run_id"].(string)
    t.Logf("workflow run created: %s", runID)

    // Wait for workflow to complete (both steps)
    deadline := time.Now().Add(120 * time.Second)
    for time.Now().Before(deadline) {
        resp := apiRequest(t, "GET", "/api/v1/workflow-runs/"+runID, nil)
        var run map[string]interface{}
        decodeJSON(t, resp, &run)

        status := run["status"].(string)
        if status == "completed" {
            // Verify both steps completed
            steps, ok := run["steps"].([]interface{})
            if !ok || len(steps) < 2 {
                t.Fatalf("expected 2 steps, got %d", len(steps))
            }

            for _, s := range steps {
                step := s.(map[string]interface{})
                if step["status"] != "completed" {
                    t.Errorf("step %s: expected completed, got %s",
                        step["step_name"], step["status"])
                }
                // Verify task_id was produced
                result, _ := step["result"].(map[string]interface{})
                if result["task_id"] == nil || result["task_id"] == "" {
                    t.Errorf("step %s: missing task_id", step["step_name"])
                }
            }
            t.Log("code-review workflow completed successfully")
            return
        }

        if status == "failed" {
            t.Fatalf("workflow failed: %v", run["error"])
        }

        time.Sleep(1 * time.Second)
    }
    t.Fatal("timed out waiting for workflow to complete")
}
```

### What the E2E Test Verifies

1. **Workflow startup** — `POST /api/v1/workflows/code-review/run` returns 201
2. **Step 1 (execute_task)** — task is created, clones repo, runs mock CLI, completes
3. **Step 2 (code_review)** — task is created with `WorkspaceTaskID` from step 1, reuses workspace (no re-clone), runs mock CLI, completes
4. **Workflow completion** — both steps have `status: completed` and `task_id` in results

### Limitations of Mock CLI Testing

The mock CLI doesn't simulate real `git diff` output or real code review behavior. The E2E test verifies the **orchestration** is correct:
- Workflow starts and runs both steps sequentially
- Workspace reuse works (step 2 doesn't need to clone)
- Template rendering works (prompt variables resolved)
- Status propagation works (both steps complete → workflow completes)

For validating actual review quality, use real CLIs in a staging environment.
