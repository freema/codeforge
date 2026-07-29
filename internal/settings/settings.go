// Package settings provides runtime-configurable server settings stored in
// Redis, so operators can adjust behavior from the UI without a restart.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
)

// ReviewSettings holds runtime defaults for webhook-triggered PR reviews.
// Zero values mean "not set" — resolution falls back to the static config
// (code_review.default_cli) and the built-in default.
type ReviewSettings struct {
	DefaultCLI   string `json:"default_cli"`
	DefaultModel string `json:"default_model"`
}

// Store persists runtime settings in Redis as JSON strings.
type Store struct {
	redis *redisclient.Client
}

// NewStore creates a settings store over the shared Redis client.
func NewStore(redis *redisclient.Client) *Store {
	return &Store{redis: redis}
}

// reviewKey returns the Redis key for the review settings.
func (s *Store) reviewKey() string {
	return s.redis.Key("settings", "review")
}

// GetReview returns the stored review settings. A missing key is not an
// error — the zero value is returned.
func (s *Store) GetReview(ctx context.Context) (ReviewSettings, error) {
	raw, err := s.redis.Unwrap().Get(ctx, s.reviewKey()).Result()
	if errors.Is(err, redis.Nil) {
		return ReviewSettings{}, nil
	}
	if err != nil {
		return ReviewSettings{}, fmt.Errorf("getting review settings: %w", err)
	}

	var rs ReviewSettings
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		return ReviewSettings{}, fmt.Errorf("parsing review settings: %w", err)
	}
	return rs, nil
}

// SetReview stores the review settings as a JSON string (no TTL).
// Storing zero values clears the runtime override.
func (s *Store) SetReview(ctx context.Context, rs ReviewSettings) error {
	data, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("encoding review settings: %w", err)
	}
	if err := s.redis.Unwrap().Set(ctx, s.reviewKey(), string(data), 0).Err(); err != nil {
		return fmt.Errorf("storing review settings: %w", err)
	}
	return nil
}
