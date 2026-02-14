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
	_ = flag.String("output-format", "", "output format")
	_ = flag.Bool("verbose", false, "verbose")
	_ = flag.String("permission-mode", "", "permission mode")
	_ = flag.String("model", "", "model")
	_ = flag.Int("max-turns", 0, "max turns")
	_ = flag.String("max-budget-usd", "", "max budget")
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

	resultText := fmt.Sprintf("Task completed successfully. Processed prompt: %s", truncate(*prompt, 100))

	// Simulate Claude Code stream-json output:
	// 1. system init event
	// 2. assistant message with text content
	// 3. result event with text and usage
	events := []map[string]interface{}{
		{
			"type":    "system",
			"subtype": "init",
			"model":   "mock-claude",
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
			"type":    "result",
			"subtype": "success",
			"result":  resultText,
			"usage": map[string]interface{}{
				"input_tokens":  150,
				"output_tokens": 50,
			},
		},
	}

	enc := json.NewEncoder(os.Stdout)
	for _, event := range events {
		time.Sleep(50 * time.Millisecond) // Simulate streaming delay
		enc.Encode(event)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
