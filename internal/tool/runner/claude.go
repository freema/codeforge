package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ClaudeRunner executes Claude Code CLI.
//
// The same type backs both the standard "claude-code" runner and the
// "claude-agent" runner (--bare mode): they share one binary and one execution
// path, differing only by extraArgs. This keeps the two from silently diverging
// (e.g. when the gosu privilege-drop logic is updated).
type ClaudeRunner struct {
	binaryPath string
	extraArgs  []string // extra CLI flags injected on every run (e.g. ["--bare"] for agent mode)
	label      string   // identifier used in log messages ("claude", "claude-agent")
}

// NewClaudeRunner creates a runner for the Claude Code CLI.
func NewClaudeRunner(binaryPath string) *ClaudeRunner {
	return newClaudeRunner(binaryPath, "claude", nil)
}

// newClaudeRunner is the shared constructor for ClaudeRunner-backed runners.
// If binaryPath contains a directory separator, it is resolved to an absolute
// path so it remains valid when cmd.Dir is set to the session workspace. Bare
// command names (e.g. "claude") are left as-is so exec.Command looks them up via PATH.
func newClaudeRunner(binaryPath, label string, extraArgs []string) *ClaudeRunner {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		if abs, err := filepath.Abs(binaryPath); err == nil {
			binaryPath = abs
		}
	}
	return &ClaudeRunner{binaryPath: binaryPath, label: label, extraArgs: extraArgs}
}

// buildArgs constructs the claude CLI argument list for a run. Extracted so the
// flag composition (including extraArgs like --bare) is unit-testable without
// executing the binary.
func (c *ClaudeRunner) buildArgs(opts RunOptions) []string {
	args := []string{
		"-p", opts.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
	}
	args = append(args, c.extraArgs...)
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}
	if opts.MCPConfigPath != "" {
		args = append(args, "--mcp-config", opts.MCPConfigPath)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}
	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', 2, 64))
	}
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}
	if opts.AllowedTools != "" {
		args = append(args, "--allowedTools", opts.AllowedTools)
	}
	return args
}

// Run executes Claude Code with stream-json output, calling OnEvent for each line.
func (c *ClaudeRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	args := c.buildArgs(opts)

	// Resolve the binary to its real path. If it's a Node.js script (shebang),
	// run it via "node" directly to avoid fork/exec ENOENT issues that can occur
	// with shebang scripts under privilege dropping (SysProcAttr.Credential).
	binary := c.binaryPath
	cmdArgs := args

	resolved, err := exec.LookPath(binary)
	if err == nil {
		// Follow symlinks to get the real file
		if real, linkErr := filepath.EvalSymlinks(resolved); linkErr == nil {
			resolved = real
		}
		// If the target is a .js file, invoke via node to bypass shebang
		if strings.HasSuffix(resolved, ".js") {
			slog.Debug(c.label+" binary is a Node.js script, using node interpreter", "script", resolved)
			cmdArgs = append([]string{resolved}, args...)
			binary = "node"
		}
	}

	cmd := exec.CommandContext(ctx, binary, cmdArgs...)
	cmd.Dir = opts.WorkDir

	// Build environment from an allowlist — the CLI runs untrusted checked-out
	// code, so it must not inherit the server's secrets (see sanitizedEnv).
	// If running as root and a "codeforge" user exists, drop privileges and
	// replace HOME/SHELL so Claude Code accepts bypassPermissions.
	baseEnv := sanitizedEnv()
	if os.Getuid() == 0 {
		if u, err := user.Lookup("codeforge"); err == nil {
			uid, _ := strconv.ParseUint(u.Uid, 10, 32)
			gid, _ := strconv.ParseUint(u.Gid, 10, 32)

			// Use gosu for privilege dropping. Go's SysProcAttr.Credential
			// can fail with ENOENT on Alpine + Docker (kernel-level exec issue
			// with Setpgid + Credential combination).
			if gosuPath, gosuErr := exec.LookPath("gosu"); gosuErr == nil {
				gosuArgs := append([]string{u.Username, cmd.Path}, cmd.Args[1:]...)
				cmd = exec.CommandContext(ctx, gosuPath, gosuArgs...)
				cmd.Dir = opts.WorkDir
				slog.Debug("dropping privileges for "+c.label+" CLI via gosu", "uid", uid, "gid", gid)
			} else {
				// Fallback: use SysProcAttr.Credential directly
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Setpgid:    true,
					Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
				}
				slog.Debug("dropping privileges for "+c.label+" CLI via credential", "uid", uid, "gid", gid)
			}

			// Filter out HOME/SHELL/USER from root env and replace them
			filtered := make([]string, 0, len(baseEnv))
			for _, e := range baseEnv {
				if !strings.HasPrefix(e, "HOME=") &&
					!strings.HasPrefix(e, "SHELL=") &&
					!strings.HasPrefix(e, "USER=") {
					filtered = append(filtered, e)
				}
			}
			filtered = append(filtered,
				"HOME="+u.HomeDir,
				"SHELL=/bin/sh",
				"USER=codeforge",
			)
			baseEnv = filtered
		}
	}
	configureGracefulKill(cmd)

	// Only set ANTHROPIC_API_KEY if provided per-session; otherwise inherit from
	// process environment (baseEnv) so a global key can be configured via env var.
	if opts.APIKey != "" {
		cmd.Env = append(baseEnv, "ANTHROPIC_API_KEY="+opts.APIKey)
	} else {
		cmd.Env = baseEnv
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s CLI: %w", c.label, err)
	}

	slog.Info(c.label+" CLI started", "pid", cmd.Process.Pid, "work_dir", opts.WorkDir)

	// Read stream-json: each line is a complete JSON object
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var resultText string        // from the "result" event (authoritative if present)
	var lastAssistantText string // from the latest "assistant" text event (fallback)
	var inputTokens, outputTokens int
	var cacheReadTokens, cacheCreationTokens int
	var costUSD float64
	var numTurns int
	var cliSessionID string

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Forward raw event to callback
		if opts.OnEvent != nil {
			eventCopy := make(json.RawMessage, len(line))
			copy(eventCopy, line)
			opts.OnEvent(eventCopy)
		}

		// Extract result text and usage from stream events
		data := extractStreamData(line)
		if data.ResultText != "" {
			resultText = data.ResultText
		}
		if data.AssistantText != "" {
			lastAssistantText = data.AssistantText
		}
		inputTokens += data.InputTokens
		outputTokens += data.OutputTokens
		cacheReadTokens += data.CacheReadInputTokens
		cacheCreationTokens += data.CacheCreationInputTokens
		if data.CostUSD > 0 {
			costUSD = data.CostUSD // total_cost_usd is a run total, not a delta
		}
		if data.NumTurns > 0 {
			numTurns = data.NumTurns
		}
		if data.SessionID != "" {
			cliSessionID = data.SessionID // later events win (resume forks to a new session id)
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		// e.g. a line exceeding the 1MB buffer — the stream (and result/usage
		// extracted from it) may be truncated.
		slog.Warn(c.label+" CLI stream scan error, output may be truncated", "error", scanErr)
	}

	err = cmd.Wait()
	duration := time.Since(startTime)

	// Prefer the result event's text (authoritative), fall back to last assistant text.
	// The result event may be empty when subtype is "error_during_execution".
	output := resultText
	if output == "" {
		output = lastAssistantText
	}

	result := &RunResult{
		Output:                   output,
		ExitCode:                 -1,
		Duration:                 duration,
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     cacheReadTokens,
		CacheCreationInputTokens: cacheCreationTokens,
		CostUSD:                  costUSD,
		NumTurns:                 numTurns,
		CLISessionID:             cliSessionID,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		slog.Warn(c.label+" CLI exited with error",
			"exit_code", result.ExitCode,
			"stderr", stderrBuf.String(),
			"duration", duration,
		)
		return result, fmt.Errorf("%s CLI exited with code %d: %w", c.label, result.ExitCode, err)
	}

	slog.Info(c.label+" CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)

	return result, nil
}

// claudeStreamData holds everything extracted from one Claude Code stream-json line.
type claudeStreamData struct {
	ResultText               string // from the final "result" event (authoritative when present)
	AssistantText            string // from "assistant" text events (fallback when result is empty)
	InputTokens              int    // from the "result" event usage
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
	CostUSD                  float64 // total_cost_usd from the "result" event (run total)
	NumTurns                 int     // num_turns from the "result" event
	SessionID                string  // session_id from the "system" init or "result" event
}

// extractStreamData parses a Claude Code stream-json line for result text,
// assistant text, usage, cost, turn count, and the CLI session id.
func extractStreamData(line []byte) claudeStreamData {
	var data claudeStreamData

	var event map[string]json.RawMessage
	if err := json.Unmarshal(line, &event); err != nil {
		return data
	}

	var eventType string
	if err := json.Unmarshal(event["type"], &eventType); err != nil {
		return data
	}

	switch eventType {
	case "result":
		var result struct {
			Result       string  `json:"result"`
			SessionID    string  `json:"session_id"`
			TotalCostUSD float64 `json:"total_cost_usd"`
			NumTurns     int     `json:"num_turns"`
			Usage        struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(line, &result); err == nil {
			data.ResultText = result.Result
			data.SessionID = result.SessionID
			data.InputTokens = result.Usage.InputTokens
			data.OutputTokens = result.Usage.OutputTokens
			data.CacheReadInputTokens = result.Usage.CacheReadInputTokens
			data.CacheCreationInputTokens = result.Usage.CacheCreationInputTokens
			data.CostUSD = result.TotalCostUSD
			data.NumTurns = result.NumTurns
		}

	case "system":
		// The "system" init event announces the CLI session id at run start.
		var sys struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(line, &sys); err == nil {
			data.SessionID = sys.SessionID
		}

	case "assistant":
		// Capture text content from assistant messages as fallback.
		// When the result event has subtype "error_during_execution",
		// its result field is empty — the actual output is here.
		var msg struct {
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &msg); err == nil {
			var sb strings.Builder
			for _, c := range msg.Message.Content {
				if c.Type == "text" && c.Text != "" {
					sb.WriteString(c.Text)
				}
			}
			data.AssistantText = sb.String()
		}
	}

	return data
}
