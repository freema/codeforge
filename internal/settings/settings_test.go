//go:build integration

package settings

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
)

// setupTestStore follows the same integration-test pattern as
// internal/session: connect to a real Redis, skip when unreachable.
func setupTestStore(t *testing.T) *Store {
	t.Helper()

	url := os.Getenv("CODEFORGE_REDIS__URL")
	if url == "" {
		url = "redis://localhost:6379"
	}

	rdb, err := redisclient.New(url, "test:settings:")
	if err != nil {
		t.Skipf("skipping: redis not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx); err != nil {
		rdb.Close()
		t.Skipf("skipping: redis not reachable: %v", err)
	}

	store := NewStore(rdb)

	t.Cleanup(func() {
		rdb.Unwrap().Del(context.Background(), store.reviewKey())
		rdb.Close()
	})

	return store
}

func TestStore_Review(t *testing.T) {
	tests := []struct {
		name string
		// set is applied in order before the final GetReview; empty slice
		// means the key is never written (redis.Nil path).
		set  []ReviewSettings
		want ReviewSettings
	}{
		{
			name: "not set returns zero value",
			set:  nil,
			want: ReviewSettings{},
		},
		{
			name: "set and get round-trip",
			set:  []ReviewSettings{{DefaultCLI: "codex", DefaultModel: "gpt-5.2-codex"}},
			want: ReviewSettings{DefaultCLI: "codex", DefaultModel: "gpt-5.2-codex"},
		},
		{
			name: "cli only, model empty",
			set:  []ReviewSettings{{DefaultCLI: "claude-code"}},
			want: ReviewSettings{DefaultCLI: "claude-code"},
		},
		{
			name: "overwrite replaces previous value",
			set: []ReviewSettings{
				{DefaultCLI: "codex", DefaultModel: "gpt-5.2-codex"},
				{DefaultCLI: "claude-code", DefaultModel: "claude-sonnet-4-6"},
			},
			want: ReviewSettings{DefaultCLI: "claude-code", DefaultModel: "claude-sonnet-4-6"},
		},
		{
			name: "zero values clear the override",
			set: []ReviewSettings{
				{DefaultCLI: "codex", DefaultModel: "gpt-5.2-codex"},
				{},
			},
			want: ReviewSettings{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupTestStore(t)
			ctx := context.Background()

			// Each subtest starts from a clean key.
			if err := store.redis.Unwrap().Del(ctx, store.reviewKey()).Err(); err != nil {
				t.Fatalf("Del: %v", err)
			}

			for _, rs := range tt.set {
				if err := store.SetReview(ctx, rs); err != nil {
					t.Fatalf("SetReview(%+v): %v", rs, err)
				}
			}

			got, err := store.GetReview(ctx)
			if err != nil {
				t.Fatalf("GetReview: %v", err)
			}
			if got != tt.want {
				t.Errorf("GetReview = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStore_GetReview_InvalidJSON(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	if err := store.redis.Unwrap().Set(ctx, store.reviewKey(), "{not-json", 0).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if _, err := store.GetReview(ctx); err == nil {
		t.Error("GetReview: expected error for invalid JSON, got nil")
	}
}
