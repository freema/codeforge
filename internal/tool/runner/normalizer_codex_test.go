package runner

import (
	"testing"
)

func TestCodexNormalizer_Normalize(t *testing.T) {
	n := NewCodexNormalizer()

	tests := []struct {
		name        string
		input       string
		wantNil     bool
		wantType    NormalizedEventType
		wantContent string
		wantCLI     string
	}{
		{
			name:        "agent_message item.completed",
			input:       `{"type":"item.completed","item":{"type":"agent_message","text":"I fixed the bug."}}`,
			wantType:    EventText,
			wantContent: "I fixed the bug.",
			wantCLI:     "codex",
		},
		{
			name:        "function_call item.completed",
			input:       `{"type":"item.completed","item":{"type":"function_call","name":"read_file","arguments":"{\"path\":\"src/main.go\"}","call_id":"call_123"}}`,
			wantType:    EventToolUse,
			wantContent: `read_file({"path":"src/main.go"})`,
			wantCLI:     "codex",
		},
		{
			name:        "function_call without arguments",
			input:       `{"type":"item.completed","item":{"type":"function_call","name":"list_files","call_id":"call_456"}}`,
			wantType:    EventToolUse,
			wantContent: "list_files",
			wantCLI:     "codex",
		},
		{
			name:        "function_call_output item.completed",
			input:       `{"type":"item.completed","item":{"type":"function_call_output","output":"package main\nfunc main() {}","call_id":"call_123"}}`,
			wantType:    EventToolResult,
			wantContent: "package main\nfunc main() {}",
			wantCLI:     "codex",
		},
		{
			name:        "command_execution item.completed",
			input:       `{"type":"item.completed","item":{"type":"command_execution","command":"npm test","exit_code":0}}`,
			wantType:    EventToolResult,
			wantContent: "npm test (exit 0)",
			wantCLI:     "codex",
		},
		{
			name:        "command_execution without exit_code",
			input:       `{"type":"item.completed","item":{"type":"command_execution","command":"go build ./..."}}`,
			wantType:    EventToolResult,
			wantContent: "go build ./...",
			wantCLI:     "codex",
		},
		{
			name:     "turn.completed",
			input:    `{"type":"turn.completed","usage":{"input_tokens":24763,"output_tokens":122}}`,
			wantType: EventResult,
			wantCLI:  "codex",
		},
		{
			name:     "thread.started maps to system",
			input:    `{"type":"thread.started","thread_id":"thread_abc123"}`,
			wantType: EventSystem,
			wantCLI:  "codex",
		},
		{
			name:     "turn.started maps to system",
			input:    `{"type":"turn.started"}`,
			wantType: EventSystem,
			wantCLI:  "codex",
		},
		{
			name:     "unknown item type maps to system",
			input:    `{"type":"item.completed","item":{"type":"unknown_type","text":"something"}}`,
			wantType: EventSystem,
			wantCLI:  "codex",
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
			name:     "agent_message with empty text",
			input:    `{"type":"item.completed","item":{"type":"agent_message","text":""}}`,
			wantType: EventText,
			wantCLI:  "codex",
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
