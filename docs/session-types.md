# Session Types

Every session has a `session_type` that controls how the user prompt is used, whether the AI may modify files, and what the output looks like. All types are selectable in the New Session form in the UI and via `session_type` on `POST /api/v1/sessions`; `GET /api/v1/session-types` lists them for clients.

| Type | Mode | Prompt | Schedulable |
|------|------|--------|-------------|
| `code` | Writes files | **Required** — the instruction itself | yes |
| `plan` | Read-only | **Required** — what to plan | yes |
| `review` | Read-only | Optional — extra review focus | yes |
| `pr_review` | Read-only | Optional — extra review focus | **no** |
| `knowledge` | Writes only inside `.codeforge/` | Optional — focus area | yes |

Template wrapping happens in the executor at runtime — `Session.Prompt` always stores the original user prompt, and the type's template (if any) is applied only when the CLI runs. See [Architecture](architecture.md) for the session type system internals.

## `code` (default)

Free-form code work. The user prompt is passed to the AI CLI **as-is** — no template wrapping. The AI may create, modify, and delete files anywhere in the workspace. This is the type behind the normal edit → review → instruct → create PR loop.

- **Output:** raw CLI result text plus the workspace diff (inspect it in the UI or via the diff endpoint before creating a PR)
- **Prompt:** required — it is the entire instruction

```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "Fix the failing tests in the auth module",
    "session_type": "code"
  }'
```

## `plan`

Read-only analysis. The prompt is wrapped in the `plan.md` template, which instructs the AI to explore the repository and produce an implementation plan **without modifying any files**.

- **Output:** structured markdown with exactly these sections:
  - **Summary** — the task and the chosen approach
  - **Files to Change** — each file's path plus what changes are needed and why
  - **Approach** — ordered implementation steps
  - **Risks** — potential risks and their mitigations
  - **Complexity** — S/M/L estimate with rationale
- **Prompt:** required — the task to plan
- **Typical follow-up:** start a `code` session (or `instruct` in the same workspace) using the plan as input

```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "Plan the migration from REST polling to SSE streaming",
    "session_type": "plan"
  }'
```

## `review`

Read-only review of the **entire repository** (not a diff). The prompt is wrapped in the `review.md` template; the AI analyzes code quality, architecture, security, performance, and test coverage, then outputs the [shared review JSON contract](#shared-review-json-contract) which is parsed into a `ReviewResult` on the session.

- **Output:** structured JSON (see contract below), stored as `review_result` and rendered in the UI
- **Prompt:** optional — when provided it acts as an **extra focus** (e.g. "focus on error handling in the worker pool"); an empty prompt runs the default full review
- **Schedulable:** yes — a weekly repo health review is the canonical schedule use case

```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "",
    "session_type": "review"
  }'
```

Not to be confused with `POST /sessions/:id/review`, which reviews **one session's changes** (`git diff HEAD~1`) — see [Code Review](code-review-workflow.md).

## `pr_review`

Read-only review of a **specific pull request / merge request diff**. Clones the target branch, fetches the PR ref (`git fetch origin pull/{N}/head:pr-{N}` — fork PRs work automatically), reviews `git diff origin/{base}...HEAD`, and outputs the [shared review JSON contract](#shared-review-json-contract). With `output_mode: "post_comments"` the result is posted to the PR/MR as line-level comments.

- **Output:** structured JSON (see contract below), stored as `review_result`; optionally posted to the PR/MR
- **Prompt:** optional — extra review focus on top of the template
- **Requires:** `config.pr_number` (plus source/target branches)
- **Schedulable:** **no** — a schedule cannot target a fixed PR number; PR reviews are event-driven (API call or [webhook](code-review-workflow.md#3-webhook-triggered-pr-review))

```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "Review pull request #42",
    "session_type": "pr_review",
    "config": {
      "pr_number": 42,
      "source_branch": "feature/login",
      "target_branch": "main",
      "output_mode": "post_comments"
    }
  }'
```

Full details (fork handling, comment posting, webhooks) in [Code Review](code-review-workflow.md).

## `knowledge`

Analyzes the repository, then creates or updates project knowledge docs. The AI writes **only inside the `.codeforge/` directory** — the rest of the workspace stays untouched.

- **Output:** three markdown files at the repo root:
  - `.codeforge/OVERVIEW.md` — purpose, tech stack, how to run/build/test, entry points
  - `.codeforge/ARCHITECTURE.md` — system design, directory structure, key abstractions, data flow
  - `.codeforge/CONVENTIONS.md` — coding patterns, error handling, testing, naming
- **Prompt:** optional — a **focus area** (e.g. "focus on the streaming subsystem"); empty prompt covers the whole repo
- **Existing files are updated, not blindly overwritten** — accurate content is preserved
- **Pairs with `create-pr`:** the docs live in the workspace like any other change, so `POST /sessions/:id/create-pr` opens a PR that persists them into the repo
- **Schedulable:** yes — e.g. a weekly schedule keeps `.codeforge/` docs fresh as the codebase evolves

The `.codeforge/` files (together with `CLAUDE.md`) are auto-injected as system context by the CI Action executor (`buildSystemContext` in `cmd/codeforge-action/ci_executor.go`), so keeping them current directly improves automated PR reviews.

```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "provider_key": "my-github-key",
    "prompt": "",
    "session_type": "knowledge"
  }'
```

## Shared Review JSON Contract

`review` and `pr_review` share a single output contract, defined once in `internal/prompt/templates/review_schema.md` and embedded into both templates:

```json
{
  "verdict": "approve" | "request_changes" | "comment",
  "score": 1-10,
  "summary": "brief overall assessment",
  "issues": [
    {
      "severity": "critical" | "major" | "minor" | "suggestion",
      "file": "path/to/file.go",
      "line": 42,
      "description": "what's wrong",
      "suggestion": "how to fix"
    }
  ],
  "auto_fixable": false
}
```

The multi-strategy parser (`internal/review/parser.go`) extracts this from the CLI output and stores it as `ReviewResult` on the session. Verdict and severity semantics are documented in [Code Review](code-review-workflow.md#review-output-format).

## Triggering

- **UI:** the New Session form has a toggle for every type; `review` and `knowledge` accept an empty prompt there as well
- **API:** `POST /api/v1/sessions` with `session_type` (examples above); `GET /api/v1/session-types` returns the list with labels and descriptions — see [API Reference](api.md#session-types)
- **Schedules:** `code`, `plan`, `review`, and `knowledge` can run on a cron via [Schedules](api.md#schedules--recurring-sessions-operator-only); `pr_review` schedules are rejected with `400`

Example — weekly knowledge refresh schedule:

```json
{
  "name": "weekly-knowledge",
  "cron": "0 6 * * 1",
  "session_request": {
    "repo_url": "https://github.com/acme/widget.git",
    "provider_key": "github-acme",
    "session_type": "knowledge",
    "prompt": ""
  }
}
```

## CI Action Type Names

The [CI Action](ci-action.md) predates the server-side type list and uses its **own** `session_type` names — they are related but not identical:

| CI Action `session_type` | Server equivalent | Notes |
|--------------------------|-------------------|-------|
| `pr_review` | `pr_review` | Same concept: review the PR/MR diff |
| `code_review` | `review` (branch-scoped) | Reviews branch changes against the base branch, no PR context needed |
| `knowledge_update` | `knowledge` | Same concept: create/update `.codeforge/` docs |
| `custom` | `code` | Runs a custom prompt |

When reading [CI Action docs](ci-action.md), use the names in the left column — they are the action's input values and are intentionally kept as-is.
