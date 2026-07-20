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

CodeForge is a backend orchestrator for AI-powered code work over git repositories. A **session** is a stateful work unit over a repo — it clones, runs an AI CLI (Claude Code, Codex, Cursor), streams progress via SSE, and supports multi-turn follow-ups, code review, PR creation, and webhook-triggered PR reviews. A React web UI is included.

**Two modes:** Server (queue + workers + API + UI) or **CI Action** (self-contained GitHub Action / GitLab CI step for automated PR review).

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

[MIT](LICENSE) | **Tomas Grasl** — [tomasgrasl.cz](https://tomasgrasl.cz)
