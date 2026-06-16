package runner

import "testing"

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func TestClaudeAgentRunner_BuildArgsIncludesBare(t *testing.T) {
	agent := NewClaudeAgentRunner("claude")
	if agent.label != "claude-agent" {
		t.Errorf("label = %q, want claude-agent", agent.label)
	}
	args := agent.buildArgs(RunOptions{Prompt: "fix it"})
	if !containsArg(args, "--bare") {
		t.Fatalf("expected --bare in claude-agent args, got %v", args)
	}
}

func TestClaudeRunner_BuildArgsNoBare(t *testing.T) {
	std := NewClaudeRunner("claude")
	if std.label != "claude" {
		t.Errorf("label = %q, want claude", std.label)
	}
	args := std.buildArgs(RunOptions{Prompt: "fix it"})
	if containsArg(args, "--bare") {
		t.Fatalf("did not expect --bare in standard claude args, got %v", args)
	}
}

func TestClaudeRunner_BuildArgsBaseFlags(t *testing.T) {
	std := NewClaudeRunner("claude")
	args := std.buildArgs(RunOptions{Prompt: "p", Model: "m", MCPConfigPath: "/tmp/.mcp.json"})
	for _, want := range []string{"-p", "--output-format", "stream-json", "--permission-mode", "bypassPermissions", "--model", "--mcp-config"} {
		if !containsArg(args, want) {
			t.Errorf("missing expected arg %q in %v", want, args)
		}
	}
}
