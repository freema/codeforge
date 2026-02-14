package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
)

// RateLimiter implements Redis-based sliding window rate limiting.
type RateLimiter struct {
	redis  *redisclient.Client
	limit  int
	window time.Duration
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter(rdb *redisclient.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		redis:  rdb,
		limit:  limit,
		window: window,
	}
}

// Middleware returns an HTTP middleware that enforces rate limits per Bearer token.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := extractClientID(r)
			if clientID == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfter := rl.allow(r, clientID)
			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":   "rate_limit_exceeded",
					"message": fmt.Sprintf("rate limit exceeded, retry after %ds", int(retryAfter.Seconds())),
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(r *http.Request, clientID string) (bool, time.Duration) {
	ctx := r.Context()
	key := rl.redis.Key("ratelimit", hashToken(clientID))

	now := time.Now().UnixMilli()
	windowStart := now - rl.window.Milliseconds()
	member := strconv.FormatInt(now, 10)

	pipe := rl.redis.Unwrap().Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
	countCmd := pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: member})
	pipe.Expire(ctx, key, rl.window)
	_, _ = pipe.Exec(ctx)

	count := countCmd.Val()
	if count >= int64(rl.limit) {
		retryAfter := rl.window / time.Duration(rl.limit)
		return false, retryAfter
	}
	return true, 0
}

func extractClientID(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if auth == token {
		return ""
	}
	return token
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:8])
}
