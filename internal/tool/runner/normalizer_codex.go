package runner

import (
	"encoding/json"
	"fmt"
)

// CodexNormalizer converts Codex JSONL events into NormalizedEvent.
type CodexNormalizer struct{}

// NewCodexNormalizer creates a normalizer for Codex CLI JSON output.
func NewCodexNormalizer() *CodexNormalizer {
	return &CodexNormalizer{}
}

// codexItem represents a Codex item.completed payload with all possible fields.
type codexItem struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Name      string `json:"name"`      // function_call: tool name
	Arguments string `json:"arguments"` // function_call: JSON args
	CallID    string `json:"call_id"`   // function_call / function_call_output
	Output    string `json:"output"`    // function_call_output: result text
	Command   string `json:"command"`   // command_execution
	ExitCode  *int   `json:"exit_code"` // command_execution
}

// Normalize parses a Codex JSONL line and returns NormalizedEvents.
// Returns nil for lines that cannot be parsed.
func (n *CodexNormalizer) Normalize(line []byte) []*NormalizedEvent {
	var envelope struct {
		Type string    `json:"type"`
		Item codexItem `json:"item"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}

	raw := make(json.RawMessage, len(line))
	copy(raw, line)

	switch envelope.Type {
	case "item.completed":
		return n.normalizeItem(&envelope.Item, raw)
	case "turn.completed":
		return []*NormalizedEvent{{
			Type: EventResult,
			CLI:  "codex",
			Raw:  raw,
		}}
	default:
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "codex",
			Raw:  raw,
		}}
	}
}

func (n *CodexNormalizer) normalizeItem(item *codexItem, raw json.RawMessage) []*NormalizedEvent {
	switch item.Type {
	case "agent_message":
		return []*NormalizedEvent{{
			Type:    EventText,
			Content: item.Text,
			CLI:     "codex",
			Raw:     raw,
		}}
	case "function_call":
		content := item.Name
		if item.Arguments != "" {
			content = fmt.Sprintf("%s(%s)", item.Name, item.Arguments)
		}
		return []*NormalizedEvent{{
			Type:    EventToolUse,
			Content: content,
			CLI:     "codex",
			Raw:     raw,
		}}
	case "function_call_output":
		return []*NormalizedEvent{{
			Type:    EventToolResult,
			Content: item.Output,
			CLI:     "codex",
			Raw:     raw,
		}}
	case "command_execution":
		content := item.Command
		if item.ExitCode != nil {
			content = fmt.Sprintf("%s (exit %d)", item.Command, *item.ExitCode)
		}
		return []*NormalizedEvent{{
			Type:    EventToolResult,
			Content: content,
			CLI:     "codex",
			Raw:     raw,
		}}
	default:
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "codex",
			Raw:  raw,
		}}
	}
}
