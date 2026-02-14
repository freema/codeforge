package cli

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
type ClaudeRunner struct {
	binaryPath string
}

// NewClaudeRunner creates a runner for the Claude Code CLI.
// If binaryPath contains a directory separator, it is resolved to an
// absolute path so it remains valid when cmd.Dir is set to the task
// workspace. Bare command names (e.g. "claude") are left as-is so
// exec.Command looks them up via PATH.
func NewClaudeRunner(binaryPath string) *ClaudeRunner {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		if abs, err := filepath.Abs(binaryPath); err == nil {
			binaryPath = abs
		}
	}
	return &ClaudeRunner{binaryPath: binaryPath}
}

// Run executes Claude Code with stream-json output, calling OnEvent for each line.
func (c *ClaudeRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	args := []string{
		"-p", opts.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
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

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	cmd.Dir = opts.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Build environment. If running as root and a "codeforge" user exists,
	// drop privileges and replace HOME/SHELL so Claude Code accepts bypassPermissions.
	baseEnv := os.Environ()
	if os.Getuid() == 0 {
		if u, err := user.Lookup("codeforge"); err == nil {
			uid, _ := strconv.ParseUint(u.Uid, 10, 32)
			gid, _ := strconv.ParseUint(u.Gid, 10, 32)
			cmd.SysProcAttr.Credential = &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
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
			slog.Debug("dropping privileges for claude CLI", "uid", uid, "gid", gid)
		}
	}
	cmd.Env = append(baseEnv, "ANTHROPIC_API_KEY="+opts.APIKey)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting claude CLI: %w", err)
	}

	slog.Info("claude CLI started", "pid", cmd.Process.Pid, "work_dir", opts.WorkDir)

	// Read stream-json: each line is a complete JSON object
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var resultText string      // from the "result" event (authoritative if present)
	var lastAssistantText string // from the latest "assistant" text event (fallback)
	var inputTokens, outputTokens int

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

	// Prefer the result event's text (authoritative), fall back to last assistant text.
	// The result event may be empty when subtype is "error_during_execution".
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
		slog.Warn("claude CLI exited with error",
			"exit_code", result.ExitCode,
			"stderr", stderrBuf.String(),
			"duration", duration,
		)
		return result, fmt.Errorf("claude CLI exited with code %d: %w", result.ExitCode, err)
	}

	slog.Info("claude CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)

	return result, nil
}

// extractStreamData parses a Claude Code stream-json line for result text,
// assistant text, and usage info.
//
// Returns:
//   - resultText: from the final "result" event (authoritative when present)
//   - assistantText: from "assistant" text events (fallback when result is empty)
//   - inputTokens, outputTokens: from the "result" event usage
func extractStreamData(line []byte) (resultText, assistantText string, inputTokens, outputTokens int) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(line, &event); err != nil {
		return "", "", 0, 0
	}

	var eventType string
	if err := json.Unmarshal(event["type"], &eventType); err != nil {
		return "", "", 0, 0
	}

	switch eventType {
	case "result":
		var result struct {
			Result string `json:"result"`
			Usage  struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(line, &result); err == nil {
			resultText = result.Result
			inputTokens = result.Usage.InputTokens
			outputTokens = result.Usage.OutputTokens
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
			assistantText = sb.String()
		}
	}

	return resultText, assistantText, inputTokens, outputTokens
}

// KillProcessGroup sends SIGKILL to the entire process group.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
