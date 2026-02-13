package redisclient

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps go-redis with connection pooling and health check.
type Client struct {
	rdb    *redis.Client
	prefix string
}

// New creates a Redis client from a URL string (redis://...).
func New(url, prefix string) (*Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	opt.PoolSize = 10
	opt.MinIdleConns = 5
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second
	opt.MaxRetries = 3
	opt.MinRetryBackoff = 8 * time.Millisecond
	opt.MaxRetryBackoff = 512 * time.Millisecond

	rdb := redis.NewClient(opt)

	return &Client{rdb: rdb, prefix: prefix}, nil
}

// Ping checks Redis connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close shuts down the Redis connection pool.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Unwrap returns the underlying go-redis client for direct access.
func (c *Client) Unwrap() *redis.Client {
	return c.rdb
}

// Key returns a prefixed Redis key.
func (c *Client) Key(parts ...string) string {
	key := ""
	for i, p := range parts {
		if i > 0 {
			key += ":"
		}
		key += p
	}
	if c.prefix != "" {
		return c.prefix + key
	}
	return key
}
