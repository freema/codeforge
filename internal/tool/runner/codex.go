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
	"time"
)

// CodexRunner executes OpenAI Codex CLI.
type CodexRunner struct {
	binaryPath string
}

// NewCodexRunner creates a runner for the Codex CLI.
// If binaryPath contains a directory separator, it is resolved to an
// absolute path so it remains valid when cmd.Dir is set to the session
// workspace. Bare command names (e.g. "codex") are left as-is so
// exec.Command looks them up via PATH.
func NewCodexRunner(binaryPath string) *CodexRunner {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		if abs, err := filepath.Abs(binaryPath); err == nil {
			binaryPath = abs
		}
	}
	return &CodexRunner{binaryPath: binaryPath}
}

// Run executes Codex CLI with JSON output, calling OnEvent for each line.
func (c *CodexRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	// --full-auto = --sandbox workspace-write + auto-approve on-request.
	// We use danger-full-access instead because Codex's Landlock sandbox
	// does not work inside Docker (missing kernel support / capabilities).
	// The Docker container itself provides the isolation.
	// danger-full-access implies no approval prompts, so no extra flag needed.
	args := []string{
		"exec",
		"--json",
		"--sandbox", "danger-full-access",
		"--skip-git-repo-check",
	}
	if opts.WorkDir != "" {
		args = append(args, "--cd", opts.WorkDir)
	}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	// MaxTurns, MaxBudgetUSD, AllowedTools are silently ignored — Codex does not support them.

	// If AppendSystemPrompt is set, prepend it to the prompt (Codex has no system prompt flag).
	prompt := opts.Prompt
	if opts.AppendSystemPrompt != "" {
		prompt = opts.AppendSystemPrompt + "\n\n---\n\n" + prompt
	}

	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	cmd.Dir = opts.WorkDir

	// Build environment. If running as root and a "codeforge" user exists,
	// drop privileges via gosu.
	baseEnv := os.Environ()
	if os.Getuid() == 0 {
		if u, err := user.Lookup("codeforge"); err == nil {
			uid, _ := strconv.ParseUint(u.Uid, 10, 32)
			gid, _ := strconv.ParseUint(u.Gid, 10, 32)

			if gosuPath, gosuErr := exec.LookPath("gosu"); gosuErr == nil {
				gosuArgs := append([]string{u.Username, cmd.Path}, cmd.Args[1:]...)
				cmd = exec.CommandContext(ctx, gosuPath, gosuArgs...)
				cmd.Dir = opts.WorkDir
				slog.Debug("dropping privileges for codex CLI via gosu", "uid", uid, "gid", gid)
			} else {
				slog.Debug("gosu not found, running codex CLI as root")
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

	// Codex exec reads CODEX_API_KEY (not OPENAI_API_KEY).
	// Per-session key takes priority; otherwise propagate OPENAI_API_KEY → CODEX_API_KEY
	// so the operator only needs to set one env var.
	if opts.APIKey != "" {
		cmd.Env = append(baseEnv, "CODEX_API_KEY="+opts.APIKey)
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cmd.Env = append(baseEnv, "CODEX_API_KEY="+key)
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
		return nil, fmt.Errorf("starting codex CLI: %w", err)
	}

	slog.Info("codex CLI started", "pid", cmd.Process.Pid, "work_dir", opts.WorkDir)

	// Read JSONL: each line is a complete JSON object
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var resultText string
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
		text, iTokens, oTokens := extractCodexStreamData(line)
		if text != "" {
			resultText = text
		}
		inputTokens += iTokens
		outputTokens += oTokens
	}

	err = cmd.Wait()
	duration := time.Since(startTime)

	result := &RunResult{
		Output:       resultText,
		ExitCode:     -1,
		Duration:     duration,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		slog.Warn("codex CLI exited with error",
			"exit_code", result.ExitCode,
			"stderr", stderrBuf.String(),
			"duration", duration,
		)
		return result, fmt.Errorf("codex CLI exited with code %d: %w", result.ExitCode, err)
	}

	slog.Info("codex CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)

	return result, nil
}

// extractCodexStreamData parses a Codex JSONL event for result text and usage.
//
// Codex emits events like:
//
//	{"type":"item.completed","item":{"type":"agent_message","text":"Done."}}
//	{"type":"turn.completed","usage":{"input_tokens":24763,"output_tokens":122}}
//
// Returns:
//   - text: from "item.completed" events with item.type == "agent_message"
//   - inputTokens, outputTokens: from "turn.completed" usage
func extractCodexStreamData(line []byte) (text string, inputTokens, outputTokens int) {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return "", 0, 0
	}

	switch event.Type {
	case "item.completed":
		if event.Item.Type == "agent_message" {
			text = event.Item.Text
		}
	case "turn.completed":
		inputTokens = event.Usage.InputTokens
		outputTokens = event.Usage.OutputTokens
	}

	return text, inputTokens, outputTokens
}
