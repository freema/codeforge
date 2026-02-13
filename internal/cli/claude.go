package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
func NewClaudeRunner(binaryPath string) *ClaudeRunner {
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
	cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+opts.APIKey)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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

	var lastResultText string
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
		text, iTokens, oTokens := extractStreamData(line)
		if text != "" {
			lastResultText += text
		}
		inputTokens += iTokens
		outputTokens += oTokens
	}

	err = cmd.Wait()
	duration := time.Since(startTime)

	result := &RunResult{
		Output:       lastResultText,
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

// extractStreamData parses a stream-json line for result text and usage info.
// Claude Code stream-json events include:
// - content_block_delta with text_delta: accumulate result text
// - message_delta with usage: extract token counts
func extractStreamData(line []byte) (text string, inputTokens, outputTokens int) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(line, &event); err != nil {
		return "", 0, 0
	}

	var eventType string
	if err := json.Unmarshal(event["type"], &eventType); err != nil {
		return "", 0, 0
	}

	switch eventType {
	case "content_block_delta":
		var delta struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(line, &delta); err == nil && delta.Delta.Type == "text_delta" {
			text = delta.Delta.Text
		}

	case "message_delta":
		var msg struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(line, &msg); err == nil {
			inputTokens = msg.Usage.InputTokens
			outputTokens = msg.Usage.OutputTokens
		}
	}

	return text, inputTokens, outputTokens
}

// KillProcessGroup sends SIGKILL to the entire process group.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
