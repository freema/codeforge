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

// CursorRunner executes Cursor CLI (cursor-agent).
type CursorRunner struct {
	binaryPath string
}

// NewCursorRunner creates a runner for the Cursor CLI.
func NewCursorRunner(binaryPath string) *CursorRunner {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		if abs, err := filepath.Abs(binaryPath); err == nil {
			binaryPath = abs
		}
	}
	return &CursorRunner{binaryPath: binaryPath}
}

// Run executes Cursor CLI with stream-json output, calling OnEvent for each line.
func (c *CursorRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	prompt := opts.Prompt
	if opts.AppendSystemPrompt != "" {
		prompt = opts.AppendSystemPrompt + "\n\n---\n\n" + prompt
	}

	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--force",
	}
	if opts.WorkDir != "" {
		args = append(args, "--workspace", opts.WorkDir)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
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
				slog.Debug("dropping privileges for cursor CLI via gosu", "uid", uid, "gid", gid)
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

	configureGracefulKill(cmd)

	if opts.APIKey != "" {
		cmd.Env = append(baseEnv, "CURSOR_API_KEY="+opts.APIKey)
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
		return nil, fmt.Errorf("starting cursor CLI: %w", err)
	}

	slog.Info("cursor CLI started", "pid", cmd.Process.Pid, "work_dir", opts.WorkDir)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var resultText string

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

		text := extractCursorStreamData(line)
		if text != "" {
			resultText = text
		}
	}

	err = cmd.Wait()
	duration := time.Since(startTime)

	result := &RunResult{
		Output:   resultText,
		ExitCode: -1,
		Duration: duration,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		slog.Warn("cursor CLI exited with error",
			"exit_code", result.ExitCode,
			"stderr", stderrBuf.String(),
			"duration", duration,
		)
		return result, fmt.Errorf("cursor CLI exited with code %d: %w", result.ExitCode, err)
	}

	slog.Info("cursor CLI completed",
		"exit_code", result.ExitCode,
		"duration", duration,
	)

	return result, nil
}

// extractCursorStreamData parses a Cursor stream-json line for result text.
// Cursor does not expose token usage in stream events.
func extractCursorStreamData(line []byte) string {
	var event struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}

	if event.Type == "result" && event.Subtype == "success" {
		return event.Result
	}

	return ""
}
