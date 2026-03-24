package runner

import (
	"encoding/json"
)

// ClaudeNormalizer converts Claude Code stream-json events into NormalizedEvent.
type ClaudeNormalizer struct{}

// NewClaudeNormalizer creates a normalizer for Claude Code stream-json output.
func NewClaudeNormalizer() *ClaudeNormalizer {
	return &ClaudeNormalizer{}
}

// Normalize parses a Claude Code stream-json line and returns NormalizedEvents.
// A single assistant message may produce multiple events (e.g. text + tool_use).
// Returns nil for lines that cannot be parsed.
func (n *ClaudeNormalizer) Normalize(line []byte) []*NormalizedEvent {
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
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}}
	}
}

// normalizeAssistant splits an assistant message into separate events per content block.
// Text blocks are accumulated; each tool_use gets its own event with a synthetic Raw
// so the frontend can extract the tool name/input from content[0].
func (n *ClaudeNormalizer) normalizeAssistant(line []byte, raw json.RawMessage) []*NormalizedEvent {
	var msg struct {
		Message struct {
			Content []json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}}
	}

	if len(msg.Message.Content) == 0 {
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}}
	}

	var events []*NormalizedEvent
	var textBuf []byte // accumulates contiguous text blocks

	flushText := func() {
		if len(textBuf) > 0 {
			events = append(events, &NormalizedEvent{
				Type:    EventText,
				Content: string(textBuf),
				CLI:     "claude-code",
				Raw:     raw,
			})
			textBuf = nil
		}
	}

	for _, block := range msg.Message.Content {
		var header struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(block, &header); err != nil {
			continue
		}

		switch header.Type {
		case "text":
			textBuf = append(textBuf, header.Text...)

		case "thinking":
			flushText()
			events = append(events, &NormalizedEvent{
				Type:    EventThinking,
				Content: header.Text,
				CLI:     "claude-code",
				Raw:     raw,
			})

		case "tool_use":
			flushText()
			// Build a synthetic Raw containing only this tool_use block so
			// the frontend's extractToolFromRaw finds it at content[0].
			blockRaw, _ := json.Marshal(map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"content": []json.RawMessage{block},
				},
			})
			events = append(events, &NormalizedEvent{
				Type: EventToolUse,
				CLI:  "claude-code",
				Raw:  blockRaw,
			})

		case "tool_result":
			flushText()
			events = append(events, &NormalizedEvent{
				Type: EventToolResult,
				CLI:  "claude-code",
				Raw:  raw,
			})
		}
	}

	flushText()

	if len(events) == 0 {
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "claude-code",
			Raw:  raw,
		}}
	}

	return events
}

func (n *ClaudeNormalizer) normalizeResult(line []byte, raw json.RawMessage) []*NormalizedEvent {
	var msg struct {
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil
	}

	if msg.Subtype == "error_during_execution" {
		return []*NormalizedEvent{{
			Type: EventError,
			CLI:  "claude-code",
			Raw:  raw,
		}}
	}

	// Don't set Content — the same text was already emitted by the preceding
	// assistant message. We emit the result event only for its metadata
	// (cost, tokens, turns, duration) which the frontend extracts from Raw.
	return []*NormalizedEvent{{
		Type: EventResult,
		CLI:  "claude-code",
		Raw:  raw,
	}}
}
