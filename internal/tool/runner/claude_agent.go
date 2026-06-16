package runner

// NewClaudeAgentRunner creates a Claude Code runner in bare/agent mode.
//
// Agent mode reuses the exact same execution path as the standard Claude Code
// runner (ClaudeRunner) — same binary, same stream-json parsing, same privilege
// dropping. The only difference is the injected --bare flag, which skips
// auto-discovery of hooks, skills, plugins, MCP, and CLAUDE.md for deterministic
// CI/scripted behavior. Returning *ClaudeRunner (rather than a duplicate type)
// guarantees the two runners cannot silently diverge.
func NewClaudeAgentRunner(binaryPath string) *ClaudeRunner {
	return newClaudeRunner(binaryPath, "claude-agent", []string{"--bare"})
}
