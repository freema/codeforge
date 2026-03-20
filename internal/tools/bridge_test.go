package tools

import (
	"testing"
)

func TestToMCPServers_SingleTool_Stdio(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "playwright",
				MCPPackage: "@playwright/mcp",
				MCPCommand: "npx",
			},
			Config: map[string]string{},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	srv := servers[0]
	if srv.Name != "playwright" {
		t.Errorf("Name = %q, want playwright", srv.Name)
	}
	if srv.Package != "@playwright/mcp" {
		t.Errorf("Package = %q, want @playwright/mcp", srv.Package)
	}
	if srv.Command != "npx" {
		t.Errorf("Command = %q, want npx", srv.Command)
	}
}

func TestToMCPServers_SingleTool_HTTP(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:         "custom-http",
				MCPTransport: "http",
				MCPURL:       "https://example.com/mcp",
				RequiredConfig: []ConfigField{
					{Name: "api_key", EnvVar: "API_KEY"},
				},
			},
			Config: map[string]string{"api_key": "tok123"},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	srv := servers[0]
	if srv.Name != "custom-http" {
		t.Errorf("Name = %q, want custom-http", srv.Name)
	}
	if srv.Transport != "http" {
		t.Errorf("Transport = %q, want http", srv.Transport)
	}
	if srv.URL != "https://example.com/mcp" {
		t.Errorf("URL = %q, want https://example.com/mcp", srv.URL)
	}
	if srv.Headers["API_KEY"] != "tok123" {
		t.Errorf("Headers[API_KEY] = %q, want tok123", srv.Headers["API_KEY"])
	}
	// HTTP tools should NOT have Package/Env
	if srv.Package != "" {
		t.Errorf("Package should be empty for HTTP tool, got %q", srv.Package)
	}
	if srv.Env != nil {
		t.Errorf("Env should be nil for HTTP tool, got %v", srv.Env)
	}
}

func TestToMCPServers_SentryStdio(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "sentry",
				MCPPackage: "@sentry/mcp-server",
				MCPCommand: "npx",
				RequiredConfig: []ConfigField{
					{Name: "auth_token", EnvVar: "SENTRY_ACCESS_TOKEN"},
				},
			},
			Config: map[string]string{"auth_token": "sntrys_tok123"},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	srv := servers[0]
	if srv.Name != "sentry" {
		t.Errorf("Name = %q, want sentry", srv.Name)
	}
	if srv.Package != "@sentry/mcp-server" {
		t.Errorf("Package = %q, want @sentry/mcp-server", srv.Package)
	}
	if srv.Command != "npx" {
		t.Errorf("Command = %q, want npx", srv.Command)
	}
	if srv.Env["SENTRY_ACCESS_TOKEN"] != "sntrys_tok123" {
		t.Errorf("Env[SENTRY_ACCESS_TOKEN] = %q, want sntrys_tok123", srv.Env["SENTRY_ACCESS_TOKEN"])
	}
	// STDIO tools should NOT have URL/Headers
	if srv.URL != "" {
		t.Errorf("URL should be empty for stdio tool, got %q", srv.URL)
	}
	if srv.Headers != nil {
		t.Errorf("Headers should be nil for stdio tool, got %v", srv.Headers)
	}
}

func TestToMCPServers_EnvVarMapping(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "jira",
				MCPPackage: "mcp-atlassian",
				RequiredConfig: []ConfigField{
					{Name: "url", EnvVar: "JIRA_URL"},
					{Name: "username", EnvVar: "JIRA_USERNAME"},
					{Name: "api_token", EnvVar: "JIRA_API_TOKEN"},
				},
			},
			Config: map[string]string{
				"url":       "https://myco.atlassian.net",
				"username":  "dev@example.com",
				"api_token": "secret",
			},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	env := servers[0].Env
	if env["JIRA_URL"] != "https://myco.atlassian.net" {
		t.Errorf("JIRA_URL = %q", env["JIRA_URL"])
	}
	if env["JIRA_USERNAME"] != "dev@example.com" {
		t.Errorf("JIRA_USERNAME = %q", env["JIRA_USERNAME"])
	}
	if env["JIRA_API_TOKEN"] != "secret" {
		t.Errorf("JIRA_API_TOKEN = %q", env["JIRA_API_TOKEN"])
	}
}

func TestToMCPServers_MultipleTools(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "sentry",
				MCPPackage: "@sentry/mcp-server",
				MCPCommand: "npx",
				RequiredConfig: []ConfigField{
					{Name: "auth_token", EnvVar: "SENTRY_ACCESS_TOKEN"},
				},
			},
			Config: map[string]string{"auth_token": "tok123"},
		},
		{
			Definition: &ToolDefinition{
				Name:       "playwright",
				MCPPackage: "@playwright/mcp",
				MCPCommand: "npx",
			},
			Config: map[string]string{},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Name != "sentry" {
		t.Errorf("servers[0].Name = %q, want sentry", servers[0].Name)
	}
	if servers[0].Package != "@sentry/mcp-server" {
		t.Errorf("servers[0].Package = %q, want @sentry/mcp-server", servers[0].Package)
	}
	if servers[1].Name != "playwright" {
		t.Errorf("servers[1].Name = %q, want playwright", servers[1].Name)
	}
	if servers[1].Package != "@playwright/mcp" {
		t.Errorf("servers[1].Package = %q, want @playwright/mcp", servers[1].Package)
	}
}

func TestToMCPServers_EmptyConfig(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "git",
				MCPPackage: "@cyanheads/git-mcp-server",
			},
			Config: map[string]string{},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Env != nil {
		t.Errorf("expected nil Env for tool with no config mapping, got %v", servers[0].Env)
	}
}

func TestToMCPServers_Empty(t *testing.T) {
	servers := ToMCPServers(nil)
	if servers != nil {
		t.Errorf("expected nil for empty instances, got %v", servers)
	}
}

func TestToMCPServers_SkipsNoPackage(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name: "no-mcp",
				Type: ToolTypeCustom,
			},
			Config: map[string]string{},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 0 {
		t.Errorf("expected 0 servers for tool without MCPPackage, got %d", len(servers))
	}
}

func TestToMCPServers_SkipsHTTPNoURL(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:         "broken-http",
				MCPTransport: "http",
				MCPURL:       "", // missing URL
			},
			Config: map[string]string{},
		},
	}

	servers := ToMCPServers(instances)
	if len(servers) != 0 {
		t.Errorf("expected 0 servers for HTTP tool without URL, got %d", len(servers))
	}
}
