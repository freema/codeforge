package runner

import "encoding/json"

// NormalizedEventType represents the type of a normalized stream event.
type NormalizedEventType string

const (
	EventThinking   NormalizedEventType = "thinking"
	EventText       NormalizedEventType = "text"
	EventToolUse    NormalizedEventType = "tool_use"
	EventToolResult NormalizedEventType = "tool_result"
	EventResult     NormalizedEventType = "result"
	EventError      NormalizedEventType = "error"
	EventSystem     NormalizedEventType = "system"
)

// NormalizedEvent is a CLI-agnostic stream event emitted to the client.
type NormalizedEvent struct {
	Type    NormalizedEventType `json:"type"`
	Content string              `json:"content,omitempty"`
	CLI     string              `json:"cli"`
	Raw     json.RawMessage     `json:"raw"`
}

// StreamNormalizer converts raw CLI-specific events into NormalizedEvent.
// Normalize returns nil or an empty slice for events that should be ignored.
type StreamNormalizer interface {
	Normalize(line []byte) []*NormalizedEvent
}
