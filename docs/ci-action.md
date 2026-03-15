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
      - uses: freema/codeforge-action@v1
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
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
      - uses: freema/codeforge-action@v1
        with:
          task_type: knowledge_update
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
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
```

`$ANTHROPIC_API_KEY` must be set as a GitLab CI/CD variable. `$CI_JOB_TOKEN` is automatic.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `task_type` | `pr_review` | `pr_review`, `code_review`, `knowledge_update`, `custom` |
| `prompt` | | Custom prompt (required for `custom`, optional for reviews) |
| `cli` | `claude-code` | AI CLI: `claude-code` or `codex` |
| `model` | | AI model override |
| `api_key` | | AI API key (overrides `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`) |
| `provider_token` | | GitHub/GitLab token (defaults to `$GITHUB_TOKEN` / `$CI_JOB_TOKEN`) |
| `mcp_config` | | MCP config JSON string or path to `.mcp.json` |
| `post_comments` | `true` | Post review as PR/MR comments |
| `output_format` | `json` | Output format: `json`, `markdown`, `text` |
| `max_turns` | | Max AI conversation turns |
| `allowed_tools` | | Comma-separated tool allowlist for Claude Code |

## Outputs

| Output | Description |
|--------|-------------|
| `verdict` | Review verdict: `approve`, `request_changes`, `comment` |
| `score` | Review score (1-10) |
| `issues_count` | Number of issues found |
| `review` | Full review result as JSON |
| `output` | Raw CLI output |

## Task Types

### `pr_review` (default)

Reviews the PR/MR diff. Automatically detects PR number, branches, and commit SHA from the CI environment. Posts review comments if `post_comments=true`.

Exit code: `0` for approve/comment, `1` for request_changes.

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

Provider tokens for PR comments are automatic:
- **GitHub**: `$GITHUB_TOKEN` (per-job, no setup needed)
- **GitLab**: `$CI_JOB_TOKEN` (automatic)

## Docker Image

```bash
docker pull ghcr.io/freema/codeforge-action:latest
```

~130 MB base image (Alpine + git + Node.js). The selected CLI is installed at runtime via npm (~30s, negligible vs 2-5 min AI execution).

Build locally:

```bash
task build:action
```

## Environment Variables

The CI Action reads configuration from `INPUT_*` environment variables (set automatically by GitHub Actions from `with:` inputs). For GitLab CI or standalone use, set variables directly:

| Variable | Maps to |
|----------|---------|
| `INPUT_TASK_TYPE` | `task_type` input |
| `INPUT_CLI` or `CODEFORGE_CLI` | `cli` input |
| `INPUT_PROMPT` | `prompt` input |
| `ANTHROPIC_API_KEY` | Claude Code API key |
| `OPENAI_API_KEY` | Codex API key |
| `GITHUB_TOKEN` | GitHub provider token |
| `GITLAB_TOKEN` or `CI_JOB_TOKEN` | GitLab provider token |
