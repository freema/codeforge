package runner

import (
	"testing"
)

func TestClaudeNormalizer_Normalize(t *testing.T) {
	n := NewClaudeNormalizer()

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
			wantCLI:     "claude-code",
		},
		{
			name:        "assistant thinking",
			input:       `{"type":"assistant","message":{"content":[{"type":"thinking","text":"Let me think..."}]}}`,
			wantType:    EventThinking,
			wantContent: "Let me think...",
			wantCLI:     "claude-code",
		},
		{
			name:     "assistant tool_use",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool_1","name":"Read","input":{}}]}}`,
			wantType: EventToolUse,
			wantCLI:  "claude-code",
		},
		{
			name:     "assistant tool_result",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_result","tool_use_id":"tool_1"}]}}`,
			wantType: EventToolResult,
			wantCLI:  "claude-code",
		},
		{
			name:        "assistant multiple text blocks concatenated",
			input:       `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}}`,
			wantType:    EventText,
			wantContent: "Hello world",
			wantCLI:     "claude-code",
		},
		{
			name:        "result event",
			input:       `{"type":"result","result":"Task completed successfully","usage":{"input_tokens":100,"output_tokens":50}}`,
			wantType:    EventResult,
			wantContent: "Task completed successfully",
			wantCLI:     "claude-code",
		},
		{
			name:     "result error_during_execution",
			input:    `{"type":"result","subtype":"error_during_execution","result":"","usage":{"input_tokens":100,"output_tokens":50}}`,
			wantType: EventError,
			wantCLI:  "claude-code",
		},
		{
			name:     "unknown type maps to system",
			input:    `{"type":"init","session_id":"abc123"}`,
			wantType: EventSystem,
			wantCLI:  "claude-code",
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
		{
			name:     "assistant with empty content array",
			input:    `{"type":"assistant","message":{"content":[]}}`,
			wantType: EventSystem,
			wantCLI:  "claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.Normalize([]byte(tt.input))

			if tt.wantNil {
				if result != nil {
					t.Fatalf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			}

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
