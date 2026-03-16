<p align="center">
  <img src="assets/logo.svg" alt="CodeForge" width="720"/>
</p>

<p align="center">
  <a href="https://github.com/freema/codeforge/actions/workflows/ci.yaml"><img src="https://github.com/freema/codeforge/actions/workflows/ci.yaml/badge.svg" alt="CI"/></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/freema/codeforge" alt="Go"/></a>
  <a href="https://github.com/freema/codeforge/pkgs/container/codeforge"><img src="https://img.shields.io/badge/GHCR-ghcr.io%2Ffreema%2Fcodeforge-blue?logo=github" alt="GHCR"/></a>
  <a href="https://tomasgrasl.cz"><img src="https://img.shields.io/badge/Author-Tom%C3%A1%C5%A1%20Grasl-orange" alt="Author"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"/></a>
</p>

## Overview

CodeForge is a backend orchestrator for AI-powered code work over git repositories. A task is a **session over a repo** — it clones, runs an AI CLI (Claude Code, Codex), streams progress via SSE, and supports multi-turn follow-ups, code review, PR creation, and webhook-triggered PR reviews.

**Two modes:** Server (queue + workers + API) or **CI Action** (self-contained GitHub Action / GitLab CI step for automated PR review).

```
Client ──▶ POST /tasks ──▶ Queue ──▶ Worker (clone → CLI → result)
       ◀── SSE stream ◀──────────────────────────────────────────┘
       ──▶ POST /tasks/{id}/instruct | review | create-pr
```

## Quick Start

```bash
docker pull ghcr.io/freema/codeforge:latest
```

A ready-to-use compose file is at [`deployments/docker-compose.production.yaml`](deployments/docker-compose.production.yaml). For development:

```bash
# Requires Docker + Task runner (https://taskfile.dev)
task dev
```

```bash
# Create a task
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"repo_url": "https://github.com/user/repo.git", "access_token": "ghp_...", "prompt": "Fix the failing tests"}'
```

See [API Reference](docs/api.md) for all endpoints and examples.

## CI Action (GitHub Actions / GitLab CI)

Add automated AI code review to any repository — **1 secret, 12 lines of YAML:**

```yaml
# .github/workflows/review.yml
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
      - uses: freema/codeforge@main
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

**Task types:** `pr_review` (default), `code_review`, `knowledge_update`, `custom`
**CLIs:** Claude Code, Codex | **Platforms:** GitHub Actions, GitLab CI

See [CI Action docs](docs/ci-action.md) for full configuration.

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/api.md) | Endpoints, request/response, task types, webhooks |
| [Architecture](docs/architecture.md) | System design, Redis schema, task lifecycle, streaming |
| [Code Review](docs/code-review-workflow.md) | Task review, PR review, webhook-triggered reviews |
| [Configuration](docs/configuration.md) | Environment variables, YAML config, all options |
| [Deployment](docs/deployment.md) | Docker, Kubernetes, monitoring |
| [Development](docs/development.md) | Dev setup, testing, project structure, conventions |
| [Manual E2E Testing](docs/manual-e2e-testing.md) | Manual lifecycle tests against real repos (Claude + Codex) |
| [CI Action](docs/ci-action.md) | GitHub Action / GitLab CI setup, inputs, task types |

## Roadmap

- [x] Multi-step workflows (fetch → task → action)
- [x] Multi-CLI support (Claude Code + Codex)
- [x] Code review as action (`POST /tasks/:id/review`)
- [x] Task types (code, plan, review, pr_review)
- [x] Automated PR review (webhooks + comment posting)
- [x] **CI Action** — self-contained GitHub Action / GitLab CI step (multi-CLI, tools, review posting)
- [ ] Cross-task memory (project context across tasks)
- [ ] Enhanced PR descriptions
- [ ] Multi-user auth + usage tracking

## License

[MIT](LICENSE) | **Tomas Grasl** — [tomasgrasl.cz](https://tomasgrasl.cz)
