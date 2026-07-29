# CI Action

CodeForge ships a self-contained CI binary (`cmd/codeforge-action`) that runs as a **GitHub Action** or **GitLab CI step**. No server, Redis, or database needed — single-shot execution using the existing CI checkout.

## GitHub Actions

### Minimal Setup (PR Review)

```yaml
name: Code Review
on: pull_request

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: freema/codeforge@v1
        with:
          api_key: ${{ secrets.ANTHROPIC_API_KEY }}
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Manual Trigger (workflow_dispatch)

```yaml
name: Code Review
on:
  workflow_dispatch:
    inputs:
      pr_number:
        description: 'PR number to review'
        required: true
        type: number

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          ref: refs/pull/${{ inputs.pr_number }}/head
      - name: Fetch base branch
        run: git fetch origin main
      - uses: freema/codeforge@v1
        with:
          api_key: ${{ secrets.ANTHROPIC_API_KEY }}
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Knowledge Update (on merge to main)

```yaml
name: Update Knowledge
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  knowledge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: freema/codeforge@v1
        with:
          session_type: knowledge_update
          api_key: ${{ secrets.ANTHROPIC_API_KEY }}
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## GitLab CI

```yaml
code-review:
  stage: review
  image: ghcr.io/freema/codeforge-action:latest
  variables:
    CODEFORGE_CLI: claude-code
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  artifacts:
    reports:
      dotenv: codeforge.env
```

`$ANTHROPIC_API_KEY` and `$GITLAB_TOKEN` must be set as GitLab CI/CD variables. `GITLAB_TOKEN` needs to be a project (or personal) access token with `api` scope — **`$CI_JOB_TOKEN` cannot post MR discussions** (the API rejects it as `PRIVATE-TOKEN`), so the action fails fast with an actionable error if posting is requested with only a job token. Job tokens remain usable for cloning (git username `gitlab-ci-token`). Set `INPUT_POST_COMMENTS: "false"` to run without any GitLab token.

Self-managed GitLab instances work with zero extra configuration: the provider is detected from `$CI_SERVER_URL`, which GitLab CI sets automatically (scheme, host, and port are preserved). `GITLAB_URL` / `GITHUB_URL` are also honored as manual overrides.

### Manual MR review (trigger/web pipelines)

Pipelines started outside a merge-request event have no MR context. Target a specific MR by setting `INPUT_MR_IID` (or the cross-platform alias `INPUT_PR_NUMBER`) as a pipeline variable:

```yaml
manual-review:
  stage: review
  image: ghcr.io/freema/codeforge-action:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "web" && $INPUT_MR_IID
```

`CI_MERGE_REQUEST_IID` always wins when the pipeline does run in an MR context.

### GitLab outputs (dotenv)

The action writes `codeforge.env` for downstream jobs (`artifacts: reports: dotenv:`):

| Variable | Description |
|----------|-------------|
| `CODEFORGE_VERDICT` | Review verdict (reviews only) |
| `CODEFORGE_SCORE` | Review score 1-10 (reviews only) |
| `CODEFORGE_ISSUES_COUNT` | Number of issues found (reviews only) |
| `CODEFORGE_INPUT_TOKENS` | Input tokens consumed |
| `CODEFORGE_OUTPUT_TOKENS` | Output tokens consumed |
| `CODEFORGE_REVIEW_URL` | URL of the posted review (only when comments were posted) |

The CI log additionally shows the same human-readable review summary GitHub Actions gets.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `session_type` | `pr_review` | `pr_review`, `code_review`, `knowledge_update`, `custom` |
| `prompt` | | Custom prompt (required for `custom`, optional for reviews) |
| `cli` | `claude-code` | AI CLI: `claude-code` or `codex` |
| `model` | | AI model override |
| `api_key` | | AI API key (overrides `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`) |
| `provider_token` | | GitHub/GitLab token (defaults to `$GITHUB_TOKEN` / `$GITLAB_TOKEN`; `$CI_JOB_TOKEN` is a last resort and cannot post MR comments) |
| `mcp_config` | | MCP config JSON string or path to `.mcp.json` |
| `post_comments` | `true` | Post review as PR/MR comments |
| `output_format` | `json` | Output format: `json`, `markdown`, `text` |
| `max_turns` | | Max AI conversation turns |
| `allowed_tools` | | Comma-separated tool allowlist for Claude Code |
| `fail_on_request_changes` | `false` | Exit with code 1 when verdict is `request_changes` |

## Outputs

| Output | Description |
|--------|-------------|
| `verdict` | Review verdict: `approve`, `request_changes`, `comment` |
| `score` | Review score (1-10) |
| `issues_count` | Number of issues found |
| `input_tokens` | Input tokens consumed |
| `output_tokens` | Output tokens consumed |
| `review_url` | URL of the posted review (only when comments were posted) |
| `review` | Full review result as JSON |
| `output` | Raw CLI output |

On GitHub these are `$GITHUB_OUTPUT` step outputs; on GitLab the equivalent data is written to the `codeforge.env` dotenv artifact as `CODEFORGE_*` variables (see the GitLab section above).

## Task Types

### `pr_review` (default)

Reviews the PR/MR diff. Automatically detects PR number, branches, and commit SHA from the CI environment. Posts review comments if `post_comments=true`.

Inline comments are validated against the PR diff — only lines within diff hunks get inline comments, other issues go into the review summary body.

Exit code: `0` by default. Set `fail_on_request_changes: true` to exit with `1` on `request_changes` verdict.

### `code_review`

Reviews branch changes against base branch. Same review output format as `pr_review` but works without a PR context.

### `knowledge_update`

Analyzes the repository and creates/updates `.codeforge/` knowledge files:

- `.codeforge/OVERVIEW.md` — project purpose, tech stack, build/test
- `.codeforge/ARCHITECTURE.md` — system design, directory structure
- `.codeforge/CONVENTIONS.md` — coding patterns, error handling, naming

### `custom`

Runs a custom prompt. Requires `prompt` input.

## Knowledge System

The CI Action reads `.codeforge/` files and `CLAUDE.md` before running the AI CLI. This context is injected via `--append-system-prompt` (Claude Code) or prepended to the prompt (Codex).

**Self-improving loop:**

1. Developer merges PR → `knowledge_update` runs → updates `.codeforge/` docs
2. Next PR review reads fresh context → more accurate reviews
3. Repeat

## Authentication

| CLI | Required Secret |
|-----|----------------|
| Claude Code | `ANTHROPIC_API_KEY` |
| Codex | `OPENAI_API_KEY` |

Provider tokens for PR/MR comments:
- **GitHub**: `$GITHUB_TOKEN` (per-job, no setup needed)
- **GitLab**: `$GITLAB_TOKEN` — a project (or personal) access token with `api` scope, set as a CI/CD variable. `$CI_JOB_TOKEN` **cannot** post MR discussions; it is only picked up as a last resort and the action fails fast with a clear error when comment posting is requested with it. For cloning with a job token, git requires the username `gitlab-ci-token`.

## Docker Image

```bash
docker pull ghcr.io/freema/codeforge-action:latest
```

~130 MB base image (Alpine + git + Node.js). The selected CLI is installed at runtime via npm (~30s, negligible vs 2-5 min AI execution).

## Environment Variables

The CI Action reads configuration from `INPUT_*` environment variables (set automatically by GitHub Actions from `with:` inputs). For GitLab CI or standalone use, set variables directly:

| Variable | Maps to |
|----------|---------|
| `INPUT_SESSION_TYPE` (legacy: `INPUT_TASK_TYPE`) | `session_type` input |
| `INPUT_CLI` or `CODEFORGE_CLI` | `cli` input |
| `INPUT_PROMPT` | `prompt` input |
| `INPUT_MR_IID` or `INPUT_PR_NUMBER` | Manual MR/PR override for non-MR pipelines (GitLab) |
| `ANTHROPIC_API_KEY` | Claude Code API key |
| `OPENAI_API_KEY` | Codex API key |
| `GITHUB_TOKEN` | GitHub provider token |
| `GITLAB_TOKEN` (fallback: `CI_JOB_TOKEN`, clone-only) | GitLab provider token |
| `CI_SERVER_URL` / `GITLAB_URL` / `GITHUB_URL` | Self-managed instance detection (`CI_SERVER_URL` is set automatically by GitLab CI) |
