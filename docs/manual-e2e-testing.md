# Manual E2E Testing

Manual end-to-end tests verify the full CodeForge lifecycle against a real GitHub repository. These tests use real CLI execution (Claude Code / Codex), real GitHub API calls, and validate the complete flow from task creation to PR cleanup.

## Prerequisites

- Dev environment running: `task dev` (or `task dev:detach`)
- GitHub access key registered as `my-github` (or any name — adjust `provider_key` below)
- `gh` CLI authenticated (for PR verification and cleanup)
- Test repository: `https://github.com/freema/fb-pilot.git` (or any repo accessible with the registered key)

```bash
# Verify environment
curl -s http://localhost:8080/health | jq .
curl -s -H "Authorization: Bearer dev-token" http://localhost:8080/api/v1/auth/verify | jq .
curl -s -H "Authorization: Bearer dev-token" http://localhost:8080/api/v1/cli | jq .
curl -s -H "Authorization: Bearer dev-token" http://localhost:8080/api/v1/keys | jq .
```

## Variables

All tests use these values — adjust as needed:

```bash
export BASE=http://localhost:8080
export TOKEN="dev-token"
export AUTH="Authorization: Bearer $TOKEN"
export REPO="https://github.com/freema/fb-pilot.git"
export PROVIDER_KEY="my-github"
export GH_REPO="freema/fb-pilot"
```

---

## Test 1: Claude Code + Follow-up + PR

Full lifecycle: clone → Claude Code task → follow-up instruction → create PR → verify → cleanup.

### Step 1: Create task

```bash
TASK=$(curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"repo_url\": \"$REPO\",
    \"provider_key\": \"$PROVIDER_KEY\",
    \"prompt\": \"Add a comment at the top of README.md: // E2E Test 1\",
    \"config\": {\"cli\": \"claude-code\", \"timeout_seconds\": 300}
  }")
TASK_ID=$(echo $TASK | jq -r .id)
echo "Task: $TASK_ID"
```

### Step 2: Wait for completion

```bash
while true; do
  STATUS=$(curl -s -H "$AUTH" "$BASE/api/v1/tasks/$TASK_ID" | jq -r .status)
  echo "Status: $STATUS"
  [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ] && break
  sleep 5
done
```

**Expected:** `status: completed`, `result` contains description of changes, `iteration: 1`.

### Step 3: Follow-up instruct

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/instruct" \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"prompt": "Add a second comment line: // E2E Test 1 follow-up"}' | jq .
```

**Expected:** `iteration: 2`, `status: awaiting_instruction`.

Wait for completion again (same polling loop). Then verify iterations:

```bash
curl -s -H "$AUTH" "$BASE/api/v1/tasks/$TASK_ID?include=iterations" | jq '.iterations | length'
```

**Expected:** `2` iterations.

### Step 4: Create PR

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/create-pr" \
  -H "$AUTH" -H "Content-Type: application/json" -d '{}' | jq .
```

**Expected:** `pr_url`, `pr_number`, `branch` starting with `codeforge/`.

### Step 5: Verify and cleanup

```bash
# Verify PR exists on GitHub
gh pr view <PR_NUMBER> --repo $GH_REPO --json title,state,headRefName

# Cleanup
gh pr close <PR_NUMBER> --repo $GH_REPO --delete-branch
```

**Expected:** PR is OPEN, title starts with "CodeForge:". After close: branch deleted.

---

## Test 2: Claude Code + Codex Code Review + Post Comments

Flow: clone → Claude task → code review (Codex) → create PR → post review comments → verify → cleanup.

> **Important:** Review must happen BEFORE create-pr. The state machine does not allow `pr_created → reviewing`.

### Step 1: Create task (Claude Code)

```bash
TASK=$(curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"repo_url\": \"$REPO\",
    \"provider_key\": \"$PROVIDER_KEY\",
    \"prompt\": \"Add a file CODEFORGE_TEST.md with: # E2E Test 2\",
    \"config\": {\"cli\": \"claude-code\", \"timeout_seconds\": 300}
  }")
TASK_ID=$(echo $TASK | jq -r .id)
```

Wait for `completed`.

### Step 2: Code Review with Codex

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/review" \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"cli": "codex"}' | jq .
```

**Expected:** `verdict`, `score`, `summary`, `reviewed_by: "codex:..."`.

Verify review_result on task:

```bash
curl -s -H "$AUTH" "$BASE/api/v1/tasks/$TASK_ID" | jq .review_result
```

### Step 3: Create PR

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/create-pr" \
  -H "$AUTH" -H "Content-Type: application/json" -d '{}' | jq .
```

### Step 4: Post review comments to GitHub

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/post-review" \
  -H "$AUTH" -H "Content-Type: application/json" -d '{}' | jq .
```

**Expected:** `review_url` (GitHub URL), `comments_posted` (number), `pr_number`.

### Step 5: Verify and cleanup

```bash
# Check review exists on PR
gh pr view <PR_NUMBER> --repo $GH_REPO --json reviews

# Cleanup
gh pr close <PR_NUMBER> --repo $GH_REPO --delete-branch
```

**Expected:** Review comment with CodeForge summary posted on PR.

---

## Test 3: Codex Task + Claude Code Review + Post Comments

Reversed CLI combination: Codex writes code, Claude Code reviews.

### Step 1: Create task (Codex)

```bash
TASK=$(curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"repo_url\": \"$REPO\",
    \"provider_key\": \"$PROVIDER_KEY\",
    \"prompt\": \"Create CODEX_TEST.md with: # Codex Test\",
    \"config\": {\"cli\": \"codex\", \"timeout_seconds\": 300}
  }")
TASK_ID=$(echo $TASK | jq -r .id)
```

Wait for `completed`.

### Step 2: Code Review with Claude Code

```bash
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/review" \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"cli": "claude-code"}' | jq .
```

**Expected:** `verdict`, `score`, `reviewed_by: "claude-code:..."`.

### Step 3-5: Create PR → Post Review → Verify → Cleanup

Same as Test 2 steps 3-5.

> **Known limitation:** If Claude Code gives verdict `approve`, `post-review` will fail on GitHub with "Can not approve your own pull request" when the same token owns the PR. Workaround: use a different token for review posting, or accept `COMMENT` verdict.

---

## Test 4: Task Cancellation

Verify that canceling a running/cloning task transitions it to `failed`.

```bash
# Create a long-running task
TASK=$(curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"repo_url\": \"$REPO\",
    \"provider_key\": \"$PROVIDER_KEY\",
    \"prompt\": \"Write a detailed analysis of every file in the repo\",
    \"config\": {\"cli\": \"claude-code\", \"timeout_seconds\": 300}
  }")
TASK_ID=$(echo $TASK | jq -r .id)

# Wait until cloning or running
# (poll status until cloning/running)

# Cancel
curl -s -X POST "$BASE/api/v1/tasks/$TASK_ID/cancel" -H "$AUTH" | jq .
```

**Expected:** Response: `status: canceling`. After a few seconds, task status becomes `failed` with error `canceled by user`.

---

## Test 5: Edge Cases

### pr_review without pr_number

```bash
curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"repo_url": "https://github.com/freema/fb-pilot.git", "prompt": "Review", "task_type": "pr_review"}' | jq .
```

**Expected:** 400 with `fields.pr_number: "pr_number is required for pr_review tasks"`.

### Non-existent task

```bash
curl -s -H "$AUTH" "$BASE/api/v1/tasks/nonexistent-id" | jq .
```

**Expected:** 404 with `"task nonexistent-id not found"`.

### Invalid repo URL

```bash
curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"repo_url": "not-a-url", "prompt": "test"}' | jq .
```

**Expected:** 400 validation error.

### Missing prompt

```bash
curl -s -X POST $BASE/api/v1/tasks \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"repo_url": "https://github.com/freema/fb-pilot.git"}' | jq .
```

**Expected:** 400 with `fields.Prompt: "field is required"`.

### No auth

```bash
curl -s $BASE/api/v1/tasks | jq .
```

**Expected:** 401 `"missing or invalid Bearer token"`.

### Review on non-existent task

```bash
curl -s -X POST "$BASE/api/v1/tasks/fake-id/review" \
  -H "$AUTH" -H "Content-Type: application/json" -d '{}' | jq .
```

**Expected:** 404 `"task fake-id not found"`.

---

## Known Limitations

| Issue | Description |
|-------|-------------|
| **Review before PR** | State machine requires review BEFORE `create-pr`. Order: task → review → create-pr → post-review |
| **Approve own PR** | GitHub API rejects `APPROVE` review on own PR. Use different token or expect `COMMENT` |
| **Codex diff issue** | Codex review may report `HEAD~1` not available — review prompt could provide diff explicitly |

## Test Results Log

Record test runs here for tracking:

| Date | Tester | Tests Run | Pass | Fail | Bugs Found | Notes |
|------|--------|-----------|------|------|------------|-------|
| 2026-03-12 | Claude Code | 1-5 | 4 | 0 | 2 fixed | nil comments in post-review, cancel stuck in cloning |
