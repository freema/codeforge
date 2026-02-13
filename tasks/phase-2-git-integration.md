# Phase 2 — Git Integration & PR/MR (v0.3.0)

> **PR/MR is never automatic.** Consumer sees `changes_summary` and explicitly requests PR creation via `POST /tasks/:id/create-pr`.

---

## Task 2.1: POST /tasks/:id/create-pr Endpoint

**Priority:** P0
**Files:** `internal/server/handlers/tasks.go`, `internal/task/service.go`

### Description

New endpoint that creates a PR/MR for a completed task. Consumer calls this only when they decide to create a PR based on `changes_summary`.

### Acceptance Criteria

- [ ] `POST /api/v1/tasks/{taskID}/create-pr` with optional body `{"title":"...","description":"...","target_branch":"main"}`
- [ ] Task must be in COMPLETED status (409 otherwise)
- [ ] Task must have changes (400 if `changes_summary` shows zero changes)
- [ ] Transitions: COMPLETED → CREATING_PR → PR_CREATED (or → FAILED)
- [ ] If no title/description, auto-generate via prompt analyzer (Task 2.2)
- [ ] Returns 200 with `{"pr_url":"...","pr_number":123,"branch":"codeforge/..."}`
- [ ] PR URL, number, branch stored in task state

### Dependencies

- Task 1.1 (state machine), Task 1.9 (changes_summary), Tasks 2.2-2.6

---

## Task 2.2: Prompt Analyzer (for PR naming)

**Priority:** P1
**Files:** `internal/cli/analyzer.go`

### Description

Auto-generate branch slug, PR title, and description when consumer doesn't provide them. Uses Anthropic API directly (not full Claude Code CLI — too heavy for 3 fields).

### Acceptance Criteria

- [ ] Input: task prompt + changes_summary
- [ ] Output: `AnalysisResult{BranchSlug, PRTitle, Description}`
- [ ] Uses Anthropic Messages API directly with claude-haiku (fast + cheap)
- [ ] Falls back to generic names on failure: `task-{short_id}`, `CodeForge: {truncated prompt}`
- [ ] Fast: < 2 seconds

### Implementation Notes

```go
// Anthropic Messages API — direct HTTP call (no SDK needed for this simple use case)
// Base URL: https://api.anthropic.com/v1/messages
// Required headers:
//   x-api-key: <ANTHROPIC_API_KEY>
//   anthropic-version: 2023-06-01
//   content-type: application/json
// Model: "claude-haiku-4-5" (fast, cheap — ~$0.001 per call)
//
// Request body:
// {
//   "model": "claude-haiku-4-5",
//   "max_tokens": 256,
//   "messages": [{"role": "user", "content": "<prompt>"}]
// }

func (a *Analyzer) Analyze(ctx context.Context, prompt string, summary *ChangesSummary) (*AnalysisResult, error) {
    body := map[string]interface{}{
        "model":      "claude-haiku-4-5",
        "max_tokens": 256,
        "messages": []map[string]string{
            {"role": "user", "content": buildAnalyzerPrompt(prompt, summary)},
        },
    }

    req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", marshalBody(body))
    req.Header.Set("x-api-key", a.apiKey)
    req.Header.Set("anthropic-version", "2023-06-01")
    req.Header.Set("content-type", "application/json")

    resp, err := a.client.Do(req)
    // Parse response → extract branch slug, PR title, description
    // On any error → return fallback values
}
```

### Dependencies

- Task 0.2 (AI API key config)

---

## Task 2.3: Branch Management

**Priority:** P0
**Files:** `internal/git/branch.go`

### Description

Create branch, stage changes, commit, push. Only called via create-pr endpoint.

### Acceptance Criteria

- [ ] Branch: `codeforge/{slug}` (prefix from config), numeric suffix if exists
- [ ] `git add -A` → commit → push
- [ ] Handle empty commits (error if nothing to commit)
- [ ] Conventional commit: `feat(codeforge): {title}`
- [ ] Token passed via `GIT_ASKPASS` for push (same mechanism as clone — Task 1.6)
- [ ] Token NEVER stored in `.git/config` or remote URL
- [ ] All via `exec.Command` (no shell injection)
- [ ] Stream events: `git.branch_created`, `git.push_completed`

### Dependencies

- Task 2.1 (create-pr endpoint), Task 2.2 (slug)

---

## Task 2.4: GitHub PR Creator

**Priority:** P0
**Files:** `internal/git/github.go`

### Description

Create Pull Request on GitHub via REST API.

### Acceptance Criteria

- [ ] `POST /repos/{owner}/{repo}/pulls` with title, body, head, base
- [ ] Labels: `codeforge` (best effort)
- [ ] Returns PR URL + number
- [ ] Error handling: branch not pushed, insufficient permissions

### Dependencies

- Task 2.3 (branch pushed), Task 2.6 (provider detection)

---

## Task 2.5: GitLab MR Creator

**Priority:** P0
**Files:** `internal/git/gitlab.go`

### Description

Create Merge Request on GitLab via REST API.

### Acceptance Criteria

- [ ] `POST /api/v4/projects/{id}/merge_requests`
- [ ] Project ID from URL-encoded path
- [ ] Returns MR URL + IID

### Dependencies

- Task 2.3 (branch pushed), Task 2.6 (provider detection)

---

## Task 2.6: Provider Detection

**Priority:** P0
**Files:** `internal/git/provider.go`

### Description

Auto-detect GitHub vs GitLab from repo URL. Extract owner/repo. Support custom domains via config.

### Acceptance Criteria

- [ ] Detects `github.com` → GitHub, `gitlab.com` → GitLab
- [ ] Custom domains via config: `git.provider_domains: {"git.company.com": "gitlab"}`
- [ ] Returns `RepoInfo{Provider, Owner, Repo, Host}`
- [ ] Unknown providers: error "PR creation not supported"

### Dependencies

None.

---

## Task 2.7: Commit Formatting

**Priority:** P1
**Files:** `internal/git/commit.go`

### Description

Conventional commit format with task metadata.

### Acceptance Criteria

- [ ] Format: `feat(codeforge): {title}\n\nTask ID: {id}\nCo-authored-by: CodeForge Bot <codeforge@noreply>`

### Dependencies

- Task 2.3 (commit step)
