package runner

import (
	"encoding/json"
	"strings"
)

// ClaudeNormalizer converts Claude Code stream-json events into NormalizedEvent.
type ClaudeNormalizer struct{}

// NewClaudeNormalizer creates a normalizer for Claude Code stream-json output.
func NewClaudeNormalizer() *ClaudeNormalizer {
	return &ClaudeNormalizer{}
}

// Normalize parses a Claude Code stream-json line and returns a NormalizedEvent.
// Returns nil for lines that cannot be parsed.
func (n *ClaudeNormalizer) Normalize(line []byte) *NormalizedEvent {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}

	raw := make(json.RawMessage, len(line))
	copy(raw, line)

	switch envelope.Type {
	case "assistant":
		return n.normalizeAssistant(line, raw)
	case "result":
		return n.normalizeResult(line, raw)
	default:
		return &NormalizedEvent{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}
	}
}

func (n *ClaudeNormalizer) normalizeAssistant(line []byte, raw json.RawMessage) *NormalizedEvent {
	var msg struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return &NormalizedEvent{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}
	}

	// Determine event type from the content blocks.
	// A single assistant message may contain multiple content blocks of
	// different types. We pick the most significant one:
	//   tool_use > tool_result > thinking > text
	eventType := EventSystem
	var sb strings.Builder

	for _, c := range msg.Message.Content {
		switch c.Type {
		case "thinking":
			if eventType != EventToolUse && eventType != EventToolResult {
				eventType = EventThinking
			}
			sb.WriteString(c.Text)
		case "text":
			if eventType == EventSystem {
				eventType = EventText
			}
			sb.WriteString(c.Text)
		case "tool_use":
			eventType = EventToolUse
		case "tool_result":
			if eventType != EventToolUse {
				eventType = EventToolResult
			}
		}
	}

	return &NormalizedEvent{
		Type:    eventType,
		Content: sb.String(),
		CLI:     "claude-code",
		Raw:     raw,
	}
}

func (n *ClaudeNormalizer) normalizeResult(line []byte, raw json.RawMessage) *NormalizedEvent {
	var result struct {
		Result  string `json:"result"`
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(line, &result); err != nil {
		return &NormalizedEvent{
			Type: EventResult,
			CLI:  "claude-code",
			Raw:  raw,
		}
	}

	evtType := EventResult
	if result.Subtype == "error_during_execution" {
		evtType = EventError
	}

	return &NormalizedEvent{
		Type:    evtType,
		Content: result.Result,
		CLI:     "claude-code",
		Raw:     raw,
	}
}
