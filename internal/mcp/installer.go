package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Installer generates .mcp.json configuration files in workspaces.
type Installer struct {
	registry *Registry
}

// NewInstaller creates a new MCP installer.
func NewInstaller(registry *Registry) *Installer {
	return &Installer{registry: registry}
}

// Setup resolves MCP servers and writes .mcp.json to the workspace.
// If no servers are configured, no file is written.
func (i *Installer) Setup(ctx context.Context, workDir string, projectID string, taskServers []Server) error {
	servers, err := i.registry.ResolveMCPServers(ctx, projectID, taskServers)
	if err != nil {
		return fmt.Errorf("resolving MCP servers: %w", err)
	}

	if len(servers) == 0 {
		return nil // nothing to install
	}

	return WriteMCPConfig(workDir, servers)
}

// WriteMCPConfig writes .mcp.json to the given directory.
func WriteMCPConfig(workDir string, servers []Server) error {
	mcpServers := make(map[string]interface{})
	for _, srv := range servers {
		entry := map[string]interface{}{
			"command": "npx",
			"args":    append([]string{"-y", srv.Package}, srv.Args...),
		}
		if len(srv.Env) > 0 {
			entry["env"] = srv.Env
		}
		mcpServers[srv.Name] = entry
	}

	config := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}

	configPath := filepath.Join(workDir, ".mcp.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("writing .mcp.json: %w", err)
	}

	return nil
}
