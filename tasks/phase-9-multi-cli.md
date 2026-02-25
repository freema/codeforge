# Phase 9: Multi-CLI Support

## Cíl

Rozšířit CLI registry o podporu dalších AI coding nástrojů — **OpenCode**, **Codex CLI**, **Aider**, a dalších. Každý CLI runner implementuje stejný `Runner` interface.

## Závislosti

- Žádné přímé závislosti (existující `cli.Registry` je připravený)

## Současný stav

```go
// internal/cli/runner.go
type Runner interface {
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}

// internal/cli/registry.go
type Registry struct {
    runners    map[string]Runner
    defaultCLI string  // "claude-code"
}
```

Registry pattern je již implementován, ale existuje pouze `ClaudeRunner`. Potřebujeme:
1. Nové runner implementace
2. Unifikaci stream formátu (každý CLI má jiný output)
3. Per-task CLI selection
4. Docker: instalace dalších CLI nástrojů

## CLI Runners

### 9.1 — OpenCode Runner

**OpenCode** (https://opencode.ai) — open-source AI coding agent napsaný v Go.

#### Instalace
```bash
# Install script (doporučeno)
curl -sL opencode.ai/install | bash

# Homebrew
brew install opencode

# npm (wrapper kolem Go binary)
npm install -g opencode-ai
```

**npm package:** `opencode-ai` (NE `opencode`)
**Aktuální verze:** 1.2.5 (únor 2026)
**Repository:** [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode)

#### Non-interactive mód
```bash
opencode run "Your prompt here"
opencode run --format json "Your prompt here"    # JSON output
opencode run --model anthropic/claude-sonnet-4-20250514 "prompt"
opencode run --file src/auth.ts "Review this code"
opencode run --continue "Follow-up instruction"
opencode run --attach http://localhost:4096 "prompt"  # attach k running serve
```

#### Klíčové CLI flagy

| Flag | Popis |
|------|-------|
| `--model` / `-m` | Model ve formátu `provider/model` |
| `--format` / `-f` | Output formát: `json` nebo `text` |
| `--agent` | Specifický agent (např. `plan`, `code-reviewer`) |
| `--file` | Přiložit soubor k promptu |
| `--continue` | Pokračovat z poslední session |
| `--attach` | Připojit se k running `opencode serve` instanci |
| `-d` | Debug logging |
| `-c /path` | Working directory |
| `-q` / `--quiet` | Potlačit spinner |
| `--dangerously-skip-permissions` | Přeskočit všechny permission prompty |

#### Headless server mód
```bash
opencode serve                    # HTTP server (API přístup)
opencode serve --port 4096        # Custom port
opencode acp                      # ACP server (stdin/stdout, nd-JSON)
```

#### Output formát
- **text** (default) — plain text response
- **json** (`--format json`) — JSON-formatted output
- **stream-json** — nd-JSON streaming (via SDK)
- **ACP** (`opencode acp`) — nd-JSON přes stdin/stdout, event typy: `agent_message_chunk`, `requestPermission`, `sessionUpdate`

#### Permissions
V `opencode.json`:
```json
{
  "permission": {
    "edit": "allow",
    "bash": "ask",
    "webfetch": "deny"
  }
}
```

#### API klíče
```bash
export ANTHROPIC_API_KEY=sk-ant-xxx
export OPENAI_API_KEY=sk-xxx
# nebo přes opencode auth login
```

Konfigurace v `opencode.json`:
```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-20250514",
  "provider": { ... }
}
```

#### CodeForge Runner implementace

```go
// internal/cli/opencode.go

type OpenCodeRunner struct {
    binaryPath string  // "opencode"
}

func (r *OpenCodeRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    args := []string{"run"}
    if opts.OutputFormat == "json" {
        args = append(args, "--format", "json")
    }
    args = append(args, "--model", opts.Model)
    args = append(args, "-q")  // quiet mode pro scripting
    args = append(args, "--dangerously-skip-permissions")
    args = append(args, opts.Prompt)
    // exec.CommandContext(ctx, r.binaryPath, args...)
}
```

#### Srovnání s Claude Code

| Feature | Claude Code | OpenCode |
|---------|-------------|----------|
| Non-interactive | `claude -p "prompt"` | `opencode run "prompt"` |
| JSON output | `--output-format stream-json` | `--format json` (nebo `stream-json` via SDK) |
| Skip permissions | `--permission-mode bypassPermissions` | `--dangerously-skip-permissions` |
| Max turns | `--max-turns N` | **Není k dispozici** |
| Config file | `CLAUDE.md` | `opencode.json` |
| Install | `npm install -g @anthropic-ai/claude-code` | `npm install -g opencode-ai` nebo install script |

**Tasky:**
- [x] 9.1.1 — Prozkoumat OpenCode CLI rozhraní (argumenty, output formát, stream mode)
- [ ] 9.1.2 — Implementovat `OpenCodeRunner`
- [ ] 9.1.3 — Stream normalizace: OpenCode JSON output → unified `StreamEvent`
- [ ] 9.1.4 — Docker: přidat OpenCode do Dockerfile (`npm install -g opencode-ai` nebo install script)
- [ ] 9.1.5 — Konfigurace: `cli.opencode.path`, `cli.opencode.default_model`
- [ ] 9.1.6 — Unit testy + integration test

### 9.2 — Codex CLI Runner

**Codex** (OpenAI) — AI coding assistant CLI, přepsaný z TypeScript do **Rust** (červen 2025).

#### Instalace
```bash
# npm (doporučeno — wrapper kolem Rust binary)
npm install -g @openai/codex

# Homebrew (macOS)
brew install --cask codex

# GitHub Releases (platform-specific binary)
# https://github.com/openai/codex/releases
```

**npm package:** `@openai/codex`
**SDK:** `@openai/codex-sdk` (v0.101.0)
**Repository:** [github.com/openai/codex](https://github.com/openai/codex)
**Flagship model:** GPT-5.3-Codex

#### Non-interactive mód (`codex exec`)
```bash
codex exec "Generate a unit test for the User class"
codex exec --json "List all TODO comments"                    # JSONL streaming output
codex exec --output-schema schema.json "Extract endpoints"    # Structured JSON output
codex exec --last-message-file result.txt "Summarize README"  # Uložit finální zprávu
echo "Explain this error" | codex exec -                      # Prompt ze stdin
codex exec resume --last                                      # Pokračovat poslední session
```

#### Klíčové CLI flagy

| Flag | Popis |
|------|-------|
| `-m <model>` | Override model (např. `gpt-4o`, `o4-mini`) |
| `--json` | JSONL stream na stdout |
| `--output-schema <file>` | Structured JSON output dle schématu |
| `--last-message-file <file>` | Uložit finální zprávu do souboru |
| `--full-auto` | Alias: `--sandbox workspace-write --ask-for-approval on-request` |
| `-a <policy>` | Approval policy: `always`, `on-request`, `never` |
| `--sandbox <mode>` | `read-only`, `workspace-write`, `danger-full-access` |
| `-c key=value` | Override jakékoliv config hodnoty |
| `-C <path>` | Working directory |
| `-i <image>` | Multimodal input (obrázek) |
| `--search` | Povolit web search |
| `--skip-git-repo-check` | Přeskočit kontrolu git repo |

#### Output formát

**Bez flagů:**
- stderr: streaming progress
- stdout: finální text zpráva

**S `--json`:**
- stdout = **JSONL stream** (jeden JSON objekt na řádek, jeden na state change)
- Event typy: `thread.started`, `turn.started`, `turn.completed`, `turn.failed`, `item.*`, `error`

```bash
codex exec --json "summarize the repo" | jq
```

#### Sandbox a Permissions

| Sandbox | Popis |
|---------|-------|
| `read-only` | Default, agent může jen číst |
| `workspace-write` | Čtení + zápis v working directory |
| `danger-full-access` | Bez sandboxu (rizikové) |

| Approval | Popis |
|----------|-------|
| `always` | Ptá se na vše (nejrestriktivnější) |
| `on-request` | Ptá se jen když agent požaduje |
| `never` | Nikdy se neptá (plně autonomní) |

**`--full-auto`** = `--sandbox workspace-write --ask-for-approval on-request`

#### API klíče
```bash
export OPENAI_API_KEY=sk-xxx          # Standardní
export CODEX_API_KEY=sk-xxx           # Alternativní (CI)
export OPENAI_BASE_URL=https://...    # Custom endpoint
```

#### Konfigurace (`~/.codex/config.toml`)
```toml
model = "o4-mini"
model_provider = "openai"

[permissions]
approval_policy = "on-request"
sandbox_mode = "workspace-write"

[mcp_servers.filesystem]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/path"]
enabled = true
```

Instructions file: `AGENTS.md` (analogie k `CLAUDE.md`)

#### CodeForge Runner implementace

```go
// internal/cli/codex.go

type CodexRunner struct {
    binaryPath string  // "codex"
}

func (r *CodexRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    args := []string{"exec", "--json"}
    if opts.Model != "" {
        args = append(args, "-m", opts.Model)
    }
    // Full auto pro headless execution
    args = append(args, "--full-auto")
    args = append(args, "--skip-git-repo-check")
    args = append(args, opts.Prompt)
    // exec.CommandContext(ctx, r.binaryPath, args...)
    // Parse JSONL stream: thread.started → turn.started → item.* → turn.completed
}
```

#### Srovnání s Claude Code

| Feature | Claude Code | Codex CLI |
|---------|-------------|-----------|
| Non-interactive | `claude -p "prompt"` | `codex exec "prompt"` |
| Streaming | `--output-format stream-json` | `--json` (JSONL stdout) |
| Permission bypass | `--permission-mode bypassPermissions` | `--full-auto` nebo `-a never --sandbox danger-full-access` |
| API key env | `ANTHROPIC_API_KEY` | `OPENAI_API_KEY` / `CODEX_API_KEY` |
| Config file | `CLAUDE.md` | `AGENTS.md` + `~/.codex/config.toml` |
| MCP support | `.mcp.json` | `[mcp_servers.*]` v config.toml |
| Stdin prompt | `echo "x" \| claude -p -` | `echo "x" \| codex exec -` |

**Tasky:**
- [x] 9.2.1 — Prozkoumat Codex CLI rozhraní (exec, --json, sandbox, JSONL events)
- [ ] 9.2.2 — Implementovat `CodexRunner`
- [ ] 9.2.3 — Stream normalizace: JSONL events (`thread.*`, `turn.*`, `item.*`) → unified `StreamEvent`
- [ ] 9.2.4 — Docker: přidat Codex do Dockerfile (`npm install -g @openai/codex`)
- [ ] 9.2.5 — API key management (`OPENAI_API_KEY` / `CODEX_API_KEY`)
- [ ] 9.2.6 — Unit testy + integration test

### 9.3 — Aider Runner

**Aider** (https://aider.chat) — AI pair programming v terminálu, napsaný v Pythonu.

#### Instalace
```bash
# Doporučeno: UV-based install (nejrychlejší)
pip install aider-install && aider-install

# Alternativně přímý UV install
uv tool install --force --python python3.12 --with pip aider-chat@latest

# pipx (izolované prostředí)
pipx install aider-chat

# pip (tradiční, může mít dependency konflikty)
pip install aider-chat
```

**PyPI package:** `aider-chat` (NE `aider`)
**Aktuální verze:** 0.86.1+ (aktivní vývoj)
**Python requirement:** 3.9-3.12 (NE 3.13+)
**Repository:** [github.com/Aider-AI/aider](https://github.com/Aider-AI/aider)

#### Non-interactive mód
```bash
aider --message "Add type hints to all functions" utils.py
aider --message-file instructions.txt utils.py
```

`--message` / `-m` pošle prompt, zpracuje AI odpověď, aplikuje edity a **automaticky ukončí** (žádná interaktivní session).

#### Klíčové CLI flagy pro automatizaci

| Flag | Env Var | Popis |
|------|---------|-------|
| `--message MSG` / `-m` | `AIDER_MESSAGE` | Prompt, pak exit |
| `--message-file FILE` / `-f` | `AIDER_MESSAGE_FILE` | Prompt ze souboru, pak exit |
| `--yes` | `AIDER_YES` | Potvrzení na vše |
| `--yes-always` | `AIDER_YES_ALWAYS` | Potvrzení i na nebezpečné akce |
| `--no-git` | `AIDER_NO_GIT` | Vypnout veškeré git operace |
| `--no-auto-commits` | `AIDER_AUTO_COMMITS=false` | Bez auto-commit po editech |
| `--no-stream` | `AIDER_STREAM=false` | Čekat na kompletní odpověď |
| `--stream` | `AIDER_STREAM=true` | Streaming (default) |
| `--no-pretty` | `AIDER_PRETTY=false` | Bez ANSI colors |
| `--no-fancy-input` | `AIDER_FANCY_INPUT=false` | Bez readline (headless) |
| `--no-suggest-shell-commands` | — | Nenavrhovat shell příkazy |
| `--no-detect-urls` | — | Nevyhledávat URL |
| `--no-check-update` | — | Nekontrolovat verze |
| `--verbose` | `AIDER_VERBOSE` | Verbose output |
| `--file FILE` | — | Soubor k editaci (vícekrát) |
| `--read FILE` | — | Soubor jen pro čtení (kontext) |
| `--lint-cmd CMD` | `AIDER_LINT_CMD` | Lint příkaz po editech |
| `--auto-lint` | `AIDER_AUTO_LINT` | Auto lint po každém editu |
| `--test-cmd CMD` | `AIDER_TEST_CMD` | Test příkaz po editech |
| `--auto-test` | `AIDER_AUTO_TEST` | Auto test po každém editu |
| `--dry-run` | `AIDER_DRY_RUN` | Dry run bez aplikace |

#### Model a API klíče
```bash
# Univerzální --model flag (doporučeno)
aider --model sonnet "prompt"
aider --model gpt-4o "prompt"
aider --model deepseek "prompt"
aider --model openrouter/anthropic/claude-3.7-sonnet "prompt"

# Architect mód (dva modely)
aider --architect --model sonnet --editor-model gpt-4o "prompt"

# API klíče přes --api-key
aider --api-key anthropic=sk-ant-xxx --model sonnet "prompt"
aider --api-key openai=sk-xxx --model gpt-4o "prompt"

# Nebo přes env vars
export ANTHROPIC_API_KEY=sk-ant-xxx
export OPENAI_API_KEY=sk-xxx
export DEEPSEEK_API_KEY=xxx
```

#### Output formát

**KRITICKÁ DIFERENCIACE: Aider NEMÁ strukturovaný JSON output.**

- stdout/stderr: **plain text** (human-readable terminálový output)
- Streaming je default (`--stream`), lze vypnout (`--no-stream`)
- **Žádný `--output-format` flag** — žádný `stream-json` ani strukturovaný JSON
- Pro automatizaci nutné parsovat plain text output

Edit formáty (interní, pro komunikaci s LLM): `whole`, `diff`, `udiff`, `editor-diff`

#### Konfigurace
Hierarchie priorit: CLI flags > env vars > `.aider.conf.yml` > defaults

`.aider.conf.yml` v root repo:
```yaml
model: sonnet
auto-commits: false
yes: true
no-git: true
```

`.env` soubor:
```
ANTHROPIC_API_KEY=sk-ant-xxx
```

#### CodeForge Runner implementace

```go
// internal/cli/aider.go

type AiderRunner struct {
    binaryPath string  // "aider"
}

func (r *AiderRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    args := []string{
        "--message", opts.Prompt,
        "--yes",                       // auto-confirm
        "--no-git",                    // CodeForge manages git
        "--no-stream",                 // wait for complete response (easier parsing)
        "--no-suggest-shell-commands", // headless
        "--no-detect-urls",
        "--no-check-update",
        "--no-fancy-input",
        "--no-pretty",                 // clean output bez ANSI
    }
    if opts.Model != "" {
        args = append(args, "--model", opts.Model)
    }
    if opts.APIKey != "" {
        args = append(args, "--api-key", opts.Provider+"="+opts.APIKey)
    }
    for _, f := range opts.Files {
        args = append(args, "--file", f)
    }
    // exec.CommandContext(ctx, r.binaryPath, args...)
    // Capture stdout as plain text, parse manually
}
```

#### Srovnání s Claude Code

| Feature | Claude Code | Aider |
|---------|-------------|-------|
| Non-interactive | `claude -p "prompt"` | `aider --message "prompt"` |
| Output formát | `--output-format stream-json` (JSON) | **Plain text only** (žádný JSON) |
| Skip permissions | `--permission-mode bypassPermissions` | `--yes` / `--yes-always` |
| Git control | N/A (external) | `--no-git`, `--no-auto-commits` |
| Model | `--model` (Anthropic only) | `--model <any>` + `--api-key provider=key` |
| Streaming | JSON events na stdout | Raw text streaming |
| Install | npm | pip/pipx/uv (Python 3.9-3.12) |
| Runtime | Node.js | Python |

**Hlavní challenge:** Aider nemá JSON streaming → normalizace vyžaduje text parsing.

**Tasky:**
- [x] 9.3.1 — Prozkoumat Aider CLI rozhraní (--yes, --no-git, output formát, model selection)
- [ ] 9.3.2 — Implementovat `AiderRunner`
- [ ] 9.3.3 — Stream normalizace: **plain text parsing** → unified `StreamEvent` (obtížnější než u ostatních CLI)
- [ ] 9.3.4 — Docker: přidat Aider (Python) do Dockerfile (`pip install aider-chat` nebo `uv tool install`)
- [ ] 9.3.5 — Unit testy + integration test

## Unifikovaný Stream

Každý CLI tool má jiný output formát. Potřebujeme normalizační vrstvu:

### Přehled output formátů per CLI

| CLI | Formát | Popis |
|-----|--------|-------|
| Claude Code | stream-json (NDJSON) | `{"type":"assistant","message":...}`, `{"type":"result",...}` |
| OpenCode | JSON (`-f json`) nebo ACP (NDJSON) | JSON response nebo nd-JSON events (`agent_message_chunk`) |
| Codex | JSONL (`--json`) | `thread.started`, `turn.started`, `turn.completed`, `item.*` events |
| Aider | **Plain text** | Žádný JSON — nutné parsovat text output |

### Normalizační interface

```go
// internal/cli/stream_normalizer.go

type NormalizedEvent struct {
    Type    string // "thinking", "text", "tool_use", "tool_result", "result", "error"
    Content string
    Raw     json.RawMessage  // originální event
}

type StreamNormalizer interface {
    // Parsuje řádek outputu a vrátí normalizovaný event (nebo nil pro skip)
    Normalize(line []byte) (*NormalizedEvent, error)
}

// Implementace per-CLI:
type ClaudeStreamNormalizer struct{}   // stream-json: {"type":"assistant",...} → NormalizedEvent
type OpenCodeStreamNormalizer struct{} // JSON output: celý JSON response → NormalizedEvent{Type:"result"}
type CodexStreamNormalizer struct{}    // JSONL: turn.completed → NormalizedEvent{Type:"result"}, item.* → text/tool_use
type AiderStreamNormalizer struct{}    // Plain text: regex/heuristic parsing → NormalizedEvent{Type:"text"}
```

### Mapování Codex JSONL events → NormalizedEvent

| Codex Event | → NormalizedEvent.Type |
|-------------|----------------------|
| `thread.started` | (skip — internal) |
| `turn.started` | `"thinking"` |
| `item.message` (text) | `"text"` |
| `item.command_execution` | `"tool_use"` |
| `item.file_change` | `"tool_result"` |
| `turn.completed` | `"result"` |
| `turn.failed` | `"error"` |
| `error` | `"error"` |

### Aider text parsing strategie

Aider nemá JSON output, takže normalizace vyžaduje heuristiku:
1. **Celý stdout** po ukončení = `NormalizedEvent{Type: "result", Content: stdout}`
2. Pokud `--stream` je zapnutý, line-by-line buffering s akumulací do jednoho `"text"` eventu
3. Exit code > 0 → `NormalizedEvent{Type: "error"}`

**Tasky:**
- [ ] 9.4.1 — Definovat `NormalizedEvent` a `StreamNormalizer` interface
- [ ] 9.4.2 — Implementovat `ClaudeStreamNormalizer` (refaktor z `claude.go`)
- [ ] 9.4.3 — Upravit `Streamer.EmitCLIOutput()` aby pracoval s `NormalizedEvent`
- [ ] 9.4.4 — Unit testy normalizace

## Per-Task CLI Selection

```json
POST /api/v1/tasks
{
  "repo_url": "...",
  "prompt": "...",
  "config": {
    "cli": "codex",           // výběr CLI (default: claude-code)
    "ai_model": "gpt-4o",    // model závisí na CLI
    "ai_api_key": "sk-..."   // API key pro daný provider
  }
}
```

**Tasky:**
- [ ] 9.5.1 — Rozšířit `TaskConfig.CLI` validaci o nové CLI typy
- [ ] 9.5.2 — Validace: CLI + model kompatibilita (claude model nelze s codex CLI)
- [ ] 9.5.3 — Validace: správný API key typ pro daný CLI
- [ ] 9.5.4 — API docs: seznam podporovaných CLI a jejich modelů

## Docker Multi-CLI

```dockerfile
# Dockerfile — multi-CLI support

# Claude Code (npm, Node.js)
ARG CLAUDE_CODE_VERSION=latest
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}

# OpenCode (npm wrapper kolem Go binary)
ARG OPENCODE_VERSION=latest
RUN npm install -g opencode-ai@${OPENCODE_VERSION}
# Alternativně: curl -sL opencode.ai/install | bash

# Codex (npm wrapper kolem Rust binary)
ARG CODEX_VERSION=latest
RUN npm install -g @openai/codex@${CODEX_VERSION}
# Alternativně: brew install --cask codex

# Aider (Python, vyžaduje Python 3.9-3.12)
ARG AIDER_VERSION=latest
RUN pip install aider-chat==${AIDER_VERSION}
# Doporučeno pro izolaci: pip install uv && uv tool install aider-chat@${AIDER_VERSION}
```

**Poznámky k Docker image:**
- OpenCode i Codex jsou npm wrappers kolem nativních binárních souborů (Go/Rust) — npm install stáhne správný platform binary
- Aider vyžaduje Python 3.9-3.12 v kontejneru
- Pro minimalizaci image: instalovat jen potřebné CLI (dle konfigurace)

**Tasky:**
- [ ] 9.6.1 — Dockerfile: multi-CLI instalace s version pinning
- [ ] 9.6.2 — Konfigurace: `cli.{name}.binary_path` pro každý CLI
- [ ] 9.6.3 — Health check: ověřit dostupnost binárky při startu
- [ ] 9.6.4 — API endpoint: `GET /api/v1/cli` — seznam dostupných CLI + verze

## Konfigurace

```yaml
# codeforge.yaml
cli:
  default: claude-code
  claude_code:
    path: claude
    default_model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY
  opencode:
    path: opencode
    default_model: anthropic/claude-sonnet-4-20250514   # formát: provider/model
    api_key_env: ANTHROPIC_API_KEY                       # nebo OPENAI_API_KEY
  codex:
    path: codex
    default_model: o4-mini                              # OpenAI modely
    api_key_env: OPENAI_API_KEY                          # nebo CODEX_API_KEY
  aider:
    path: aider
    default_model: sonnet                               # aider model aliasy
    api_key_env: ANTHROPIC_API_KEY                       # univerzální --api-key flag
```

### Env vars per CLI

| CLI | API Key Env | Alternativy |
|-----|------------|-------------|
| Claude Code | `ANTHROPIC_API_KEY` | — |
| OpenCode | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` | Závisí na modelu/provideru |
| Codex | `OPENAI_API_KEY` | `CODEX_API_KEY` (CI), `OPENAI_BASE_URL` |
| Aider | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` | `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY` |

## Testovací strategie

### Interface-first design (testovatelnost)

Každý runner implementuje `cli.Runner` interface. Testy mohou používat mock runner bez reálného CLI:

```go
// Mock runner pro testy:
type mockRunner struct {
    result  *RunResult
    err     error
    events  []json.RawMessage
}

func (m *mockRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    for _, e := range m.events {
        if opts.OnEvent != nil { opts.OnEvent(e) }
    }
    return m.result, m.err
}
```

### Unit testy per runner

**`internal/cli/opencode_test.go`:**
- [ ] `TestOpenCodeRunner_BuildArgs` — ověř: `run --format json --model provider/model -q --dangerously-skip-permissions "prompt"`
- [ ] `TestOpenCodeRunner_BuildArgs_Minimal` — jen povinné: `run -q "prompt"`
- [ ] `TestOpenCodeRunner_ParseOutput` — JSON output → RunResult
- [ ] `TestOpenCodeRunner_ContextCancellation` — cancel ctx → graceful stop

**`internal/cli/codex_test.go`:**
- [ ] `TestCodexRunner_BuildArgs` — ověř: `exec --json --full-auto --skip-git-repo-check -m model "prompt"`
- [ ] `TestCodexRunner_APIKeyEnv` — OPENAI_API_KEY / CODEX_API_KEY (ne ANTHROPIC_API_KEY)
- [ ] `TestCodexRunner_ParseOutput` — JSONL events → RunResult (turn.completed → result)

**`internal/cli/aider_test.go`:**
- [ ] `TestAiderRunner_BuildArgs` — ověř: `--message "prompt" --yes --no-git --no-stream --no-pretty --no-fancy-input --model X --api-key provider=key`
- [ ] `TestAiderRunner_ParseOutput` — plain text stdout → RunResult
- [ ] `TestAiderRunner_ErrorHandling` — non-zero exit → error

### Stream normalizer testy

**`internal/cli/stream_normalizer_test.go`:**
- [ ] `TestClaudeNormalizer_ResultEvent` — stream-json result event → NormalizedEvent{Type: "result"}
- [ ] `TestClaudeNormalizer_AssistantEvent` — text event → NormalizedEvent{Type: "text"}
- [ ] `TestClaudeNormalizer_InvalidJSON` — invalid JSON line → error
- [ ] `TestClaudeNormalizer_EmptyLine` — empty line → nil (skip)
- [ ] `TestOpenCodeNormalizer_JSONResult` — JSON format output → NormalizedEvent{Type: "result"}
- [ ] `TestOpenCodeNormalizer_ACPChunk` — ACP agent_message_chunk → NormalizedEvent{Type: "text"}
- [ ] `TestCodexNormalizer_TurnCompleted` — JSONL `turn.completed` → NormalizedEvent{Type: "result"}
- [ ] `TestCodexNormalizer_ItemMessage` — JSONL `item.message` → NormalizedEvent{Type: "text"}
- [ ] `TestCodexNormalizer_TurnFailed` — JSONL `turn.failed` → NormalizedEvent{Type: "error"}
- [ ] `TestCodexNormalizer_CommandExecution` — JSONL `item.command_execution` → NormalizedEvent{Type: "tool_use"}
- [ ] `TestAiderNormalizer_PlainTextResult` — celý stdout → NormalizedEvent{Type: "result", Content: text}
- [ ] `TestAiderNormalizer_ErrorExit` — non-zero exit code → NormalizedEvent{Type: "error"}

```go
func TestClaudeNormalizer_ResultEvent(t *testing.T) {
    n := &ClaudeStreamNormalizer{}
    line := []byte(`{"type":"result","result":"Hello","usage":{"input":100,"output":50}}`)
    ev, err := n.Normalize(line)
    if err != nil { t.Fatalf("Normalize: %v", err) }
    if ev == nil { t.Fatal("expected event, got nil") }
    if ev.Type != "result" { t.Errorf("Type = %q, want %q", ev.Type, "result") }
    if ev.Content != "Hello" { t.Errorf("Content = %q, want %q", ev.Content, "Hello") }
}
```

### Registry testy

**`internal/cli/registry_test.go`:**
- [ ] `TestRegistry_GetDefault` — prázdný name → default runner
- [ ] `TestRegistry_GetByName` — explicit name → správný runner
- [ ] `TestRegistry_GetUnknown` — neznámý name → error
- [ ] `TestRegistry_Available` — seznam všech registrovaných runners
- [ ] `TestRegistry_RegisterMultiple` — registrace více runners

### Mock CLI rozšíření

**`tests/mockcli/`** — rozšířit stávající mock pro nové CLI:
- [ ] `--cli=opencode` flag → OpenCode-style output
- [ ] `--cli=codex` flag → Codex-style output
- [ ] Reuse v E2E testech

### Validation testy

- [ ] `TestCLIModelCompat_ClaudeWithAnthropicModel` — OK
- [ ] `TestCLIModelCompat_CodexWithOpenAIModel` — OK
- [ ] `TestCLIModelCompat_CodexWithAnthropicModel` — error
- [ ] `TestCLIModelCompat_ClaudeWithOpenAIModel` — error

## Linter checklist

- [ ] `exec.Command` vždy s explicitními argumenty (žádný shell injection)
- [ ] API keys nikdy v logách (`slog` field filtering)
- [ ] Context propagace do všech exec volání
- [ ] `task fmt` + `task lint` MUSÍ projít

## Priorita

| CLI | Priorita | Důvod |
|-----|----------|-------|
| OpenCode | Vysoká | Open-source, Go-based, JSON output (`-f json`), npm install, podobný Claude Code |
| Codex | Vysoká | Rust binary, JSONL streaming (`--json`), `--full-auto`, strukturovaný výstup, OpenAI ekosystém |
| Aider | Nízká | Python dependency (3.9-3.12), **žádný JSON output** (plain text only), obtížnější stream normalizace |
