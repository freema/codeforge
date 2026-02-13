# Phase 4 — Key Registry & MCP Management (v0.5.0)

> Token management and MCP server configuration.

---

## Task 4.1: Key Registry API

**Priority:** P0
**Files:** `internal/keys/registry.go`, `internal/server/handlers/keys.go`

### Description

CRUD API for GitHub/GitLab access tokens. Tokens are stored encrypted in Redis and referenced by name in task payloads.

### Acceptance Criteria

- [ ] `POST /api/v1/keys` — register a new key `{name, provider, token, scope}`
- [ ] `GET /api/v1/keys` — list all keys (names + provider + created_at, NO tokens)
- [ ] `DELETE /api/v1/keys/{name}` — remove a key
- [ ] Keys stored in Redis: `keys:{provider}:{name}` → Hash `{encrypted_token, created_at, scope}`
- [ ] Token encrypted before storage (see Task 4.2)
- [ ] Key name must be unique per provider
- [ ] Returns 409 if key name already exists
- [ ] Returns 404 on delete of non-existent key
- [ ] Validate provider is "github" or "gitlab"

### Implementation Notes

```go
type Key struct {
    Name      string   `json:"name"`
    Provider  string   `json:"provider"`  // "github" | "gitlab"
    Token     string   `json:"token,omitempty"` // only in create request
    Scope     string   `json:"scope,omitempty"` // e.g., "repo", "read_api"
    CreatedAt time.Time `json:"created_at"`
}

type KeyRegistry struct {
    redis  *redis.Client
    crypto *CryptoService
}

func (kr *KeyRegistry) Create(ctx context.Context, key Key) error {
    redisKey := fmt.Sprintf("keys:%s:%s", key.Provider, key.Name)

    // Check uniqueness
    exists, _ := kr.redis.Exists(ctx, redisKey).Result()
    if exists > 0 {
        return ErrKeyExists
    }

    // Encrypt token
    encrypted, err := kr.crypto.Encrypt(key.Token)
    // Store hash
    kr.redis.HSet(ctx, redisKey, map[string]interface{}{
        "encrypted_token": encrypted,
        "scope":           key.Scope,
        "created_at":      time.Now().UTC().Format(time.RFC3339),
    })
    return nil
}

func (kr *KeyRegistry) Resolve(ctx context.Context, provider, name string) (string, error) {
    redisKey := fmt.Sprintf("keys:%s:%s", provider, name)
    encrypted, err := kr.redis.HGet(ctx, redisKey, "encrypted_token").Result()
    return kr.crypto.Decrypt(encrypted)
}
```

### Dependencies

- Task 4.2 (encryption for token storage)
- Task 0.6 (Redis client)
- Task 0.7 (HTTP server for routes)

---

## Task 4.2: Key Encryption (AES-256-GCM)

**Priority:** P0
**Files:** `internal/keys/crypto.go`

### Description

Encrypt/decrypt tokens using AES-256-GCM before storing in Redis. Encryption key from config/env.

> **Shared service:** Used by both Key Registry (Task 4.1) and Task model (Task 1.1) for encrypting `access_token` and `ai_api_key` in task state. Consider placing in `internal/crypto/` instead of `internal/keys/` since it's not key-registry-specific.

### Acceptance Criteria

- [ ] AES-256-GCM encryption with random nonce per encryption
- [ ] Encryption key from env: `CODEFORGE_ENCRYPTION_KEY` (32 bytes, base64-encoded)
- [ ] `Encrypt(plaintext) → ciphertext (base64)`
- [ ] `Decrypt(ciphertext) → plaintext`
- [ ] Nonce prepended to ciphertext (standard pattern)
- [ ] Key rotation support (future): store key version with ciphertext
- [ ] Unit tests with known test vectors

### Implementation Notes

```go
type CryptoService struct {
    key []byte // 32 bytes for AES-256
}

func NewCryptoService(keyBase64 string) (*CryptoService, error) {
    key, err := base64.StdEncoding.DecodeString(keyBase64)
    if len(key) != 32 {
        return nil, fmt.Errorf("encryption key must be 32 bytes")
    }
    return &CryptoService{key: key}, nil
}

func (c *CryptoService) Encrypt(plaintext string) (string, error) {
    block, _ := aes.NewCipher(c.key)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (c *CryptoService) Decrypt(encoded string) (string, error) {
    ciphertext, _ := base64.StdEncoding.DecodeString(encoded)
    block, _ := aes.NewCipher(c.key)
    gcm, _ := cipher.NewGCM(block)
    nonceSize := gcm.NonceSize()
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    return string(plaintext), err
}
```

### Dependencies

- Task 0.2 (config for encryption key)

---

## Task 4.3: Default Key Resolution

**Priority:** P0
**Files:** `internal/keys/resolver.go`

### Description

Resolve the access token for a task: check task-level override → registered key by name → env var fallback.

### Acceptance Criteria

- [ ] Resolution order:
  1. Task `access_token` field (top-level on Task struct)
  2. Registered key by `provider_key` name
  3. Env var fallback: `GITHUB_TOKEN` / `GITLAB_TOKEN`
- [ ] Error if no token can be resolved
- [ ] Provider auto-detected from repo URL (Task 2.6)
- [ ] Returns decrypted token ready for use

### Implementation Notes

```go
func (r *Resolver) ResolveToken(ctx context.Context, task *Task) (string, error) {
    // 1. Inline token (top-level field on Task, NOT inside Config)
    if task.AccessToken != "" {
        return task.AccessToken, nil
    }

    // 2. Registered key
    if task.ProviderKey != "" {
        provider := detectProvider(task.RepoURL)
        return r.registry.Resolve(ctx, string(provider), task.ProviderKey)
    }

    // 3. Env var fallback
    provider := detectProvider(task.RepoURL)
    switch provider {
    case ProviderGitHub:
        if t := os.Getenv("GITHUB_TOKEN"); t != "" { return t, nil }
    case ProviderGitLab:
        if t := os.Getenv("GITLAB_TOKEN"); t != "" { return t, nil }
    }

    return "", ErrNoToken
}
```

### Dependencies

- Task 4.1 (key registry)
- Task 2.6 (provider detection)

---

## Task 4.4: MCP Server Registry API

**Priority:** P0
**Files:** `internal/mcp/registry.go`, `internal/server/handlers/mcp.go`

### Description

CRUD API for global MCP (Model Context Protocol) servers. These are npx-based servers that extend Claude Code's capabilities.

### Acceptance Criteria

- [ ] `POST /api/v1/mcp/servers` — register MCP server `{name, package, args, env}`
- [ ] `GET /api/v1/mcp/servers` — list all registered MCP servers
- [ ] `DELETE /api/v1/mcp/servers/{name}` — remove MCP server
- [ ] Stored in Redis as per-server keys: `mcp:global:{name}` → Hash `{package, args_json, env_json, created_at}`
- [ ] Server config: `{name, package, args: [], env: {}}`
- [ ] Unique name enforced by Redis key (HSETNX or existence check)
- [ ] List all: `SCAN mcp:global:*` or maintain index set `mcp:global:_index` (SADD/SMEMBERS)

### Implementation Notes

```go
type MCPServer struct {
    Name    string            `json:"name"`
    Package string            `json:"package"`    // e.g., "@anthropic-ai/mcp-server-fetch"
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
}

// Per-server Redis keys (no race conditions on concurrent updates)
// SET mcp:global:fetch → Hash {package, args_json, env_json}
// Index: SADD mcp:global:_index "fetch"
// List:  SMEMBERS mcp:global:_index → for each, HGETALL mcp:global:{name}
// Delete: DEL mcp:global:{name} + SREM mcp:global:_index {name}
```

### Dependencies

- Task 0.6 (Redis client)
- Task 0.7 (HTTP server)

---

## Task 4.5: Per-Project MCP Config

**Priority:** P1
**Files:** `internal/mcp/registry.go`, `internal/server/handlers/mcp.go`

### Description

Override MCP servers for specific repositories. When a task targets a repo with custom MCP config, use that instead of (or merged with) global config.

### Acceptance Criteria

- [ ] `PUT /api/v1/mcp/projects/{repo}` — set MCP config for a repo URL
- [ ] `GET /api/v1/mcp/projects/{repo}` — get project-specific MCP config
- [ ] `DELETE /api/v1/mcp/projects/{repo}` — remove project override
- [ ] Stored in Redis as per-server keys: `mcp:project:{encoded_repo_url}:{name}` → Hash (same pattern as global)
- [ ] Merge strategy: project servers added to global servers (or replace if same name)

### Dependencies

- Task 4.4 (global MCP registry)

---

## Task 4.6: MCP Installation

**Priority:** P0
**Files:** `internal/mcp/installer.go`

### Description

Install MCP servers at task startup using npx. Create temporary MCP config that Claude Code can use.

### Acceptance Criteria

- [ ] Resolve MCP servers: global + per-project + per-task overrides
- [ ] Generate `.mcp.json` in workspace directory before Claude Code runs
- [ ] **Claude Code launches MCP servers itself** from `.mcp.json` — CodeForge does NOT manage child processes
- [ ] Claude Code handles MCP server lifecycle (start, communication, cleanup)
- [ ] If `.mcp.json` generation fails: log warning, continue without MCP (non-blocking)
- [ ] `.mcp.json` cleaned up with workspace (TTL cleanup)

### Implementation Notes

```go
// Claude Code MCP config format (placed in workspace)
// .mcp.json
{
    "mcpServers": {
        "server-name": {
            "command": "npx",
            "args": ["-y", "@package/name", "--arg1"],
            "env": {"KEY": "value"}
        }
    }
}

func (i *Installer) Setup(ctx context.Context, workDir string, servers []MCPServer) error {
    config := map[string]interface{}{
        "mcpServers": buildMCPConfig(servers),
    }
    data, _ := json.MarshalIndent(config, "", "  ")
    return os.WriteFile(filepath.Join(workDir, ".mcp.json"), data, 0644)
}
```

### Dependencies

- Task 4.4 (MCP registry to resolve servers)
- Task 1.7 (Claude executor uses MCP config)

---

## Task 4.7: Claude Code MCP Integration

**Priority:** P0
**Files:** `internal/cli/claude.go`

### Description

Pass MCP server configuration to Claude Code CLI when executing tasks.

### Acceptance Criteria

- [ ] MCP config file (`.mcp.json`) placed in workspace before Claude Code runs (by Task 4.6)
- [ ] Claude Code discovers `.mcp.json` in working directory automatically
- [ ] MCP servers available to Claude Code during execution
- [ ] No health check — MCP protocol has no standard health check; Claude Code handles errors internally

### Dependencies

- Task 4.6 (MCP installation)
- Task 1.7 (Claude executor)

---

## Task 4.8: Per-Task MCP Override

**Priority:** P1
**Files:** `internal/mcp/manager.go`, `internal/worker/executor.go`

### Description

Task payload can include additional MCP servers that are only used for that specific task.

### Acceptance Criteria

- [ ] Task config accepts `mcp_servers` array
- [ ] Per-task servers merged with global + per-project servers
- [ ] Per-task servers take precedence (override by name)
- [ ] Cleanup: per-task MCP processes killed after task completes

### Implementation Notes

```go
// Task config
type TaskConfig struct {
    // ...
    MCPServers []MCPServer `json:"mcp_servers,omitempty"`
}

// Resolution order (later overrides earlier):
// 1. Global MCP servers
// 2. Per-project MCP servers
// 3. Per-task MCP servers
func (m *Manager) ResolveMCPServers(ctx context.Context, task *Task) ([]MCPServer, error) {
    servers := m.getGlobal(ctx)
    servers = mergeServers(servers, m.getProject(ctx, task.RepoURL))
    if task.Config != nil {
        servers = mergeServers(servers, task.Config.MCPServers)
    }
    return servers, nil
}
```

### Dependencies

- Task 4.4, 4.5, 4.6 (MCP infrastructure)
