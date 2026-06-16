package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Installer generates MCP configuration files in workspaces.
type Installer struct {
	registry Registry
}

// NewInstaller creates a new MCP installer.
func NewInstaller(registry Registry) *Installer {
	return &Installer{registry: registry}
}

// Setup resolves MCP servers and writes the CLI-appropriate config file to the
// workspace. Claude Code / Codex read .mcp.json; Cursor reads .cursor/cli.json
// (it has no --mcp-config flag). If no servers are configured, no file is written.
func (i *Installer) Setup(ctx context.Context, workDir, projectID, cli string, taskServers []Server) error {
	servers, err := i.registry.ResolveMCPServers(ctx, projectID, taskServers)
	if err != nil {
		return fmt.Errorf("resolving MCP servers: %w", err)
	}

	if len(servers) == 0 {
		return nil // nothing to install
	}

	return WriteMCPConfigForCLI(workDir, cli, servers)
}

// configRelPath returns the MCP config path (relative to the workspace) for a CLI.
func configRelPath(cli string) string {
	if cli == "cursor" {
		return filepath.Join(".cursor", "cli.json")
	}
	return ".mcp.json"
}

// ConfigPath returns the absolute MCP config path a CLI reads from in the workspace.
func ConfigPath(workDir, cli string) string {
	return filepath.Join(workDir, configRelPath(cli))
}

// WriteMCPConfig writes the standard .mcp.json (Claude Code / Codex format).
// Kept for backward compatibility; new callers should prefer WriteMCPConfigForCLI.
func WriteMCPConfig(workDir string, servers []Server) error {
	return writeConfigFile(workDir, ".mcp.json", servers)
}

// WriteMCPConfigForCLI writes the MCP config in the location the given CLI expects.
func WriteMCPConfigForCLI(workDir, cli string, servers []Server) error {
	return writeConfigFile(workDir, configRelPath(cli), servers)
}

// buildMCPServers builds the "mcpServers" object shared by every CLI's config schema
// (Claude Code, Codex and Cursor all use the same {"mcpServers": {...}} shape).
func buildMCPServers(servers []Server) map[string]interface{} {
	mcpServers := make(map[string]interface{})
	for _, srv := range servers {
		if srv.IsHTTP() {
			entry := map[string]interface{}{
				"type": "http",
				"url":  srv.URL,
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
			mcpServers[srv.Name] = entry
			continue
		}

		// stdio transport
		command := srv.Command
		if command == "" {
			command = "npx"
		}

		// Build args based on command type
		var args []string
		switch command {
		case "npx":
			args = append([]string{"-y", srv.Package}, srv.Args...)
		case "docker":
			args = append([]string{"run", "-i", "--rm", srv.Package}, srv.Args...)
		default:
			args = append([]string{srv.Package}, srv.Args...)
		}

		entry := map[string]interface{}{
			"command": command,
			"args":    args,
		}
		if len(srv.Env) > 0 {
			entry["env"] = srv.Env
		}
		mcpServers[srv.Name] = entry
	}
	return mcpServers
}

// writeConfigFile serializes the MCP config and writes it to workDir/relPath,
// creating parent directories as needed and gitignoring the generated file.
// It MERGES into any existing config (e.g. a repo's committed .cursor/cli.json),
// setting only the "mcpServers" key so other settings are never destroyed.
func writeConfigFile(workDir, relPath string, servers []Server) error {
	configPath := filepath.Join(workDir, relPath)

	config := map[string]interface{}{}
	if existing, readErr := os.ReadFile(configPath); readErr == nil {
		if json.Unmarshal(existing, &config) != nil {
			config = map[string]interface{}{} // unparseable existing file → start fresh
		}
	}
	config["mcpServers"] = buildMCPServers(servers)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}

	if dir := filepath.Dir(configPath); dir != workDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating MCP config dir: %w", err)
		}
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", relPath, err)
	}

	// Ensure the generated config (which contains secrets) is gitignored so the
	// AI's in-session commits never include it. The PR-time cleanup in branch.go
	// only protects the final commit; this protects any commit the agent makes
	// mid-session (e.g. one-commit-per-fix workflows).
	if err := ensureGitignore(workDir, filepath.ToSlash(relPath)); err != nil {
		return fmt.Errorf("updating .gitignore for %s: %w", relPath, err)
	}

	return nil
}

// ensureGitignore appends entry to the workspace .gitignore if not already present.
// Idempotent and best-effort about formatting (always writes a trailing newline).
func ensureGitignore(workDir, entry string) error {
	path := filepath.Join(workDir, ".gitignore")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // already ignored
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	prefix := ""
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + entry + "\n"); err != nil {
		return err
	}
	return nil
}
