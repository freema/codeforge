package tools

import (
	"context"
	"errors"
	"log/slog"

	"github.com/freema/codeforge/internal/apperror"
)

var builtinTools = []ToolDefinition{
	{
		Name:         "sentry",
		Type:         ToolTypeMCP,
		Description:  "Sentry error tracking — search issues, get stack traces, resolve errors",
		MCPTransport: "http",
		MCPURL:       "https://mcp.sentry.dev/mcp",
		RequiredConfig: []ConfigField{
			{Name: "auth_token", Description: "Sentry authentication token", Type: "secret", EnvVar: "SENTRY_AUTH_TOKEN", Sensitive: true},
		},
		Capabilities: []string{"error-tracking", "issues"},
		Builtin:      true,
	},
	{
		Name:        "jira",
		Type:        ToolTypeMCP,
		Description: "Jira issue tracking — search, create, update issues and boards",
		MCPPackage:  "mcp-atlassian",
		MCPCommand:  "uvx",
		RequiredConfig: []ConfigField{
			{Name: "url", Description: "Jira instance URL", Type: "url", EnvVar: "JIRA_URL"},
			{Name: "username", Description: "Jira username (email)", Type: "string", EnvVar: "JIRA_USERNAME"},
			{Name: "api_token", Description: "Jira API token", Type: "secret", EnvVar: "JIRA_API_TOKEN", Sensitive: true},
		},
		Capabilities: []string{"issue-tracking", "project-management"},
		Builtin:      true,
	},
	{
		Name:        "git",
		Type:        ToolTypeMCP,
		Description: "Git operations — diff, log, blame, branch management via MCP",
		MCPPackage:  "@cyanheads/git-mcp-server",
		MCPCommand:  "npx",
		Capabilities: []string{"version-control", "git"},
		Builtin:      true,
	},
	{
		Name:        "github",
		Type:        ToolTypeMCP,
		Description: "GitHub — issues, PRs, repos, actions via GitHub MCP server",
		MCPPackage:  "@modelcontextprotocol/server-github",
		MCPCommand:  "npx",
		RequiredConfig: []ConfigField{
			{Name: "token", Description: "GitHub personal access token", Type: "secret", EnvVar: "GITHUB_PERSONAL_ACCESS_TOKEN", Sensitive: true},
		},
		Capabilities: []string{"github", "issues", "pull-requests"},
		Builtin:      true,
	},
	{
		Name:        "playwright",
		Type:        ToolTypeMCP,
		Description: "Playwright browser automation — navigate, click, screenshot, test",
		MCPPackage:  "@playwright/mcp",
		MCPCommand:  "npx",
		Capabilities: []string{"browser", "testing", "automation"},
		Builtin:      true,
	},
}

// BuiltinCatalog returns all built-in tool definitions.
func BuiltinCatalog() []ToolDefinition {
	out := make([]ToolDefinition, len(builtinTools))
	copy(out, builtinTools)
	return out
}

// BuiltinByName looks up a built-in tool by name. Returns nil if not found.
func BuiltinByName(name string) *ToolDefinition {
	for i := range builtinTools {
		if builtinTools[i].Name == name {
			def := builtinTools[i]
			return &def
		}
	}
	return nil
}

// SeedBuiltins inserts all built-in tools into the registry (idempotent).
func SeedBuiltins(ctx context.Context, reg Registry) error {
	for _, def := range builtinTools {
		err := reg.Create(ctx, "global", def)
		if err != nil {
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && errors.Is(appErr, apperror.ErrConflict) {
				slog.Debug("built-in tool already exists, skipping", "name", def.Name)
				continue
			}
			return err
		}
		slog.Info("seeded built-in tool", "name", def.Name)
	}
	return nil
}
