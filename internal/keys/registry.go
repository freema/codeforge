package keys

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/redisclient"
)

// Key represents a stored access token.
type Key struct {
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Token     string    `json:"token,omitempty"` // only in create request, never in responses
	Scope     string    `json:"scope,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Registry manages encrypted access tokens in Redis.
type Registry struct {
	redis  *redisclient.Client
	crypto *crypto.Service
}

// NewRegistry creates a new key registry.
func NewRegistry(redis *redisclient.Client, cryptoSvc *crypto.Service) *Registry {
	return &Registry{redis: redis, crypto: cryptoSvc}
}

// Create registers a new key. Returns error if the name already exists for the provider.
func (r *Registry) Create(ctx context.Context, key Key) error {
	if key.Provider != "github" && key.Provider != "gitlab" {
		return apperror.Validation("provider must be 'github' or 'gitlab'")
	}

	redisKey := r.redisKey(key.Provider, key.Name)

	// Check uniqueness
	exists, err := r.redis.Unwrap().Exists(ctx, redisKey).Result()
	if err != nil {
		return fmt.Errorf("checking key existence: %w", err)
	}
	if exists > 0 {
		return apperror.Conflict("key '%s' already exists for provider '%s'", key.Name, key.Provider)
	}

	// Encrypt token
	encrypted, err := r.crypto.Encrypt(key.Token)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	fields := map[string]interface{}{
		"name":            key.Name,
		"provider":        key.Provider,
		"encrypted_token": encrypted,
		"scope":           key.Scope,
		"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := r.redis.Unwrap().HSet(ctx, redisKey, fields).Err(); err != nil {
		return fmt.Errorf("storing key: %w", err)
	}

	return nil
}

// List returns all keys (without tokens).
func (r *Registry) List(ctx context.Context) ([]Key, error) {
	pattern := r.redis.Key("keys", "*")
	var keys []Key

	iter := r.redis.Unwrap().Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		redisKey := iter.Val()
		fields, err := r.redis.Unwrap().HGetAll(ctx, redisKey).Result()
		if err != nil {
			continue
		}
		key := Key{
			Name:     fields["name"],
			Provider: fields["provider"],
			Scope:    fields["scope"],
		}
		if v := fields["created_at"]; v != "" {
			key.CreatedAt, _ = time.Parse(time.RFC3339Nano, v)
		}
		keys = append(keys, key)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scanning keys: %w", err)
	}

	if keys == nil {
		keys = []Key{}
	}

	return keys, nil
}

// Delete removes a key. Returns error if not found.
func (r *Registry) Delete(ctx context.Context, name string) error {
	// Try both providers
	for _, provider := range []string{"github", "gitlab"} {
		redisKey := r.redisKey(provider, name)
		deleted, err := r.redis.Unwrap().Del(ctx, redisKey).Result()
		if err != nil {
			return fmt.Errorf("deleting key: %w", err)
		}
		if deleted > 0 {
			return nil
		}
	}
	return apperror.NotFound("key '%s' not found", name)
}

// Resolve decrypts and returns the token for a given provider and key name.
func (r *Registry) Resolve(ctx context.Context, provider, name string) (string, error) {
	redisKey := r.redisKey(provider, name)
	encrypted, err := r.redis.Unwrap().HGet(ctx, redisKey, "encrypted_token").Result()
	if err == redis.Nil {
		return "", apperror.NotFound("key '%s' not found for provider '%s'", name, provider)
	}
	if err != nil {
		return "", fmt.Errorf("reading key: %w", err)
	}
	return r.crypto.Decrypt(encrypted)
}

func (r *Registry) redisKey(provider, name string) string {
	return r.redis.Key("keys", provider, name)
}
