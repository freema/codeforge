package worker

import "testing"

func TestShouldResumeSession(t *testing.T) {
	tests := []struct {
		name         string
		cli          string
		cliSessionID string
		iteration    int
		freshClone   bool
		want         bool
	}{
		{
			name:         "claude-code follow-up with stored id resumes",
			cli:          "claude-code",
			cliSessionID: "sess-1",
			iteration:    2,
			want:         true,
		},
		{
			name:         "claude-agent follow-up with stored id resumes",
			cli:          "claude-agent",
			cliSessionID: "sess-2",
			iteration:    3,
			want:         true,
		},
		{
			name:         "first iteration never resumes",
			cli:          "claude-code",
			cliSessionID: "sess-1",
			iteration:    1,
			want:         false,
		},
		{
			name:      "missing stored id falls back to rebuilt context",
			cli:       "claude-code",
			iteration: 2,
			want:      false,
		},
		{
			name:         "fresh re-clone falls back to rebuilt context",
			cli:          "claude-code",
			cliSessionID: "sess-1",
			iteration:    2,
			freshClone:   true,
			want:         false,
		},
		{
			name:         "codex has no native resume",
			cli:          "codex",
			cliSessionID: "sess-1",
			iteration:    2,
			want:         false,
		},
		{
			name:         "cursor has no native resume",
			cli:          "cursor",
			cliSessionID: "sess-1",
			iteration:    2,
			want:         false,
		},
		{
			name:         "unknown cli never resumes",
			cli:          "some-cli",
			cliSessionID: "sess-1",
			iteration:    2,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldResumeSession(tt.cli, tt.cliSessionID, tt.iteration, tt.freshClone)
			if got != tt.want {
				t.Errorf("shouldResumeSession(%q, %q, %d, %v) = %v, want %v",
					tt.cli, tt.cliSessionID, tt.iteration, tt.freshClone, got, tt.want)
			}
		})
	}
}
