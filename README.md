<p align="center">
  <img src="assets/logo.png" alt="CodeForge" width="720"/>
</p>

<p align="center">
  <a href="https://github.com/freema/codeforge/actions/workflows/ci.yaml"><img src="https://github.com/freema/codeforge/actions/workflows/ci.yaml/badge.svg" alt="CI"/></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/freema/codeforge" alt="Go"/></a>
  <a href="https://github.com/freema/codeforge/pkgs/container/codeforge"><img src="https://img.shields.io/badge/GHCR-ghcr.io%2Ffreema%2Fcodeforge-blue?logo=github" alt="GHCR"/></a>
  <a href="https://tomasgrasl.cz"><img src="https://img.shields.io/badge/Author-Tom%C3%A1%C5%A1%20Grasl-orange" alt="Author"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"/></a>
</p>

<p align="center">
  <a href="https://codeforge.tomasgrasl.cz"><b>codeforge.tomasgrasl.cz</b></a>
</p>

## Overview

CodeForge is a backend orchestrator for AI-powered code work over git repositories. A **session** is a stateful work unit over a repo: it clones, runs an AI CLI (Claude Code, Codex, Cursor), streams progress live, and keeps the workspace around for follow-ups, review, and PR creation. A React web UI is included.

**Two modes:** Server (queue + workers + API + UI) or **CI Action** (self-contained GitHub Action / GitLab CI step for automated PR review).

### Highlights

- **Multi-turn sessions** with native conversation resume (`claude --resume`), so follow-ups keep full context
- **Live stream** of tool calls, thinking, plan progress, and per-turn token/dollar cost (SSE)
- **Human-in-the-loop**: inspect the actual diff in the UI before a PR is ever opened; PRs only on explicit action
- **Code review built in**: review a session's changes, or let GitHub/GitLab webhooks trigger PR reviews automatically
- **Schedules**: recurring cron sessions with run history, overlap guard, and failure notifications
- **Cost & usage tracking** per session and per tenant, with quotas and a key pool (subscription mode)
- **Multi-CLI** (Claude Code, Codex, Cursor) selectable per session, plus MCP tool integration
- **Ops-friendly**: Redis queue with crash-safe recovery, Prometheus metrics, Slack/Discord/Teams notifications

## Screenshots

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="Dashboard: success rate, cost, sessions" width="900"/>
</p>

| Session with live stream, cost & diff | Recurring schedules |
|---|---|
| ![Session detail](docs/screenshots/session-changes.png) | ![Schedules](docs/screenshots/schedules.png) |

| New session | Session list |
|---|---|
| ![New session](docs/screenshots/new-session.png) | ![Sessions](docs/screenshots/sessions.png) |

## Quick Start

```bash
docker pull ghcr.io/freema/codeforge:latest
```

A ready-to-use compose file is at [`deployments/docker-compose.production.yaml`](deployments/docker-compose.production.yaml). For development:

```bash
# Requires Docker + Task runner (https://taskfile.dev)
task dev
```

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/api.md) | Endpoints, request/response, session types, webhooks |
| [Architecture](docs/architecture.md) | System design, Redis schema, session lifecycle, streaming |
| [Code Review](docs/code-review-workflow.md) | Session review, PR review, webhook-triggered reviews |
| [Configuration](docs/configuration.md) | Environment variables, YAML config, all options |
| [Deployment](docs/deployment.md) | Docker, Kubernetes, monitoring |
| [Development](docs/development.md) | Dev setup, testing, project structure, conventions |
| [Manual E2E Testing](docs/manual-e2e-testing.md) | Manual lifecycle tests against real repos (Claude + Codex) |
| [CI Action](docs/ci-action.md) | GitHub Action / GitLab CI setup, inputs, session types |

## License

[MIT](LICENSE) | **Tomas Grasl** · [tomasgrasl.cz](https://tomasgrasl.cz)
