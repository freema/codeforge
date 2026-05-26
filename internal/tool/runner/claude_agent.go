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

// ClaudeAgentRunner executes Claude Code CLI in bare/agent mode.
// It uses the same binary as ClaudeRunner but adds --bare for
// deterministic CI behavior (skips hooks, skills, plugins, MCP, CLAUDE.md).
type ClaudeAgentRunner struct {
	binaryPath string
}

// NewClaudeAgentRunner creates a runner for Claude Code in agent mode.
func NewClaudeAgentRunner(binaryPath string) *ClaudeAgentRunner {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		if abs, err := filepath.Abs(binaryPath); err == nil {
			binaryPath = abs
		}
	}
	return &ClaudeAgentRunner{binaryPath: binaryPath}
}

// Run executes Claude Code with --bare flag and stream-json output.
func (c *ClaudeAgentRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	args := []string{
		"-p", opts.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--bare",
		"--permission-mode", "bypassPermissions",
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

	binary := c.binaryPath
	cmdArgs := args

	resolved, err := exec.LookPath(binary)
	if err == nil {
		if real, linkErr := filepath.EvalSymlinks(resolved); linkErr == nil {
			resolved = real
		}
		if strings.HasSuffix(resolved, ".js") {
			slog.Debug("claude-agent binary is a Node.js script, using node interpreter", "script", resolved)
			cmdArgs = append([]string{resolved}, args...)
			binary = "node"
		}
	}

	cmd := exec.CommandContext(ctx, binary, cmdArgs...)
	cmd.Dir = opts.WorkDir

	baseEnv := os.Environ()
	if os.Getuid() == 0 {
		if u, err := user.Lookup("codeforge"); err == nil {
			uid, _ := strconv.ParseUint(u.Uid, 10, 32)
			gid, _ := strconv.ParseUint(u.Gid, 10, 32)

			if gosuPath, gosuErr := exec.LookPath("gosu"); gosuErr == nil {
				gosuArgs := append([]string{u.Username, cmd.Path}, cmd.Args[1:]...)
				cmd = exec.CommandContext(ctx, gosuPath, gosuArgs...)
				cmd.Dir = opts.WorkDir
				slog.Debug("dropping privileges for claude-agent CLI via gosu", "uid", uid, "gid", gid)
			} else {
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Setpgid:    true,
					Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
				}
				slog.Debug("dropping privileges for claude-agent CLI via credential", "uid", uid, "gid", gid)
			}

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
		return nil, fmt.Errorf("starting claude-agent CLI: %w", err)
	}

	slog.Info("claude-agent CLI started", "pid", cmd.Process.Pid, "work_dir", opts.WorkDir)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var resultText string
	var lastAssistantText string
	var inputTokens, outputTokens int

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if opts.OnEvent != nil {
			eventCopy := make(json.RawMessage, len(line))
			copy(eventCopy, line)
			opts.OnEvent(eventCopy)
		}

		rText, aText, iTokens, oTokens := extractStreamData(line)
		if rText != "" {
			resultText = rText
		}
		if aText != "" {
			lastAssistantText = aText
		}
		inputTokens += iTokens
		outputTokens += oTokens
	}

	err = cmd.Wait()
	duration := time.Since(startTime)

	output := resultText
	if output == "" {
		output = lastAssistantText
	}

	result := &RunResult{
		Output:       output,
		ExitCode:     -1,
		Duration:     duration,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		slog.Warn("claude-agent CLI exited with error",
			"exit_code", result.ExitCode,
			"stderr", stderrBuf.String(),
			"duration", duration,
		)
		return result, fmt.Errorf("claude-agent CLI exited with code %d: %w", result.ExitCode, err)
	}

	slog.Info("claude-agent CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)

	return result, nil
}
