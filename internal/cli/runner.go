package cli

import (
	"context"
	"encoding/json"
	"time"
)

// Runner is the interface for CLI tool execution.
type Runner interface {
	Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}

// RunOptions configures a CLI run.
type RunOptions struct {
	Prompt       string
	WorkDir      string
	Model        string
	APIKey       string
	MaxTurns     int
	MaxBudgetUSD float64
	OnEvent      func(event json.RawMessage)
}

// RunResult holds the output of a CLI run.
type RunResult struct {
	Output       string
	ExitCode     int
	Duration     time.Duration
	InputTokens  int
	OutputTokens int
}
