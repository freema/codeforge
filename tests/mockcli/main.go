// Mock Claude CLI for integration testing.
// Simulates Claude Code --output-format stream-json output.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	prompt := flag.String("p", "", "prompt")
	resume := flag.String("resume", "", "resume a previous CLI session by id")
	_ = flag.String("output-format", "", "output format")
	_ = flag.Bool("verbose", false, "verbose")
	_ = flag.Bool("bare", false, "bare agent mode")
	_ = flag.String("permission-mode", "", "permission mode")
	_ = flag.String("model", "", "model")
	_ = flag.Int("max-turns", 0, "max turns")
	_ = flag.String("max-budget-usd", "", "max budget")
	_ = flag.String("mcp-config", "", "mcp config path")
	_ = flag.String("append-system-prompt", "", "append system prompt")
	_ = flag.String("allowedTools", "", "allowed tools")
	flag.Parse()

	// Check for special prompts that trigger different behaviors
	switch {
	case *prompt == "TIMEOUT":
		// Simulate a long-running task
		time.Sleep(10 * time.Minute)
	case *prompt == "FAIL":
		fmt.Fprintln(os.Stderr, "mock CLI: simulated failure")
		os.Exit(1)
	case *prompt == "EMPTY":
		// No output
		os.Exit(0)
	}

	// Each run announces a fresh CLI session id (a resumed run forks to a new
	// id, mirroring real Claude Code behavior).
	sessionID := fmt.Sprintf("mock-sess-%d", time.Now().UnixNano())

	resultText := fmt.Sprintf("Task completed successfully. Processed prompt: %s", truncate(*prompt, 100))
	if *resume != "" {
		// Surface the resumed session id so E2E tests can assert the --resume path.
		resultText += fmt.Sprintf(" [resumed:%s]", *resume)
	}

	// Simulate the CLI doing actual work: create/modify a file in the working
	// directory so workspace diffs are non-empty.
	writeMockChange(*prompt)

	// Simulate Claude Code stream-json output:
	// 1. system init event (carries the CLI session id)
	// 2. assistant message with text content
	// 3. result event with text, usage, cost, and session id
	events := []map[string]interface{}{
		{
			"type":       "system",
			"subtype":    "init",
			"model":      "mock-claude",
			"session_id": sessionID,
		},
		{
			"type": "assistant",
			"message": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": resultText,
					},
				},
			},
		},
		{
			"type":           "result",
			"subtype":        "success",
			"result":         resultText,
			"session_id":     sessionID,
			"total_cost_usd": 0.05,
			"num_turns":      3,
			"usage": map[string]interface{}{
				"input_tokens":                150,
				"output_tokens":               50,
				"cache_read_input_tokens":     25,
				"cache_creation_input_tokens": 10,
			},
		},
	}

	enc := json.NewEncoder(os.Stdout)
	for _, event := range events {
		time.Sleep(50 * time.Millisecond) // Simulate streaming delay
		_ = enc.Encode(event)
	}
}

// writeMockChange appends a line to MOCK_CHANGES.md in the current working
// directory (the session workspace) so every successful run produces a
// non-empty git diff. Best-effort: failures are ignored so runs outside a
// writable workspace still succeed.
func writeMockChange(prompt string) {
	f, err := os.OpenFile("MOCK_CHANGES.md", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "mock change for prompt: %s\n", truncate(prompt, 60))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
