package runner

import "encoding/json"

// CodexNormalizer converts Codex JSONL events into NormalizedEvent.
type CodexNormalizer struct{}

// NewCodexNormalizer creates a normalizer for Codex CLI JSON output.
func NewCodexNormalizer() *CodexNormalizer {
	return &CodexNormalizer{}
}

// Normalize parses a Codex JSONL line and returns a NormalizedEvent.
// Returns nil for lines that cannot be parsed.
func (n *CodexNormalizer) Normalize(line []byte) *NormalizedEvent {
	var envelope struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}

	raw := make(json.RawMessage, len(line))
	copy(raw, line)

	switch envelope.Type {
	case "item.completed":
		return n.normalizeItem(envelope.Item.Type, envelope.Item.Text, raw)
	case "turn.completed":
		return &NormalizedEvent{
			Type: EventResult,
			CLI:  "codex",
			Raw:  raw,
		}
	default:
		return &NormalizedEvent{
			Type: EventSystem,
			CLI:  "codex",
			Raw:  raw,
		}
	}
}

func (n *CodexNormalizer) normalizeItem(itemType, text string, raw json.RawMessage) *NormalizedEvent {
	switch itemType {
	case "agent_message":
		return &NormalizedEvent{
			Type:    EventText,
			Content: text,
			CLI:     "codex",
			Raw:     raw,
		}
	case "command_execution":
		return &NormalizedEvent{
			Type: EventToolResult,
			CLI:  "codex",
			Raw:  raw,
		}
	default:
		return &NormalizedEvent{
			Type: EventSystem,
			CLI:  "codex",
			Raw:  raw,
		}
	}
}
