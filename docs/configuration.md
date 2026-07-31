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
| `CODEFORGE_WORKERS__QUEUE_NAME` | `queue:sessions` | Redis queue name |

### Sessions

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_SESSIONS__DEFAULT_TIMEOUT` | `300` | Default session timeout (seconds) |
| `CODEFORGE_SESSIONS__MAX_TIMEOUT` | `1800` | Maximum session timeout (seconds) |
| `CODEFORGE_SESSIONS__WORKSPACE_BASE` | `/data/workspaces` | Workspace directory |
| `CODEFORGE_SESSIONS__WORKSPACE_TTL` | `86400` | Workspace TTL (seconds) |
| `CODEFORGE_SESSIONS__STATE_TTL` | `604800` | Session state TTL (seconds) |
| `CODEFORGE_SESSIONS__RESULT_TTL` | `604800` | Session result TTL (seconds) |
| `CODEFORGE_SESSIONS__DISK_WARNING_THRESHOLD_GB` | `10` | Disk usage warning threshold (GB) |
| `CODEFORGE_SESSIONS__DISK_CRITICAL_THRESHOLD_GB` | `20` | Disk usage critical threshold (GB) |

### CLI

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_CLI__DEFAULT` | `claude-code` | Default CLI tool (`claude-code`, `codex`, or `cursor`) |
| `CODEFORGE_CLI__CLAUDE_CODE__PATH` | `claude` | Claude Code binary path |
| `CODEFORGE_CLI__CLAUDE_CODE__DEFAULT_MODEL` | *(empty)* | Default AI model for Claude Code (empty = use CLI built-in default) |
| `CODEFORGE_CLI__CODEX__PATH` | `codex` | Codex CLI binary path |
| `CODEFORGE_CLI__CODEX__DEFAULT_MODEL` | *(empty)* | Default AI model for Codex (empty = use Codex built-in default) |
| `CODEFORGE_CLI__CURSOR__PATH` | `cursor-agent` | Cursor CLI binary path |
| `CODEFORGE_CLI__CURSOR__DEFAULT_MODEL` | *(empty)* | Default AI model for Cursor (empty = use Cursor built-in default) |

Each CLI also has a `models` list (selectable models offered to the UI) — set it via YAML (see below). Defaults: Claude Code ships with the current Sonnet/Opus models, Codex with `gpt-5.2`, `gpt-5.1`, `gpt-5`, `gpt-4.1`, `o3`, `o4-mini`, Cursor with `composer-2`.

### Git

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_GIT__BRANCH_PREFIX` | `codeforge/` | PR branch prefix |
| `CODEFORGE_GIT__COMMIT_AUTHOR` | `CodeForge Bot` | Git commit author |
| `CODEFORGE_GIT__COMMIT_EMAIL` | `codeforge@noreply` | Git commit email |
| `CODEFORGE_GIT__PROVIDER_DOMAINS` | `{}` | Custom domain->provider mapping (e.g., `{"git.company.com": "gitlab"}`) |

#### Self-hosted GitLab / GitHub Enterprise

Provider detection for self-hosted instances works out of the box when a
provider key with a `base_url` exists: the hostname of every stored
`github`/`gitlab` key's `base_url` is merged into the domain mapping at call
time. Creating a key via the API/UI (e.g. provider `gitlab`,
`base_url: https://gitlab.example.com`) is sufficient — clone auth, MR
create/status, and review posting recognize the host immediately, no restart
needed.

`GITLAB_URL`/`GITHUB_URL` env vars and explicit `git.provider_domains`
config remain supported; explicit config/env entries take precedence over
key-derived ones. `provider_domains` entries may also be keyed as
`host:port` to pin an instance on a non-standard port. The scheme and port
of the repository URL are preserved for provider API calls, so plain-`http`
instances and custom ports (e.g. `http://gitlab.example.com:8080`) work.

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
| `CODEFORGE_RATE_LIMIT__SESSIONS_PER_MINUTE` | `10` | Rate limit per token |

### Subscription

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_SUBSCRIPTION__ENABLED` | `false` | Enable the tenant subscription model. When disabled, only the static operator Bearer token is accepted and the per-session API-key (BYOK) flow is unchanged. When enabled, tenant API tokens (`cfk_...`) are also accepted and resolve to managed keys from the key pool. |

### Notifications

Chat notifications for terminal session events. Disabled unless at least one webhook URL is set.

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_NOTIFICATIONS__SLACK_WEBHOOK_URL` | *(empty)* | Slack incoming webhook URL |
| `CODEFORGE_NOTIFICATIONS__DISCORD_WEBHOOK_URL` | *(empty)* | Discord webhook URL (both may be set at once) |
| `CODEFORGE_NOTIFICATIONS__TEAMS_WEBHOOK_URL` | *(empty)* | Microsoft Teams webhook URL — classic incoming webhooks (`webhook.office.com`) get a plain text payload, any other host (e.g. Power Automate / Teams Workflows) gets an Adaptive Card |
| `CODEFORGE_NOTIFICATIONS__UI_BASE_URL` | *(empty)* | Public UI base URL — adds a session link to messages |
| `CODEFORGE_NOTIFICATIONS__EVENTS` | *(empty = all)* | Comma-separated subset of `session_completed`, `session_failed`, `pr_created`, `review_completed`, `schedule_failed` |

`schedule_failed` fires when a recurring schedule fails to create its session (and once more when a schedule is auto-disabled after 5 consecutive failures).

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
| `CODEFORGE_CODE_REVIEW__ALLOW_UNTRUSTED_AUTHORS` | `false` | Review fork PRs/MRs and accept commands from authors without write access |

> The review CLI and model can also be overridden at runtime from the UI
> (Settings → AI providers → Code review) or via `PUT /api/v1/settings/review`.
> The runtime override takes precedence over `code_review.default_cli`; an
> empty override falls back to this config.

#### Who can trigger a webhook review

A valid webhook signature proves the event came from GitHub or GitLab. It says
nothing about who opened the pull request or wrote the comment. Reviewing a PR
means checking out its branch and running an AI CLI over it with approvals
disabled, so the author effectively chooses code that runs on the server.

By default CodeForge therefore requires write access from the author:

- **Fork PRs** are reviewed only when `author_association` is `OWNER`,
  `MEMBER`, or `COLLABORATOR`. `CONTRIBUTOR` is not enough — it only means an
  earlier PR was merged.
- **Comment commands** (`/review`, `/fix`, `/fix-cr`) are dispatched under the
  same rule. `/fix` forwards the rest of the comment as the prompt for a session
  that writes code, so on a public repository this is the most sensitive entry
  point in the system.
- **GitLab** payloads carry no equivalent of `author_association`, so fork MRs
  (source project ≠ target project) are skipped outright and commands are
  refused on them.

Setting `allow_untrusted_authors: true` disables all of the above. Only do that
where each session is genuinely isolated — see
[Deployment](deployment.md#isolation-and-untrusted-code).

### Tracing

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEFORGE_TRACING__ENABLED` | `false` | Enable OpenTelemetry tracing |
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

sessions:
  default_timeout: 300
  max_timeout: 1800
  workspace_base: "/data/workspaces"

cli:
  default: "claude-code"
  claude_code:
    path: "claude"
    default_model: ""   # empty = use Claude Code's built-in default
    models:             # selectable models offered to the UI
      - "claude-sonnet-4-6-20250627"
      - "claude-opus-4-6-20250625"
  codex:
    path: "codex"
    default_model: ""   # empty = use Codex CLI's built-in default
    models:
      - "gpt-5.2"
      - "o3"
  cursor:
    path: "cursor-agent"
    default_model: ""   # empty = use Cursor's built-in default
    models:
      - "composer-2"

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

subscription:
  enabled: false   # tenant subscription model (tenant API tokens + managed key pool)

notifications:
  slack_webhook_url: ""      # Slack incoming webhook for session done/failed/PR/review messages
  discord_webhook_url: ""    # Discord webhook (both may be set at once)
  teams_webhook_url: ""      # Microsoft Teams webhook (classic webhook.office.com or Power Automate workflow URL)
  ui_base_url: ""            # e.g. https://cf.example.com — adds a session link to messages
  events: []                 # empty = all; subset of session_completed, session_failed, pr_created, review_completed, schedule_failed

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
