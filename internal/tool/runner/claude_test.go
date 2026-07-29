package runner

import "testing"

func TestExtractStreamData(t *testing.T) {
	tests := []struct {
		name string
		line string
		want claudeStreamData
	}{
		{
			name: "result event with full usage, cost and turns",
			line: `{"type":"result","subtype":"success","result":"Done.","total_cost_usd":0.4235,"num_turns":7,` +
				`"usage":{"input_tokens":1200,"output_tokens":450,"cache_read_input_tokens":30000,"cache_creation_input_tokens":5000}}`,
			want: claudeStreamData{
				ResultText:               "Done.",
				InputTokens:              1200,
				OutputTokens:             450,
				CacheReadInputTokens:     30000,
				CacheCreationInputTokens: 5000,
				CostUSD:                  0.4235,
				NumTurns:                 7,
			},
		},
		{
			name: "result event without cache or cost fields",
			line: `{"type":"result","result":"ok","usage":{"input_tokens":10,"output_tokens":20}}`,
			want: claudeStreamData{
				ResultText:   "ok",
				InputTokens:  10,
				OutputTokens: 20,
			},
		},
		{
			name: "result event with error_during_execution and empty result",
			line: `{"type":"result","subtype":"error_during_execution","result":"","total_cost_usd":0.01,"num_turns":2,"usage":{"input_tokens":5,"output_tokens":0}}`,
			want: claudeStreamData{
				InputTokens: 5,
				CostUSD:     0.01,
				NumTurns:    2,
			},
		},
		{
			name: "assistant text event",
			line: `{"type":"assistant","message":{"content":[{"type":"text","text":"working on it"}]}}`,
			want: claudeStreamData{
				AssistantText: "working on it",
			},
		},
		{
			name: "assistant event with tool_use content only",
			line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`,
			want: claudeStreamData{},
		},
		{
			name: "system event without session id ignored",
			line: `{"type":"system","subtype":"init"}`,
			want: claudeStreamData{},
		},
		{
			name: "system init event carries session id",
			line: `{"type":"system","subtype":"init","session_id":"11111111-2222-3333-4444-555555555555","cwd":"/work"}`,
			want: claudeStreamData{
				SessionID: "11111111-2222-3333-4444-555555555555",
			},
		},
		{
			name: "result event carries session id",
			line: `{"type":"result","result":"ok","session_id":"aaaa-bbbb","usage":{"input_tokens":1,"output_tokens":2}}`,
			want: claudeStreamData{
				ResultText:   "ok",
				SessionID:    "aaaa-bbbb",
				InputTokens:  1,
				OutputTokens: 2,
			},
		},
		{
			name: "invalid JSON",
			line: `{not valid json}`,
			want: claudeStreamData{},
		},
		{
			name: "missing type field",
			line: `{"result":"orphan"}`,
			want: claudeStreamData{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStreamData([]byte(tt.line))
			if got != tt.want {
				t.Errorf("extractStreamData() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// argValue returns the argument following flag, or "" if flag is absent/last.
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestClaudeRunner_BuildArgsResume(t *testing.T) {
	tests := []struct {
		name       string
		runner     *ClaudeRunner
		opts       RunOptions
		wantResume string // expected value after --resume, "" = flag must be absent
		wantBare   bool
	}{
		{
			name:       "no resume id omits flag",
			runner:     NewClaudeRunner("claude"),
			opts:       RunOptions{Prompt: "p"},
			wantResume: "",
		},
		{
			name:       "resume id appends --resume with id",
			runner:     NewClaudeRunner("claude"),
			opts:       RunOptions{Prompt: "p", ResumeSessionID: "sess-123"},
			wantResume: "sess-123",
		},
		{
			name:       "claude-agent inherits --resume alongside --bare",
			runner:     NewClaudeAgentRunner("claude"),
			opts:       RunOptions{Prompt: "p", ResumeSessionID: "sess-456"},
			wantResume: "sess-456",
			wantBare:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.runner.buildArgs(tt.opts)
			got := argValue(args, "--resume")
			if got != tt.wantResume {
				t.Errorf("--resume value = %q, want %q (args %v)", got, tt.wantResume, args)
			}
			if tt.wantResume == "" && containsArg(args, "--resume") {
				t.Errorf("unexpected --resume flag in %v", args)
			}
			if tt.wantBare && !containsArg(args, "--bare") {
				t.Errorf("expected --bare in %v", args)
			}
		})
	}
}
