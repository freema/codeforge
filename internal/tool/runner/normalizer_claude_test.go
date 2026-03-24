package runner

import (
	"testing"
)

func TestClaudeNormalizer_Normalize(t *testing.T) {
	n := NewClaudeNormalizer()

	tests := []struct {
		name string
		input string
		wantNil bool
		want    []struct {
			typ     NormalizedEventType
			content string
		}
		wantCLI string
	}{
		{
			name:  "assistant text",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventText, "Hello world"}},
			wantCLI: "claude-code",
		},
		{
			name:  "assistant thinking",
			input: `{"type":"assistant","message":{"content":[{"type":"thinking","text":"Let me think..."}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventThinking, "Let me think..."}},
			wantCLI: "claude-code",
		},
		{
			name:  "assistant tool_use",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool_1","name":"Read","input":{}}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventToolUse, ""}},
			wantCLI: "claude-code",
		},
		{
			name:  "assistant tool_result",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_result","tool_use_id":"tool_1"}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventToolResult, ""}},
			wantCLI: "claude-code",
		},
		{
			name:  "assistant multiple text blocks concatenated",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventText, "Hello world"}},
			wantCLI: "claude-code",
		},
		{
			name:  "result event",
			input: `{"type":"result","result":"Task completed successfully","usage":{"input_tokens":100,"output_tokens":50}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventResult, ""}},
			wantCLI: "claude-code",
		},
		{
			name:  "result error_during_execution",
			input: `{"type":"result","subtype":"error_during_execution","result":"","usage":{"input_tokens":100,"output_tokens":50}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventError, ""}},
			wantCLI: "claude-code",
		},
		{
			name:  "unknown type maps to system",
			input: `{"type":"init","session_id":"abc123"}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventSystem, ""}},
			wantCLI: "claude-code",
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
			name:  "assistant with empty content array",
			input: `{"type":"assistant","message":{"content":[]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{{EventSystem, ""}},
			wantCLI: "claude-code",
		},
		// Multi-block test cases
		{
			name:  "text + tool_use produces two events",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.ts"}}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{
				{EventText, "Let me check."},
				{EventToolUse, ""},
			},
			wantCLI: "claude-code",
		},
		{
			name:  "multiple tool_use produces two events",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.ts"}},{"type":"tool_use","id":"t2","name":"Grep","input":{"pattern":"foo"}}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{
				{EventToolUse, ""},
				{EventToolUse, ""},
			},
			wantCLI: "claude-code",
		},
		{
			name:  "thinking + text + tool_use produces three events",
			input: `{"type":"assistant","message":{"content":[{"type":"thinking","text":"Hmm..."},{"type":"text","text":"I will read it."},{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}`,
			want: []struct {
				typ     NormalizedEventType
				content string
			}{
				{EventThinking, "Hmm..."},
				{EventText, "I will read it."},
				{EventToolUse, ""},
			},
			wantCLI: "claude-code",
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

			if len(results) != len(tt.want) {
				t.Fatalf("got %d events, want %d", len(results), len(tt.want))
			}

			for i, w := range tt.want {
				r := results[i]
				if r.Type != w.typ {
					t.Errorf("event[%d].Type = %q, want %q", i, r.Type, w.typ)
				}
				if r.Content != w.content {
					t.Errorf("event[%d].Content = %q, want %q", i, r.Content, w.content)
				}
				if tt.wantCLI != "" && r.CLI != tt.wantCLI {
					t.Errorf("event[%d].CLI = %q, want %q", i, r.CLI, tt.wantCLI)
				}
				if r.Raw == nil {
					t.Errorf("event[%d].Raw should not be nil", i)
				}
			}
		})
	}
}

func TestClaudeNormalizer_ToolUseSyntheticRaw(t *testing.T) {
	n := NewClaudeNormalizer()

	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.ts"}},{"type":"tool_use","id":"t2","name":"Grep","input":{"pattern":"foo"}}]}}`
	results := n.Normalize([]byte(input))

	if len(results) != 2 {
		t.Fatalf("got %d events, want 2", len(results))
	}

	// Each tool_use event should have a synthetic Raw with that specific tool in content[0]
	for i, r := range results {
		if r.Type != EventToolUse {
			t.Errorf("event[%d].Type = %q, want tool_use", i, r.Type)
		}
		// The Raw should be valid JSON and parseable
		if r.Raw == nil {
			t.Fatalf("event[%d].Raw is nil", i)
		}
		// Verify the synthetic Raw is different from the original (each has one tool)
		if string(results[0].Raw) == string(results[1].Raw) {
			t.Error("both tool_use events have identical Raw, expected different synthetic Raw per tool")
		}
	}
}
