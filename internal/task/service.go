package task

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/crypto"
	gitpkg "github.com/freema/codeforge/internal/git"
	"github.com/freema/codeforge/internal/redisclient"
)

// Service manages task lifecycle and Redis persistence.
type Service struct {
	redis     *redisclient.Client
	crypto    *crypto.Service
	queueName string
	stateTTL  time.Duration
	resultTTL time.Duration
}

// NewService creates a new task service.
func NewService(redis *redisclient.Client, cryptoSvc *crypto.Service, queueName string, stateTTL, resultTTL time.Duration) *Service {
	return &Service{
		redis:     redis,
		crypto:    cryptoSvc,
		queueName: queueName,
		stateTTL:  stateTTL,
		resultTTL: resultTTL,
	}
}

// Create creates a new task in Redis and enqueues it for processing.
func (s *Service) Create(ctx context.Context, req CreateTaskRequest) (*Task, error) {
	t := &Task{
		ID:          uuid.New().String(),
		Status:      StatusPending,
		RepoURL:     req.RepoURL,
		ProviderKey: req.ProviderKey,
		AccessToken: req.AccessToken,
		Prompt:      req.Prompt,
		CallbackURL: req.CallbackURL,
		Config:      req.Config,
		Iteration:   1,
		CreatedAt:   time.Now().UTC(),
	}

	if req.Config != nil && req.Config.AIApiKey != "" {
		t.Config.AIApiKey = req.Config.AIApiKey
	}

	fields := s.taskToHash(t)

	// Encrypt sensitive fields
	if t.AccessToken != "" {
		enc, err := s.crypto.Encrypt(t.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("encrypting access token: %w", err)
		}
		fields["encrypted_access_token"] = enc
	}
	if t.Config != nil && t.Config.AIApiKey != "" {
		enc, err := s.crypto.Encrypt(t.Config.AIApiKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting ai api key: %w", err)
		}
		fields["encrypted_ai_api_key"] = enc
	}

	stateKey := s.redis.Key("task", t.ID, "state")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.HSet(ctx, stateKey, fields)
	pipe.RPush(ctx, s.redis.Key(s.queueName), t.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("creating task in redis: %w", err)
	}

	slog.Info("task created", "task_id", t.ID, "repo_url", t.RepoURL)
	return t, nil
}

// Get retrieves a task from Redis by ID. Sensitive fields are decrypted in memory.
func (s *Service) Get(ctx context.Context, taskID string) (*Task, error) {
	stateKey := s.redis.Key("task", taskID, "state")
	fields, err := s.redis.Unwrap().HGetAll(ctx, stateKey).Result()
	if err != nil {
		return nil, fmt.Errorf("getting task from redis: %w", err)
	}
	if len(fields) == 0 {
		return nil, apperror.NotFound("task %s not found", taskID)
	}

	t := s.hashToTask(fields)

	// Decrypt sensitive fields
	if enc := fields["encrypted_access_token"]; enc != "" {
		token, err := s.crypto.Decrypt(enc)
		if err != nil {
			slog.Error("failed to decrypt access token", "task_id", taskID, "error", err)
		} else {
			t.AccessToken = token
		}
	}
	if enc := fields["encrypted_ai_api_key"]; enc != "" {
		key, err := s.crypto.Decrypt(enc)
		if err != nil {
			slog.Error("failed to decrypt ai api key", "task_id", taskID, "error", err)
		} else {
			if t.Config == nil {
				t.Config = &TaskConfig{}
			}
			t.Config.AIApiKey = key
		}
	}

	// Load result if exists
	resultKey := s.redis.Key("task", taskID, "result")
	result, err := s.redis.Unwrap().Get(ctx, resultKey).Result()
	if err == nil {
		t.Result = result
	}

	return t, nil
}

// UpdateStatus transitions a task to a new status with state machine validation.
func (s *Service) UpdateStatus(ctx context.Context, taskID string, newStatus TaskStatus) error {
	stateKey := s.redis.Key("task", taskID, "state")

	currentStatus, err := s.redis.Unwrap().HGet(ctx, stateKey, "status").Result()
	if err == redis.Nil {
		return apperror.NotFound("task %s not found", taskID)
	}
	if err != nil {
		return fmt.Errorf("getting task status: %w", err)
	}

	if err := ValidateTransition(TaskStatus(currentStatus), newStatus); err != nil {
		return err
	}

	now := time.Now().UTC()
	fields := map[string]interface{}{
		"status":     string(newStatus),
		"updated_at": now.Format(time.RFC3339Nano),
	}

	// Set timestamps based on status
	switch newStatus {
	case StatusCloning, StatusRunning:
		fields["started_at"] = now.Format(time.RFC3339Nano)
	case StatusCompleted, StatusFailed, StatusPRCreated:
		fields["finished_at"] = now.Format(time.RFC3339Nano)
	}

	pipe := s.redis.Unwrap().Pipeline()
	pipe.HSet(ctx, stateKey, fields)

	// Set TTL on terminal states
	if IsFinished(newStatus) {
		pipe.Expire(ctx, stateKey, s.stateTTL)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}

	slog.Info("task status updated", "task_id", taskID, "status", newStatus)
	return nil
}

// SetResult stores the task result and changes summary.
func (s *Service) SetResult(ctx context.Context, taskID string, result string, changes *gitpkg.ChangesSummary, usage *UsageInfo) error {
	resultKey := s.redis.Key("task", taskID, "result")
	stateKey := s.redis.Key("task", taskID, "state")

	fields := map[string]interface{}{}
	if changes != nil {
		fields["changes_summary"] = MarshalChangesSummary(changes)
	}
	if usage != nil {
		fields["usage"] = MarshalUsageInfo(usage)
	}

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Set(ctx, resultKey, result, s.resultTTL)
	if len(fields) > 0 {
		pipe.HSet(ctx, stateKey, fields)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("setting task result: %w", err)
	}

	return nil
}

// SetError stores an error message on the task.
func (s *Service) SetError(ctx context.Context, taskID string, errMsg string) error {
	stateKey := s.redis.Key("task", taskID, "state")
	return s.redis.Unwrap().HSet(ctx, stateKey, "error", errMsg).Err()
}

// taskToHash converts a Task to a Redis hash map.
func (s *Service) taskToHash(t *Task) map[string]interface{} {
	fields := map[string]interface{}{
		"id":          t.ID,
		"status":      string(t.Status),
		"repo_url":    t.RepoURL,
		"prompt":      t.Prompt,
		"iteration":   t.Iteration,
		"created_at":  t.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":  t.CreatedAt.Format(time.RFC3339Nano),
	}

	if t.ProviderKey != "" {
		fields["provider_key"] = t.ProviderKey
	}
	if t.CallbackURL != "" {
		fields["callback_url"] = t.CallbackURL
	}
	if t.Config != nil {
		fields["config"] = MarshalConfig(t.Config)
	}
	if t.TraceID != "" {
		fields["trace_id"] = t.TraceID
	}

	return fields
}

// hashToTask converts a Redis hash map to a Task.
func (s *Service) hashToTask(fields map[string]string) *Task {
	t := &Task{
		ID:            fields["id"],
		Status:        TaskStatus(fields["status"]),
		RepoURL:       fields["repo_url"],
		ProviderKey:   fields["provider_key"],
		Prompt:        fields["prompt"],
		CallbackURL:   fields["callback_url"],
		CurrentPrompt: fields["current_prompt"],
		Branch:        fields["branch"],
		PRURL:         fields["pr_url"],
		Error:         fields["error"],
		TraceID:       fields["trace_id"],
	}

	if v := fields["iteration"]; v != "" {
		t.Iteration, _ = strconv.Atoi(v)
	}
	if v := fields["pr_number"]; v != "" {
		t.PRNumber, _ = strconv.Atoi(v)
	}

	if v := fields["created_at"]; v != "" {
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v := fields["started_at"]; v != "" {
		ts, _ := time.Parse(time.RFC3339Nano, v)
		t.StartedAt = &ts
	}
	if v := fields["finished_at"]; v != "" {
		ts, _ := time.Parse(time.RFC3339Nano, v)
		t.FinishedAt = &ts
	}

	t.Config = UnmarshalConfig(fields["config"])
	t.ChangesSummary = UnmarshalChangesSummary(fields["changes_summary"])
	t.Usage = UnmarshalUsageInfo(fields["usage"])

	return t
}

// CreateTaskRequest is the payload for task creation.
type CreateTaskRequest struct {
	RepoURL     string      `json:"repo_url" validate:"required,url"`
	ProviderKey string      `json:"provider_key,omitempty"`
	AccessToken string      `json:"access_token,omitempty"`
	Prompt      string      `json:"prompt" validate:"required,max=102400"`
	CallbackURL string      `json:"callback_url,omitempty" validate:"omitempty,url"`
	Config      *TaskConfig `json:"config,omitempty"`
}
