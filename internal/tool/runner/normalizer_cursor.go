package runner

import (
	"encoding/json"
)

// CursorNormalizer converts Cursor CLI stream-json events into NormalizedEvent.
type CursorNormalizer struct{}

// NewCursorNormalizer creates a normalizer for Cursor CLI stream-json output.
func NewCursorNormalizer() *CursorNormalizer {
	return &CursorNormalizer{}
}

// Normalize parses a Cursor CLI stream-json line and returns NormalizedEvents.
func (n *CursorNormalizer) Normalize(line []byte) []*NormalizedEvent {
	var envelope struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}

	raw := make(json.RawMessage, len(line))
	copy(raw, line)

	switch envelope.Type {
	case "assistant":
		return n.normalizeAssistant(line, raw)
	case "tool_call":
		return n.normalizeToolCall(envelope.Subtype, raw)
	case "result":
		return n.normalizeResult(envelope.Subtype, raw)
	default:
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "cursor",
			Raw:  raw,
		}}
	}
}

func (n *CursorNormalizer) normalizeAssistant(line []byte, raw json.RawMessage) []*NormalizedEvent {
	var msg struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "cursor",
			Raw:  raw,
		}}
	}

	var text string
	for _, c := range msg.Message.Content {
		if c.Type == "text" && c.Text != "" {
			text += c.Text
		}
	}

	if text == "" {
		return []*NormalizedEvent{{
			Type: EventSystem,
			CLI:  "cursor",
			Raw:  raw,
		}}
	}

	return []*NormalizedEvent{{
		Type:    EventText,
		Content: text,
		CLI:     "cursor",
		Raw:     raw,
	}}
}

func (n *CursorNormalizer) normalizeToolCall(subtype string, raw json.RawMessage) []*NormalizedEvent {
	if subtype == "completed" {
		return []*NormalizedEvent{{
			Type: EventToolResult,
			CLI:  "cursor",
			Raw:  raw,
		}}
	}
	return []*NormalizedEvent{{
		Type: EventToolUse,
		CLI:  "cursor",
		Raw:  raw,
	}}
}

func (n *CursorNormalizer) normalizeResult(subtype string, raw json.RawMessage) []*NormalizedEvent {
	if subtype == "error" {
		return []*NormalizedEvent{{
			Type: EventError,
			CLI:  "cursor",
			Raw:  raw,
		}}
	}
	return []*NormalizedEvent{{
		Type: EventResult,
		CLI:  "cursor",
		Raw:  raw,
	}}
}
