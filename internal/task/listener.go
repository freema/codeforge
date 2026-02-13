package task

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
)

var listenerValidate = validator.New()

// RedisInputPayload is the JSON payload pushed to the Redis input list.
type RedisInputPayload struct {
	RepoURL       string      `json:"repo_url" validate:"required,url"`
	ProviderKey   string      `json:"provider_key,omitempty"`
	AccessToken   string      `json:"access_token,omitempty"`
	Prompt        string      `json:"prompt" validate:"required,max=102400"`
	CallbackURL   string      `json:"callback_url,omitempty" validate:"omitempty,url"`
	Config        *TaskConfig `json:"config,omitempty"`
	CorrelationID string      `json:"correlation_id,omitempty"`
}

// Listener consumes task payloads from a Redis list (primary input for ScopeBot).
type Listener struct {
	redis      *redisclient.Client
	service    *Service
	inputKey   string
	resultKeyPrefix string
}

// NewListener creates a Redis input listener.
func NewListener(redis *redisclient.Client, service *Service, inputKey string) *Listener {
	return &Listener{
		redis:           redis,
		service:         service,
		inputKey:        inputKey,
		resultKeyPrefix: "input:result:",
	}
}

// Start begins listening for task payloads from the Redis input list.
func (l *Listener) Start(ctx context.Context) {
	slog.Info("redis input listener started", "key", l.inputKey)

	inputKey := l.redis.Key(l.inputKey)

	for {
		result, err := l.redis.Unwrap().BLPop(ctx, 5*time.Second, inputKey).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // timeout
			}
			if ctx.Err() != nil {
				slog.Info("redis input listener shutting down")
				return
			}
			slog.Error("redis input pop failed", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		payload := result[1]
		l.handlePayload(ctx, payload)
	}
}

func (l *Listener) handlePayload(ctx context.Context, raw string) {
	var input RedisInputPayload
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		slog.Error("invalid redis input payload", "error", err, "payload", truncateLog(raw))
		return
	}

	if err := listenerValidate.Struct(input); err != nil {
		slog.Error("redis input validation failed", "error", err)
		return
	}

	req := CreateTaskRequest{
		RepoURL:     input.RepoURL,
		ProviderKey: input.ProviderKey,
		AccessToken: input.AccessToken,
		Prompt:      input.Prompt,
		CallbackURL: input.CallbackURL,
		Config:      input.Config,
	}

	t, err := l.service.Create(ctx, req)
	if err != nil {
		slog.Error("failed to create task from redis input", "error", err)
		return
	}

	slog.Info("task created from redis input", "task_id", t.ID, "correlation_id", input.CorrelationID)

	// Write result back for correlation
	if input.CorrelationID != "" {
		resultKey := l.redis.Key(l.resultKeyPrefix + input.CorrelationID)
		resultData, _ := json.Marshal(map[string]string{
			"task_id": t.ID,
			"status":  string(t.Status),
		})
		l.redis.Unwrap().Set(ctx, resultKey, string(resultData), 5*time.Minute)
	}
}

func truncateLog(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
