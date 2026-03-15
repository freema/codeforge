# Configuration

Configuration is loaded in order: **defaults -> YAML file -> environment variables**.

Set `CODEFORGE_CONFIG` to specify a YAML config file path, or use environment variables with the `CODEFORGE_` prefix (double underscore `__` for nesting).

## Environment Variables

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_SERVER__PORT` | `8080` | HTTP server port |
| `CODEFORGE_SERVER__AUTH_TOKEN` | (required) | Bearer token for API auth |

### Redis

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_REDIS__URL` | (required) | Redis connection URL |
| `CODEFORGE_REDIS__PREFIX` | `codeforge:` | Redis key prefix |

### SQLite

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_SQLITE__PATH` | `/data/codeforge.db` | SQLite database file path |

### Encryption

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_ENCRYPTION__KEY` | (required) | Base64-encoded 32-byte AES key |

### Workers

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_WORKERS__CONCURRENCY` | `3` | Number of worker goroutines |
| `CODEFORGE_WORKERS__QUEUE_NAME` | `queue:tasks` | Redis queue name |

### Tasks

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_TASKS__DEFAULT_TIMEOUT` | `300` | Default task timeout (seconds) |
| `CODEFORGE_TASKS__MAX_TIMEOUT` | `1800` | Maximum task timeout (seconds) |
| `CODEFORGE_TASKS__WORKSPACE_BASE` | `/data/workspaces` | Workspace directory |
| `CODEFORGE_TASKS__WORKSPACE_TTL` | `86400` | Workspace TTL (seconds) |
| `CODEFORGE_TASKS__STATE_TTL` | `604800` | Task state TTL (seconds) |
| `CODEFORGE_TASKS__RESULT_TTL` | `604800` | Task result TTL (seconds) |
| `CODEFORGE_TASKS__DISK_WARNING_THRESHOLD_GB` | `10` | Disk usage warning threshold (GB) |
| `CODEFORGE_TASKS__DISK_CRITICAL_THRESHOLD_GB` | `20` | Disk usage critical threshold (GB) |

### CLI

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_CLI__DEFAULT` | `claude-code` | Default CLI tool (`claude-code` or `codex`) |
| `CODEFORGE_CLI__CLAUDE_CODE__PATH` | `claude` | Claude Code binary path |
| `CODEFORGE_CLI__CLAUDE_CODE__DEFAULT_MODEL` | `claude-sonnet-4-20250514` | Default AI model for Claude Code |
| `CODEFORGE_CLI__CODEX__PATH` | `codex` | Codex CLI binary path |
| `CODEFORGE_CLI__CODEX__DEFAULT_MODEL` | *(empty)* | Default AI model for Codex (empty = use Codex built-in default) |

### Git

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_GIT__BRANCH_PREFIX` | `codeforge/` | PR branch prefix |
| `CODEFORGE_GIT__COMMIT_AUTHOR` | `CodeForge Bot` | Git commit author |
| `CODEFORGE_GIT__COMMIT_EMAIL` | `codeforge@noreply` | Git commit email |
| `CODEFORGE_GIT__PROVIDER_DOMAINS` | `{}` | Custom domain->provider mapping (e.g., `{"git.company.com": "gitlab"}`) |

### Webhooks

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_WEBHOOKS__HMAC_SECRET` | | HMAC secret for webhook signatures |
| `CODEFORGE_WEBHOOKS__RETRY_COUNT` | `3` | Webhook retry attempts |
| `CODEFORGE_WEBHOOKS__RETRY_DELAY` | `5s` | Delay between retries |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_RATE_LIMIT__ENABLED` | `true` | Enable rate limiting |
| `CODEFORGE_RATE_LIMIT__TASKS_PER_MINUTE` | `10` | Rate limit per token |

### Workflow

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_WORKFLOW__CONTEXT_TTL_HOURS` | `24` | TTL for workflow context in Redis (hours) |
| `CODEFORGE_WORKFLOW__MAX_RUN_DURATION_SEC` | `7200` | Max workflow run duration (seconds) |

### Code Review (PR Webhooks)

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_CODE_REVIEW__REVIEW_DRAFTS` | `false` | Review draft PRs/MRs from webhooks |
| `CODEFORGE_CODE_REVIEW__DEFAULT_CLI` | `claude-code` | CLI for webhook-triggered reviews |
| `CODEFORGE_CODE_REVIEW__DEFAULT_KEY_NAME` | *(empty)* | Registered key name for git auth (required for webhooks) |
| `CODEFORGE_CODE_REVIEW__WEBHOOK_DEDUP_TTL` | `3600` | Webhook dedup TTL in seconds (prevents duplicate reviews for same commit) |
| `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITHUB` | *(empty)* | HMAC-SHA256 secret for GitHub webhook verification |
| `CODEFORGE_CODE_REVIEW__WEBHOOK_SECRETS__GITLAB` | *(empty)* | Secret token for GitLab webhook verification |

### Tracing

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_TRACING__ENABLED` | `false` | Enable OpenTelemetry tracing |
| `CODEFORGE_TRACING__EXPORTER` | `otlp` | Trace exporter type |
| `CODEFORGE_TRACING__ENDPOINT` | | OTLP collector endpoint |
| `CODEFORGE_TRACING__SAMPLING_RATE` | `0.1` | Trace sampling rate (0-1) |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_LOGGING__LEVEL` | `info` | Log level (debug/info/warn/error) |
| `CODEFORGE_LOGGING__FORMAT` | `json` | Log format (json/text) |

## YAML Configuration

You can also use a YAML config file. The structure mirrors the env var names:

```yaml
server:
  port: 8080
  auth_token: "your-token"

redis:
  url: "redis://localhost:6379"
  prefix: "codeforge:"

sqlite:
  path: "/data/codeforge.db"

encryption:
  key: "base64-encoded-32-byte-key"

workers:
  concurrency: 3

tasks:
  default_timeout: 300
  max_timeout: 1800
  workspace_base: "/data/workspaces"

cli:
  default: "claude-code"
  claude_code:
    path: "claude"
    default_model: "claude-sonnet-4-20250514"
  codex:
    path: "codex"
    default_model: ""   # empty = use Codex CLI's built-in default

git:
  branch_prefix: "codeforge/"
  commit_author: "CodeForge Bot"
  commit_email: "codeforge@noreply"

workflow:
  context_ttl_hours: 24
  max_run_duration_sec: 7200

code_review:
  review_drafts: false
  default_cli: "claude-code"
  default_key_name: "my-github-key"   # required for webhook-triggered reviews
  webhook_dedup_ttl: 3600             # seconds, prevents duplicate reviews for same commit
  webhook_secrets:
    github: "your-github-webhook-secret"
    gitlab: "your-gitlab-webhook-secret"

logging:
  level: "info"
  format: "json"
```

Set the config file path via:
```bash
CODEFORGE_CONFIG=/etc/codeforge/config.yaml
```

## Generating an Encryption Key

```bash
openssl rand -base64 32
```

The key must be exactly 32 bytes (before base64 encoding) for AES-256-GCM.
