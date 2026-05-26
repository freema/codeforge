package runner

import (
	"testing"
)

func TestExtractCursorStreamData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
	}{
		{
			name:     "result success",
			input:    `{"type":"result","subtype":"success","result":"Task completed successfully","duration_ms":5000}`,
			wantText: "Task completed successfully",
		},
		{
			name:     "result error ignored",
			input:    `{"type":"result","subtype":"error","result":"Something failed"}`,
			wantText: "",
		},
		{
			name:     "assistant event ignored",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}`,
			wantText: "",
		},
		{
			name:     "system event ignored",
			input:    `{"type":"system","subtype":"init","cwd":"/workspace"}`,
			wantText: "",
		},
		{
			name:     "invalid JSON",
			input:    `{not valid}`,
			wantText: "",
		},
		{
			name:     "empty input",
			input:    ``,
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCursorStreamData([]byte(tt.input))
			if got != tt.wantText {
				t.Errorf("extractCursorStreamData() = %q, want %q", got, tt.wantText)
			}
		})
	}
}
