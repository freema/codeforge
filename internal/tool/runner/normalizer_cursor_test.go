package runner

import (
	"testing"
)

func TestCursorNormalizer_Normalize(t *testing.T) {
	n := NewCursorNormalizer()

	tests := []struct {
		name        string
		input       string
		wantNil     bool
		wantType    NormalizedEventType
		wantContent string
		wantCLI     string
	}{
		{
			name:        "assistant text",
			input:       `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			wantType:    EventText,
			wantContent: "Hello world",
			wantCLI:     "cursor",
		},
		{
			name:     "assistant empty content",
			input:    `{"type":"assistant","message":{"content":[]}}`,
			wantType: EventSystem,
			wantCLI:  "cursor",
		},
		{
			name:     "tool_call started",
			input:    `{"type":"tool_call","subtype":"started","call_id":"call_1","tool_call":{"name":"Read","input":{}}}`,
			wantType: EventToolUse,
			wantCLI:  "cursor",
		},
		{
			name:     "tool_call completed",
			input:    `{"type":"tool_call","subtype":"completed","call_id":"call_1","tool_call":{"name":"Read","output":"file content"}}`,
			wantType: EventToolResult,
			wantCLI:  "cursor",
		},
		{
			name:     "result success",
			input:    `{"type":"result","subtype":"success","result":"Task completed","duration_ms":5000,"session_id":"sess_123"}`,
			wantType: EventResult,
			wantCLI:  "cursor",
		},
		{
			name:     "result error",
			input:    `{"type":"result","subtype":"error","result":"Something failed"}`,
			wantType: EventError,
			wantCLI:  "cursor",
		},
		{
			name:     "system init",
			input:    `{"type":"system","subtype":"init","cwd":"/workspace","model":"composer-2"}`,
			wantType: EventSystem,
			wantCLI:  "cursor",
		},
		{
			name:     "user event maps to system",
			input:    `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"fix the bug"}]}}`,
			wantType: EventSystem,
			wantCLI:  "cursor",
		},
		{
			name:    "invalid JSON returns nil",
			input:   `{not valid json}`,
			wantNil: true,
		},
		{
			name:    "empty input returns nil",
			input:   ``,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := n.Normalize([]byte(tt.input))

			if tt.wantNil {
				if len(results) != 0 {
					t.Fatalf("expected nil/empty, got %d events", len(results))
				}
				return
			}

			if len(results) != 1 {
				t.Fatalf("got %d events, want 1", len(results))
			}

			result := results[0]

			if result.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", result.Type, tt.wantType)
			}

			if result.Content != tt.wantContent {
				t.Errorf("Content = %q, want %q", result.Content, tt.wantContent)
			}

			if result.CLI != tt.wantCLI {
				t.Errorf("CLI = %q, want %q", result.CLI, tt.wantCLI)
			}

			if result.Raw == nil {
				t.Error("Raw should not be nil")
			}
		})
	}
}

func TestCursorNormalizer_MultipleTextBlocks(t *testing.T) {
	n := NewCursorNormalizer()

	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}}`
	results := n.Normalize([]byte(input))

	if len(results) != 1 {
		t.Fatalf("got %d events, want 1", len(results))
	}

	if results[0].Content != "Hello world" {
		t.Errorf("Content = %q, want %q", results[0].Content, "Hello world")
	}
}
