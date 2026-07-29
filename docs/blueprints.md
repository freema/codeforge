# Blueprints

A **blueprint** is a named, reusable session template: a stored `CreateSessionRequest` plus an optional list of input parameters. It is the single concept behind what used to be three separate ones — session presets, workflow definitions, and workflow configs:

- a **preset** is a blueprint with no parameters (`parameters: []`)
- a **workflow** is a blueprint with parameters
- a **workflow config** is a blueprint whose parameter defaults are the saved values

Running a blueprint renders the template with the given parameters and creates a **regular session** through the same path as `POST /api/v1/sessions` — same validation, rate limit, and tenant enforcement. There is no separate run entity or workflow runtime; track progress via the returned session ID like any other session.

Blueprints are stored in SQLite (`blueprints` table) and managed via operator-only CRUD at `/api/v1/blueprints`.

## The Model

```json
{
  "id": "0d9f6f0e-...",
  "name": "fix-github-issue",
  "description": "Fix a numbered GitHub issue",
  "builtin": false,
  "request": {
    "repo_url": "{{.Params.repo_url}}",
    "prompt": "Fix issue #{{.Params.issue_number}}",
    "provider_key": "{{.Params.provider_key}}"
  },
  "parameters": [
    { "name": "repo_url", "required": true },
    { "name": "issue_number", "required": true },
    { "name": "provider_key", "required": false }
  ],
  "created_at": "...",
  "updated_at": "..."
}
```

- **`name`** — unique (alphanumeric, hyphens, underscores). API endpoints accept either the UUID or the name in the path.
- **`request`** — a `CreateSessionRequest`-shaped JSON template. `repo_url` is required; `prompt` is not (the run endpoint can supply it as an override). String values may contain Go template expressions over `{{.Params.x}}`.
- **`tool_key_ref`** — one extra optional top-level field in `request`: the name of a [key registry](api.md#keys) entry whose token is injected as `auth_token` into each tool's config at run time (explicit `auth_token` values win).
- **`parameters`** — declared inputs: `{name, required, default}`. Empty for presets.
- **`builtin`** — built-in blueprints are seeded on startup and cannot be modified or deleted.

### Rendering Semantics

At run time, parameters are merged with the declaration:

1. Caller-supplied `params` win.
2. A declared parameter that is missing gets its `default`.
3. A missing **required** parameter without a default is a `400` error.
4. A missing optional parameter without a default renders as `""`.

Every **string leaf** of the request JSON is then rendered individually through the template engine — the blob is never rendered as one big template, so parameter values containing quotes or newlines cannot break the JSON structure. Non-string values (numbers, booleans, nested objects) pass through untouched.

## API

```
POST   /api/v1/blueprints                 create (name + request required)
GET    /api/v1/blueprints                 list   → {"blueprints": [...]}
GET    /api/v1/blueprints/{id-or-name}    get
PUT    /api/v1/blueprints/{id-or-name}    full replacement (builtins → 400)
DELETE /api/v1/blueprints/{id-or-name}    delete (204; builtins → 400; referenced by schedules → 409)
POST   /api/v1/blueprints/{id-or-name}/run   run → creates a session (session-creation rate limit applies)
```

A duplicate name on create/update returns `409`. Deleting a blueprint that schedules still reference returns `409` with the schedule count — detach or delete the schedules first.

### Run

```bash
curl -X POST http://localhost:8080/api/v1/blueprints/fix-github-issue/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "params": {
      "repo_url": "https://github.com/user/repo.git",
      "issue_number": "42",
      "provider_key": "my-github-key"
    }
  }'
```

The body is optional for blueprints whose required parameters all have defaults. An optional non-empty `prompt` field **overrides the rendered prompt** for this run:

```json
{ "params": { "repo_url": "..." }, "prompt": "Also add a regression test" }
```

Response `201` is the standard create-session response:

```json
{ "id": "abc-123-...", "status": "pending", "created_at": "..." }
```

Stream it via `GET /api/v1/sessions/{id}/stream`, cancel it via `POST /api/v1/sessions/{id}/cancel` — it is a normal session.

## Built-in Blueprints

Three built-ins are seeded (and kept up to date) on startup. They cannot be edited or deleted, but you can inspect them via `GET /api/v1/blueprints/{name}` and use them as templates for your own.

### `sentry-fixer`

Analyzes unresolved Sentry errors for a project, fixes the ones fixable in code (one commit per fix), and opens a single PR with all fixes. Connects the Sentry MCP tool and sets `auto_create_pr`.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `sentry_org` | yes | — | Sentry organization slug |
| `sentry_project` | yes | — | Sentry project slug |
| `repo_url` | yes | — | Repository to fix |
| `key_name` | yes | — | Registered key providing the Sentry auth token (`tool_key_ref`) |
| `provider_key` | no | — | Registered git provider key for clone/PR |
| `max_issues` | no | `5` | How many top issues to tackle |
| `pr_title` | no | `fix: resolve Sentry errors` | PR title |
| `target_branch` | no | — | PR target branch |

### `knowledge-update`

Runs a [`knowledge` session](session-types.md#knowledge): analyzes the repo, refreshes the `.codeforge/` knowledge docs, and opens a PR with the changes (`auto_create_pr` + `pr_title: "docs: update .codeforge knowledge"`).

| Parameter | Required | Description |
|-----------|----------|-------------|
| `repo_url` | yes | Repository to document |
| `provider_key` | no | Registered git provider key |
| `focus` | no | Focus area passed as the session prompt |

### `repo-review`

Runs a [`review` session](session-types.md#review): a full repository review (quality, security, architecture) producing a structured `ReviewResult`.

| Parameter | Required | Description |
|-----------|----------|-------------|
| `repo_url` | yes | Repository to review |
| `provider_key` | no | Registered git provider key |
| `focus` | no | Extra review focus passed as the session prompt |

```bash
curl -X POST http://localhost:8080/api/v1/blueprints/repo-review/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{ "params": { "repo_url": "https://github.com/user/repo.git", "provider_key": "my-github-key" } }'
```

## Scheduling a Blueprint

[Schedules](api.md#schedules--recurring-sessions-operator-only) accept **either** an inline `session_request` **or** a blueprint reference — exactly one of the two:

```json
{
  "name": "weekly-repo-review",
  "cron": "0 6 * * 1",
  "blueprint_id": "repo-review",
  "blueprint_params": { "repo_url": "https://github.com/acme/widget.git", "provider_key": "github-acme" }
}
```

The blueprint is rendered with `blueprint_params` on every firing, so edits to the blueprint take effect on the next run. The rendered request is validated like an inline one — required parameters must be satisfiable and a rendered `session_type: "pr_review"` is rejected (a schedule cannot target a fixed PR number). A blueprint referenced by schedules cannot be deleted (`409`).

## Migration from Presets / Workflows / Workflow Configs

Blueprints replace three legacy concepts. On startup, existing data **auto-migrates** into the `blueprints` table:

| Legacy concept | Becomes |
|----------------|---------|
| Session preset (`session_presets`) | Blueprint with the same id/name/request and `parameters: []` |
| Workflow definition (`workflow_definitions`, non-builtin) | Blueprint with the definition's parameters; the first `session` step's config becomes the request template (`{{.Params.x}}` placeholders kept intact) |
| Workflow config (`workflow_configs`) | Blueprint derived from the parent definition whose **parameter defaults are the saved values**, so the stored configuration keeps working as-is; a saved timeout is baked into `request.config.timeout_seconds` |

The migration is idempotent (re-runs are no-ops) and strictly non-destructive: the legacy tables stay in the schema, dormant, as a rollback archive. Name collisions with different content get a `-2`, `-3`, … suffix.

The old `/api/v1/workflows` and `/api/v1/workflow-configs` endpoints have been **removed**. There is no separate workflow runtime — running a blueprint is a synchronous render + create-session call.

## Deprecated: `/presets` Alias

`/api/v1/presets` remains mounted as a **deprecated alias** over the same blueprint store: the old create shape (no `parameters`) and the old `{"presets": [...]}` list envelope keep working, and get/update/delete/run behave identically to `/blueprints`. New integrations should use `/api/v1/blueprints`.
