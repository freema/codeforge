package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/review"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
)

// Service manages session lifecycle and Redis persistence.
type Service struct {
	redis     *redisclient.Client
	crypto    *crypto.Service
	sqlite    *SQLiteStore
	queueName string
	stateTTL  time.Duration
	resultTTL time.Duration
}

// NewService creates a new session service.
func NewService(redis *redisclient.Client, cryptoSvc *crypto.Service, db *sql.DB, queueName string, stateTTL, resultTTL time.Duration) *Service {
	svc := &Service{
		redis:     redis,
		crypto:    cryptoSvc,
		queueName: queueName,
		stateTTL:  stateTTL,
		resultTTL: resultTTL,
	}
	if db != nil {
		svc.sqlite = NewSQLiteStore(db)
	}
	return svc
}

// persistToSQLite runs fn as a fire-and-forget SQLite write.
// Errors are logged but never block the caller.
func (s *Service) persistToSQLite(fn func() error) {
	if s.sqlite == nil {
		return
	}
	if err := fn(); err != nil {
		slog.Warn("sqlite persistence failed", "error", err)
	}
}

// Create creates a new session in Redis and enqueues it for processing.
func (s *Service) Create(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	taskType := req.SessionType
	if taskType == "" {
		taskType = "code"
	}

	t := &Session{
		ID:            uuid.New().String(),
		Status:        StatusPending,
		RepoURL:       req.RepoURL,
		ProviderKey:   req.ProviderKey,
		AccessToken:   req.AccessToken,
		Prompt:        req.Prompt,
		SessionType:      taskType,
		CallbackURL:   req.CallbackURL,
		Config:        req.Config,
		WorkflowRunID: req.WorkflowRunID,
		Metadata:      req.Metadata,
		Iteration:     1,
		CreatedAt:     time.Now().UTC(),
	}

	if req.Config != nil && req.Config.AIApiKey != "" {
		t.Config.AIApiKey = req.Config.AIApiKey
	}

	fields := s.sessionToHash(t)

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

	stateKey := s.redis.Key("session", t.ID, "state")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.HSet(ctx, stateKey, fields)
	pipe.RPush(ctx, s.redis.Key(s.queueName), t.ID)
	pipe.SAdd(ctx, s.redis.Key("sessions:index"), t.ID) // track session ID for listing
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("creating session in redis: %w", err)
	}

	slog.Info("session created", "session_id", t.ID, "repo_url", t.RepoURL)

	s.persistToSQLite(func() error {
		return s.sqlite.Save(ctx, t)
	})

	return t, nil
}

// Get retrieves a session from Redis by ID. Sensitive fields are decrypted in memory.
func (s *Service) Get(ctx context.Context, taskID string) (*Session, error) {
	stateKey := s.redis.Key("session", taskID, "state")
	fields, err := s.redis.Unwrap().HGetAll(ctx, stateKey).Result()
	if err != nil {
		return nil, fmt.Errorf("getting session from redis: %w", err)
	}
	if len(fields) == 0 {
		// Fallback to SQLite for expired Redis keys
		if s.sqlite != nil {
			return s.sqlite.Get(ctx, taskID)
		}
		return nil, apperror.NotFound("session %s not found", taskID)
	}

	t := s.hashToSession(fields)

	// Decrypt sensitive fields
	if enc := fields["encrypted_access_token"]; enc != "" {
		token, err := s.crypto.Decrypt(enc)
		if err != nil {
			slog.Error("failed to decrypt access token", "session_id", taskID, "error", err)
		} else {
			t.AccessToken = token
		}
	}
	if enc := fields["encrypted_ai_api_key"]; enc != "" {
		key, err := s.crypto.Decrypt(enc)
		if err != nil {
			slog.Error("failed to decrypt ai api key", "session_id", taskID, "error", err)
		} else {
			if t.Config == nil {
				t.Config = &Config{}
			}
			t.Config.AIApiKey = key
		}
	}

	// Load result if exists
	resultKey := s.redis.Key("session", taskID, "result")
	result, err := s.redis.Unwrap().Get(ctx, resultKey).Result()
	if err == nil {
		t.Result = result
	}

	return t, nil
}

// UpdateStatus transitions a session to a new status with state machine validation.
func (s *Service) UpdateStatus(ctx context.Context, taskID string, newStatus Status) error {
	stateKey := s.redis.Key("session", taskID, "state")

	currentStatus, err := s.redis.Unwrap().HGet(ctx, stateKey, "status").Result()
	if err == redis.Nil {
		return apperror.NotFound("session %s not found", taskID)
	}
	if err != nil {
		return fmt.Errorf("getting session status: %w", err)
	}

	if err := ValidateTransition(Status(currentStatus), newStatus); err != nil {
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

	// Set TTL only on truly terminal states (failed).
	// Idle states (completed, pr_created) get a longer idle TTL
	// that resets on each interaction.
	if IsFinished(newStatus) {
		pipe.Expire(ctx, stateKey, s.stateTTL)
	} else if IsIdle(newStatus) {
		// Idle sessions get 7x the normal TTL (e.g. 7 days if stateTTL=24h)
		idleTTL := s.stateTTL * 7
		if idleTTL < 24*time.Hour {
			idleTTL = 7 * 24 * time.Hour // minimum 7 days
		}
		pipe.Expire(ctx, stateKey, idleTTL)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("updating session status: %w", err)
	}

	slog.Info("session status updated", "session_id", taskID, "status", newStatus)

	// Determine timestamps for SQLite
	var startedAt, finishedAt *time.Time
	switch newStatus {
	case StatusCloning, StatusRunning:
		startedAt = &now
	case StatusCompleted, StatusFailed, StatusPRCreated:
		finishedAt = &now
	}
	s.persistToSQLite(func() error {
		return s.sqlite.UpdateStatus(ctx, taskID, newStatus, startedAt, finishedAt)
	})

	return nil
}

// SetResult stores the session result and changes summary.
func (s *Service) SetResult(ctx context.Context, taskID string, result string, changes *gitpkg.ChangesSummary, usage *UsageInfo) error {
	resultKey := s.redis.Key("session", taskID, "result")
	stateKey := s.redis.Key("session", taskID, "state")

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
		return fmt.Errorf("setting session result: %w", err)
	}

	s.persistToSQLite(func() error {
		return s.sqlite.UpdateResult(ctx, taskID, result, changes, usage)
	})

	return nil
}

// Instruct submits a follow-up instruction for an existing task.
func (s *Service) Instruct(ctx context.Context, taskID string, prompt string) (*Session, error) {
	t, err := s.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Validate state allows instruction
	switch t.Status {
	case StatusCompleted, StatusAwaitingInstruction, StatusPRCreated:
		// ok
	case StatusRunning, StatusCloning, StatusCreatingPR:
		return nil, apperror.Conflict("session is currently %s, cannot instruct", t.Status)
	case StatusFailed:
		return nil, apperror.Validation("session has failed, create a new session instead")
	default:
		return nil, apperror.Conflict("session in status %s cannot accept instructions", t.Status)
	}

	// Transition through AWAITING_INSTRUCTION if needed
	if t.Status == StatusCompleted || t.Status == StatusPRCreated {
		if err := ValidateTransition(t.Status, StatusAwaitingInstruction); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	newIteration := t.Iteration + 1

	stateKey := s.redis.Key("session", taskID, "state")
	pipe := s.redis.Unwrap().Pipeline()

	// Update session state
	pipe.HSet(ctx, stateKey, map[string]interface{}{
		"status":         string(StatusAwaitingInstruction),
		"current_prompt": prompt,
		"iteration":      newIteration,
		"updated_at":     now.Format(time.RFC3339Nano),
		"error":          "", // clear previous error
	})

	// Remove TTL (session is active again)
	pipe.Persist(ctx, stateKey)

	// Re-enqueue for worker processing
	pipe.RPush(ctx, s.redis.Key(s.queueName), taskID)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("instructing session: %w", err)
	}

	t.Status = StatusAwaitingInstruction
	t.CurrentPrompt = prompt
	t.Iteration = newIteration
	t.Error = ""

	slog.Info("session instructed", "session_id", taskID, "iteration", newIteration)

	s.persistToSQLite(func() error {
		return s.sqlite.Save(ctx, t)
	})

	return t, nil
}

// SaveIteration appends a completed iteration record to the session's iteration history.
func (s *Service) SaveIteration(ctx context.Context, taskID string, iter Iteration) error {
	iterKey := s.redis.Key("session", taskID, "iterations")
	data, err := json.Marshal(iter)
	if err != nil {
		return fmt.Errorf("marshaling iteration: %w", err)
	}

	if err := s.redis.Unwrap().RPush(ctx, iterKey, string(data)).Err(); err != nil {
		return err
	}

	s.persistToSQLite(func() error {
		return s.sqlite.SaveIteration(ctx, taskID, iter)
	})

	return nil
}

// GetIterations loads the full iteration history from Redis, falling back to SQLite.
func (s *Service) GetIterations(ctx context.Context, taskID string) ([]Iteration, error) {
	iterKey := s.redis.Key("session", taskID, "iterations")
	items, err := s.redis.Unwrap().LRange(ctx, iterKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("loading iterations: %w", err)
	}

	if len(items) == 0 && s.sqlite != nil {
		return s.sqlite.GetIterations(ctx, taskID)
	}

	iterations := make([]Iteration, 0, len(items))
	for _, item := range items {
		var iter Iteration
		if err := json.Unmarshal([]byte(item), &iter); err != nil {
			continue
		}
		iterations = append(iterations, iter)
	}
	return iterations, nil
}

// Summary is a lightweight view of a session for listing.
type Summary struct {
	ID             string                 `json:"id"`
	Status         Status             `json:"status"`
	RepoURL        string                 `json:"repo_url"`
	Prompt         string                 `json:"prompt"`
	SessionType       string                 `json:"session_type,omitempty"`
	Iteration      int                    `json:"iteration"`
	Error          string                 `json:"error,omitempty"`
	Branch         string                 `json:"branch,omitempty"`
	PRURL          string                 `json:"pr_url,omitempty"`
	WorkflowRunID  string                 `json:"workflow_run_id,omitempty"`
	ChangesSummary *gitpkg.ChangesSummary `json:"changes_summary,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	FinishedAt     *time.Time             `json:"finished_at,omitempty"`
}

// ListOptions configures session listing.
type ListOptions struct {
	Status string // filter by status (empty = all)
	Limit  int    // max results (0 = 50)
	Offset int    // pagination offset
}

// List returns session summaries from SQLite (persistent storage).
func (s *Service) List(ctx context.Context, opts ListOptions) ([]Summary, int, error) {
	if s.sqlite != nil {
		return s.sqlite.List(ctx, opts)
	}

	// Fallback: Redis-only listing (for backwards compatibility when SQLite is nil)
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	indexKey := s.redis.Key("sessions:index")
	ids, err := s.redis.Unwrap().SMembers(ctx, indexKey).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("listing session index: %w", err)
	}

	if len(ids) == 0 {
		pattern := s.redis.Key("session", "*", "state")
		var cursor uint64
		for {
			var keys []string
			keys, cursor, err = s.redis.Unwrap().Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return nil, 0, fmt.Errorf("scanning sessions: %w", err)
			}
			for _, k := range keys {
				parts := extractTaskIDFromKey(k, s.redis.Prefix())
				if parts != "" {
					ids = append(ids, parts)
					s.redis.Unwrap().SAdd(ctx, indexKey, parts)
				}
			}
			if cursor == 0 {
				break
			}
		}
	}

	var tasks []Summary
	for _, id := range ids {
		stateKey := s.redis.Key("session", id, "state")
		fields, err := s.redis.Unwrap().HGetAll(ctx, stateKey).Result()
		if err != nil || len(fields) == 0 {
			s.redis.Unwrap().SRem(ctx, indexKey, id)
			continue
		}

		t := s.hashToSession(fields)
		if opts.Status != "" && string(t.Status) != opts.Status {
			continue
		}

		tasks = append(tasks, Summary{
			ID:             t.ID,
			Status:         t.Status,
			RepoURL:        t.RepoURL,
			Prompt:         truncatePrompt(t.Prompt, 200),
			SessionType:       t.SessionType,
			Iteration:      t.Iteration,
			Error:          t.Error,
			Branch:         t.Branch,
			PRURL:          t.PRURL,
			WorkflowRunID:  t.WorkflowRunID,
			ChangesSummary: t.ChangesSummary,
			CreatedAt:      t.CreatedAt,
			StartedAt:      t.StartedAt,
			FinishedAt:     t.FinishedAt,
		})
	}

	sortByCreatedDesc(tasks)
	total := len(tasks)

	if opts.Offset >= len(tasks) {
		return []Summary{}, total, nil
	}
	tasks = tasks[opts.Offset:]
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}

	return tasks, total, nil
}

func truncatePrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractTaskIDFromKey(key, prefix string) string {
	// key format: "{prefix}session:{id}:state"
	trimmed := key
	if prefix != "" {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			trimmed = key[len(prefix):]
		}
	}
	// trimmed should be "session:{id}:state"
	const taskPrefix = "session:"
	const stateSuffix = ":state"
	if len(trimmed) > len(taskPrefix)+len(stateSuffix) &&
		trimmed[:len(taskPrefix)] == taskPrefix &&
		trimmed[len(trimmed)-len(stateSuffix):] == stateSuffix {
		return trimmed[len(taskPrefix) : len(trimmed)-len(stateSuffix)]
	}
	return ""
}

func sortByCreatedDesc(tasks []Summary) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].CreatedAt.After(tasks[j-1].CreatedAt); j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}

// StartReviewAsync enqueues a review for worker execution (non-blocking).
// Uses Redis WATCH for atomic check-and-set to prevent double-enqueue races.
func (s *Service) StartReviewAsync(ctx context.Context, taskID, cli, model string) (*Session, error) {
	stateKey := s.redis.Key("session", taskID, "state")
	queueKey := s.redis.Key(s.queueName)
	now := time.Now().UTC()

	err := s.redis.Unwrap().Watch(ctx, func(tx *redis.Tx) error {
		current, err := tx.HGet(ctx, stateKey, "status").Result()
		if err == redis.Nil {
			return apperror.NotFound("session %s not found", taskID)
		}
		if err != nil {
			return fmt.Errorf("reading session status: %w", err)
		}

		switch Status(current) {
		case StatusCompleted, StatusAwaitingInstruction, StatusPRCreated:
			// ok — session is idle, review can happen at any idle point
		case StatusRunning, StatusCloning, StatusCreatingPR, StatusReviewing:
			return apperror.Conflict("session is currently %s, cannot start review", Status(current))
		case StatusFailed:
			return apperror.Validation("session has failed, create a new session instead")
		default:
			return apperror.Conflict("session in status %s cannot be reviewed", Status(current))
		}

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, stateKey, map[string]interface{}{
				"status":       string(StatusReviewing),
				"updated_at":   now.Format(time.RFC3339Nano),
				"review_cli":   cli,
				"review_model": model,
				"error":        "",
			})
			pipe.Persist(ctx, stateKey)
			pipe.RPush(ctx, queueKey, taskID)
			return nil
		})
		return err
	}, stateKey)

	if err != nil {
		// WATCH detects concurrent modification → business 409, not 500
		if errors.Is(err, redis.TxFailedErr) {
			return nil, apperror.Conflict("session state changed concurrently, retry the request")
		}
		return nil, err
	}

	// Load the full task for the response
	t, err := s.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	slog.Info("review enqueued", "session_id", taskID)

	s.persistToSQLite(func() error {
		return s.sqlite.UpdateStatus(ctx, taskID, StatusReviewing, nil, nil)
	})

	return t, nil
}

// CompleteReview stores the review result and transitions the session back to completed.
func (s *Service) CompleteReview(ctx context.Context, taskID string, result *review.ReviewResult) error {
	if err := s.SetReviewResult(ctx, taskID, result); err != nil {
		return err
	}
	return s.UpdateStatus(ctx, taskID, StatusCompleted)
}

// SetReviewResult stores the review result on a session.
func (s *Service) SetReviewResult(ctx context.Context, taskID string, result *review.ReviewResult) error {
	stateKey := s.redis.Key("session", taskID, "state")
	if err := s.redis.Unwrap().HSet(ctx, stateKey, "review_result", review.MarshalReviewResult(result)).Err(); err != nil {
		return fmt.Errorf("setting review result: %w", err)
	}

	s.persistToSQLite(func() error {
		return s.sqlite.UpdateReviewResult(ctx, taskID, result)
	})

	return nil
}

// UpdateConfig persists updated Config to Redis.
func (s *Service) UpdateConfig(ctx context.Context, taskID string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	stateKey := s.redis.Key("session", taskID, "state")
	if err := s.redis.Unwrap().HSet(ctx, stateKey, "config", MarshalConfig(cfg)).Err(); err != nil {
		return fmt.Errorf("updating session config: %w", err)
	}
	return nil
}

// SetError stores an error message on the session.
func (s *Service) SetError(ctx context.Context, taskID string, errMsg string) error {
	stateKey := s.redis.Key("session", taskID, "state")
	if err := s.redis.Unwrap().HSet(ctx, stateKey, "error", errMsg).Err(); err != nil {
		return err
	}

	s.persistToSQLite(func() error {
		return s.sqlite.UpdateError(ctx, taskID, errMsg)
	})

	return nil
}

// sessionToHash converts a Session to a Redis hash map.
func (s *Service) sessionToHash(t *Session) map[string]interface{} {
	fields := map[string]interface{}{
		"id":           t.ID,
		"status":       string(t.Status),
		"repo_url":     t.RepoURL,
		"prompt":       t.Prompt,
		"session_type": t.SessionType,
		"iteration":  t.Iteration,
		"created_at": t.CreatedAt.Format(time.RFC3339Nano),
		"updated_at": t.CreatedAt.Format(time.RFC3339Nano),
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
	if t.WorkflowRunID != "" {
		fields["workflow_run_id"] = t.WorkflowRunID
	}
	if t.TraceID != "" {
		fields["trace_id"] = t.TraceID
	}
	if t.ReviewCLI != "" {
		fields["review_cli"] = t.ReviewCLI
	}
	if t.ReviewModel != "" {
		fields["review_model"] = t.ReviewModel
	}
	if len(t.Metadata) > 0 {
		b, _ := json.Marshal(t.Metadata)
		fields["metadata"] = string(b)
	}

	return fields
}

// hashToSession converts a Redis hash map to a Session.
func (s *Service) hashToSession(fields map[string]string) *Session {
	t := &Session{
		ID:            fields["id"],
		Status:        Status(fields["status"]),
		RepoURL:       fields["repo_url"],
		ProviderKey:   fields["provider_key"],
		Prompt:        fields["prompt"],
		SessionType:      fields["session_type"],
		CallbackURL:   fields["callback_url"],
		CurrentPrompt: fields["current_prompt"],
		Branch:        fields["branch"],
		PRURL:         fields["pr_url"],
		Error:         fields["error"],
		WorkflowRunID: fields["workflow_run_id"],
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
	t.ReviewResult = review.UnmarshalReviewResult(fields["review_result"])
	t.ReviewCLI = fields["review_cli"]
	t.ReviewModel = fields["review_model"]

	if v := fields["metadata"]; v != "" {
		_ = json.Unmarshal([]byte(v), &t.Metadata)
	}

	return t
}

// CreateSessionRequest is the payload for session creation.
type CreateSessionRequest struct {
	RepoURL       string            `json:"repo_url" validate:"required,url"`
	ProviderKey   string            `json:"provider_key,omitempty"`
	AccessToken   string            `json:"access_token,omitempty"`
	Prompt        string            `json:"prompt" validate:"required,max=102400"`
	SessionType      string            `json:"session_type,omitempty"`
	CallbackURL   string            `json:"callback_url,omitempty" validate:"omitempty,url"`
	Config        *Config       `json:"config,omitempty"`
	WorkflowRunID string            `json:"workflow_run_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// FindByPR finds the most recent active session for a given repo + PR/MR number.
func (s *Service) FindByPR(ctx context.Context, repoURL string, prNumber int) (*Session, error) {
	// Try SQLite first (indexed, fast)
	if s.sqlite != nil {
		t, err := s.sqlite.FindByPR(ctx, repoURL, prNumber)
		if err != nil {
			return nil, err
		}
		if t != nil {
			// Try to get fresh data from Redis
			fresh, err := s.Get(ctx, t.ID)
			if err == nil {
				return fresh, nil
			}
			// Redis expired, return SQLite data
			return t, nil
		}
	}

	return nil, apperror.NotFound("no session found for repo %s PR #%d", repoURL, prNumber)
}
