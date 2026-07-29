package session

import "testing"

func TestAccumulateUsage(t *testing.T) {
	tests := []struct {
		name     string
		existing *UsageInfo
		delta    *UsageInfo
		want     *UsageInfo
	}{
		{
			name:     "both nil",
			existing: nil,
			delta:    nil,
			want:     nil,
		},
		{
			name:     "nil existing returns delta",
			existing: nil,
			delta:    &UsageInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 0.1, DurationSeconds: 30},
			want:     &UsageInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 0.1, DurationSeconds: 30},
		},
		{
			name:     "nil delta returns existing",
			existing: &UsageInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 0.1, DurationSeconds: 30},
			delta:    nil,
			want:     &UsageInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 0.1, DurationSeconds: 30},
		},
		{
			name: "sums all fields",
			existing: &UsageInfo{
				InputTokens:         1000,
				OutputTokens:        400,
				CacheReadTokens:     20000,
				CacheCreationTokens: 3000,
				CostUSD:             0.25,
				DurationSeconds:     60,
			},
			delta: &UsageInfo{
				InputTokens:         500,
				OutputTokens:        100,
				CacheReadTokens:     10000,
				CacheCreationTokens: 1000,
				CostUSD:             0.125,
				DurationSeconds:     45,
			},
			want: &UsageInfo{
				InputTokens:         1500,
				OutputTokens:        500,
				CacheReadTokens:     30000,
				CacheCreationTokens: 4000,
				CostUSD:             0.375,
				DurationSeconds:     105,
			},
		},
		{
			name:     "delta without new fields keeps existing cache and cost",
			existing: &UsageInfo{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 100, CostUSD: 0.05, DurationSeconds: 10},
			delta:    &UsageInfo{InputTokens: 20, OutputTokens: 10, DurationSeconds: 20},
			want:     &UsageInfo{InputTokens: 30, OutputTokens: 15, CacheReadTokens: 100, CostUSD: 0.05, DurationSeconds: 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccumulateUsage(tt.existing, tt.delta)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("AccumulateUsage() = %+v, want %+v", got, tt.want)
			}
			if got == nil {
				return
			}
			if *got != *tt.want {
				t.Errorf("AccumulateUsage() = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}
