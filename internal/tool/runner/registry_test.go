package runner

import (
	"context"
	"encoding/json"
	"testing"
)

type mockRunner struct{}

func (m *mockRunner) Run(_ context.Context, _ RunOptions) (*RunResult, error) {
	return &RunResult{Output: "mock"}, nil
}

func TestRegistry_GetWithMeta(t *testing.T) {
	reg := NewRegistry("default-cli")

	normFactory := func() StreamNormalizer { return NewClaudeNormalizer() }

	reg.Register("default-cli", &mockRunner{}, RunnerMeta{
		NormalizerFactory: normFactory,
		AIProvider:        "anthropic",
	})
	reg.Register("other-cli", &mockRunner{}, RunnerMeta{
		NormalizerFactory: func() StreamNormalizer { return NewCodexNormalizer() },
		AIProvider:        "openai",
	})

	tests := []struct {
		name       string
		cliName    string
		wantErr    bool
		wantProvider string
	}{
		{
			name:         "explicit name",
			cliName:      "other-cli",
			wantProvider: "openai",
		},
		{
			name:         "empty resolves to default",
			cliName:      "",
			wantProvider: "anthropic",
		},
		{
			name:    "unknown name errors",
			cliName: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, meta, err := reg.GetWithMeta(tt.cliName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r == nil {
				t.Fatal("runner should not be nil")
			}
			if meta.AIProvider != tt.wantProvider {
				t.Errorf("AIProvider = %q, want %q", meta.AIProvider, tt.wantProvider)
			}
			if meta.NormalizerFactory == nil {
				t.Error("NormalizerFactory should not be nil")
			}
		})
	}
}

func TestRegistry_Available(t *testing.T) {
	reg := NewRegistry("a")
	reg.Register("a", &mockRunner{}, RunnerMeta{AIProvider: "x"})
	reg.Register("b", &mockRunner{}, RunnerMeta{AIProvider: "y"})

	avail := reg.Available()
	if len(avail) != 2 {
		t.Fatalf("got %d available, want 2", len(avail))
	}

	names := map[string]bool{}
	for _, n := range avail {
		names[n] = true
	}
	if !names["a"] || !names["b"] {
		t.Errorf("expected [a, b], got %v", avail)
	}
}

// Ensure mockRunner satisfies Runner at compile time.
var _ Runner = (*mockRunner)(nil)

// Suppress unused import warning for json package used in other test files.
var _ = json.RawMessage{}
