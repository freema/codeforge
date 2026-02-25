package runner

import (
	"path/filepath"
	"testing"
)

func TestExtractCodexStreamData(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantText     string
		wantInTokens int
		wantOutTokens int
	}{
		{
			name:  "agent_message item.completed",
			input: `{"type":"item.completed","item":{"type":"agent_message","text":"I fixed the bug."}}`,
			wantText: "I fixed the bug.",
		},
		{
			name:          "turn.completed with usage",
			input:         `{"type":"turn.completed","usage":{"input_tokens":24763,"output_tokens":122}}`,
			wantInTokens:  24763,
			wantOutTokens: 122,
		},
		{
			name:  "thread.started ignored",
			input: `{"type":"thread.started","thread_id":"thread_abc123"}`,
		},
		{
			name:  "turn.started ignored",
			input: `{"type":"turn.started"}`,
		},
		{
			name:  "command_execution item ignored",
			input: `{"type":"item.completed","item":{"type":"command_execution","command":"npm test","exit_code":0}}`,
		},
		{
			name:  "invalid JSON",
			input: `{not valid json}`,
		},
		{
			name:  "empty input",
			input: ``,
		},
		{
			name:  "agent_message with empty text",
			input: `{"type":"item.completed","item":{"type":"agent_message","text":""}}`,
		},
		{
			name:          "turn.completed with zero usage",
			input:         `{"type":"turn.completed","usage":{"input_tokens":0,"output_tokens":0}}`,
			wantInTokens:  0,
			wantOutTokens: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, inTokens, outTokens := extractCodexStreamData([]byte(tt.input))

			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if inTokens != tt.wantInTokens {
				t.Errorf("inputTokens = %d, want %d", inTokens, tt.wantInTokens)
			}
			if outTokens != tt.wantOutTokens {
				t.Errorf("outputTokens = %d, want %d", outTokens, tt.wantOutTokens)
			}
		})
	}
}

func TestNewCodexRunner_PathResolution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantBare bool // true = path should remain unchanged (bare name)
	}{
		{
			name:     "bare name stays as-is",
			input:    "codex",
			wantBare: true,
		},
		{
			name:     "relative path gets resolved",
			input:    "./bin/codex",
			wantBare: false,
		},
		{
			name:     "absolute path stays absolute",
			input:    "/usr/local/bin/codex",
			wantBare: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewCodexRunner(tt.input)
			if tt.wantBare {
				if r.binaryPath != tt.input {
					t.Errorf("binaryPath = %q, want %q (bare)", r.binaryPath, tt.input)
				}
			} else {
				if !filepath.IsAbs(r.binaryPath) {
					t.Errorf("binaryPath = %q, want absolute path", r.binaryPath)
				}
			}
		})
	}
}
