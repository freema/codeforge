# CodeForge

[![CI](https://github.com/freema/codeforge/actions/workflows/ci.yaml/badge.svg)](https://github.com/freema/codeforge/actions/workflows/ci.yaml)
[![Go](https://img.shields.io/github/go-mod/go-version/freema/codeforge)](https://go.dev/)
[![Author](https://img.shields.io/badge/Author-Tom%C3%A1%C5%A1%20Grasl-orange)](https://tomasgrasl.cz)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Remote AI code task runner. Submit tasks via HTTP, stream progress through Redis, get results back.

## Overview

```
Client                  CodeForge                          AI CLI
  │                        │                                 │
  │  POST /api/v1/tasks    │                                 │
  ├───────────────────────▶│                                 │
  │         201 {id}       │                                 │
  │◀───────────────────────┤                                 │
  │                        │  git clone ──▶ run CLI          │
  │                        ├────────────────────────────────▶│
  │    Redis Pub/Sub       │         stream-json events      │
  │◀ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─┤◀────────────────────────────────┤
  │                        │                                 │
  │  GET /tasks/{id}       │           result + diff         │
  ├───────────────────────▶│◀────────────────────────────────┤
  │     200 {result}       │                                 │
  │◀───────────────────────┤                                 │
  │                        │                                 │
  │  POST /tasks/{id}/     │                                 │
  │       create-pr        │  git push ──▶ GitHub/GitLab API │
  ├───────────────────────▶├─────────────────────────────────▶
  │     200 {pr_url}       │                                 │
  │◀───────────────────────┤                                 │
```

CodeForge receives task requests via REST API, clones the repository, runs an AI CLI tool (Claude Code) against it, streams progress via Redis Pub/Sub, and optionally creates pull requests. It supports multi-turn conversations, webhook callbacks, and workspace lifecycle management.

## Quick Start

```bash
# Start the server (requires Docker + docker-compose)
docker compose -f deployments/docker-compose.yaml -f deployments/docker-compose.dev.yaml up --build

# Create a task
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo.git",
    "access_token": "ghp_your_token",
    "prompt": "Fix the failing tests in the auth module"
  }'

# Check status
curl http://localhost:8080/api/v1/tasks/{id} \
  -H "Authorization: Bearer dev-token"
```

If you have [Task](https://taskfile.dev/) installed, just run `task dev` instead of the docker compose command.

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/api.md) | Endpoints, request/response examples, webhook format |
| [Architecture](docs/architecture.md) | System design, Redis schema, task lifecycle |
| [Configuration](docs/configuration.md) | Environment variables, YAML config |
| [Deployment](docs/deployment.md) | Docker, Kubernetes, monitoring |
| [Development](docs/development.md) | Dev setup, testing, project structure |

## Roadmap

- [ ] **Multi-CLI Support** — Runners for OpenCode, Codex, and other AI coding CLIs alongside Claude Code
- [ ] **Task Sessions** — Cross-task memory for projects; remember context from previous tasks on the same repository
- [ ] **Code Review** — Automated review of changes by a separate model before creating a pull request
- [ ] **Enhanced PR Messages** — Richer pull request descriptions with structured sections and conventional commits
- [ ] **Subscription & Multi-User Auth** — Per-user API keys, usage tracking, and subscription management

## Author

**Tomáš Grasl** — [tomasgrasl.cz](https://tomasgrasl.cz)

## License

[MIT](LICENSE)
