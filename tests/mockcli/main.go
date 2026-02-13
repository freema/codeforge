// Mock Claude CLI for integration testing.
// Simulates stream-json output format.
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
	outputFormat := flag.String("output-format", "", "output format")
	_ = flag.Bool("verbose", false, "verbose")
	_ = flag.String("permission-mode", "", "permission mode")
	_ = flag.String("model", "", "model")
	_ = flag.Int("max-turns", 0, "max turns")
	_ = flag.String("max-budget-usd", "", "max budget")
	flag.Parse()

	_ = outputFormat

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

	// Simulate stream-json output
	events := []map[string]interface{}{
		{
			"type": "content_block_delta",
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "Task completed successfully. ",
			},
		},
		{
			"type": "content_block_delta",
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": fmt.Sprintf("Processed prompt: %s", truncate(*prompt, 100)),
			},
		},
		{
			"type": "message_delta",
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
