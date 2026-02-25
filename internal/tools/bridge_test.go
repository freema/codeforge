package tools

import (
	"testing"
)

func TestToMCPServers_SingleTool(t *testing.T) {
	instances := []ToolInstance{
		{
			Definition: &ToolDefinition{
				Name:       "sentry",
				MCPPackage: "@sentry/mcp-server",
				MCPArgs:    []string{"--stdio"},
				RequiredConfig: []ConfigField{
					{Name: "auth_token", EnvVar: "SENTRY_AUTH_TOKEN"},
				},
			},
			Config: map[string]string{"auth_token": "tok123"},
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
	if len(srv.Args) != 1 || srv.Args[0] != "--stdio" {
		t.Errorf("Args = %v, want [--stdio]", srv.Args)
	}
	if srv.Env["SENTRY_AUTH_TOKEN"] != "tok123" {
		t.Errorf("Env[SENTRY_AUTH_TOKEN] = %q, want tok123", srv.Env["SENTRY_AUTH_TOKEN"])
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
			},
			Config: map[string]string{},
		},
		{
			Definition: &ToolDefinition{
				Name:       "playwright",
				MCPPackage: "@playwright/mcp",
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
	if servers[1].Name != "playwright" {
		t.Errorf("servers[1].Name = %q, want playwright", servers[1].Name)
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
