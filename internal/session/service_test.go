//go:build integration

package session

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/redisclient"
)

func getRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CODEFORGE_REDIS__URL")
	if url == "" {
		url = "redis://localhost:6379"
	}
	return url
}

func setupTestService(t *testing.T) (*Service, *redisclient.Client) {
	t.Helper()

	rdb, err := redisclient.New(getRedisURL(t), "test:service:")
	if err != nil {
		t.Skipf("skipping: redis not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx); err != nil {
		rdb.Close()
		t.Skipf("skipping: redis not reachable: %v", err)
	}

	key := base64.StdEncoding.EncodeToString([]byte("test-encryption-key-32-bytes!xxx"))
	cryptoSvc, err := crypto.NewService(key)
	if err != nil {
		t.Fatalf("crypto.NewService: %v", err)
	}

	svc := NewService(rdb, cryptoSvc, nil, "queue:test-tasks", 7*24*time.Hour, 7*24*time.Hour)

	t.Cleanup(func() {
		// Clean up test keys
		rdb.Unwrap().FlushDB(context.Background())
		rdb.Close()
	})

	return svc, rdb
}

func createTestSession(t *testing.T, svc *Service, status Status) *Session {
	t.Helper()
	ctx := context.Background()

	sess, err := svc.Create(ctx, CreateSessionRequest{
		RepoURL: "https://github.com/test/repo.git",
		Prompt:  "test prompt",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Transition to desired status
	switch status {
	case StatusPending:
		// already pending
	case StatusCompleted:
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
	case StatusAwaitingInstruction:
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusAwaitingInstruction); err != nil {
			t.Fatalf("UpdateStatus awaiting: %v", err)
		}
	case StatusRunning:
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
	case StatusFailed:
		if err := svc.UpdateStatus(ctx, sess.ID, StatusFailed); err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}
	case StatusReviewing:
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
		if err := svc.UpdateStatus(ctx, sess.ID, StatusReviewing); err != nil {
			t.Fatalf("UpdateStatus reviewing: %v", err)
		}
	}

	return sess
}

func TestCreate_PromptHandling(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		sessionType string
		prompt      string
		wantErr     bool
		wantPrompt  string
	}{
		{"code requires prompt", "code", "", true, ""},
		{"plan requires prompt", "plan", "", true, ""},
		{"review empty gets default", "review", "", false, "Review this repository for code quality, security, and architecture."},
		{"pr_review empty gets default", "pr_review", "", false, "Review this pull request."},
		{"knowledge empty allowed", "knowledge", "", false, ""},
		{"knowledge prompt passed through", "knowledge", "focus on auth", false, "focus on auth"},
		{"review prompt prefixed", "review", "check error handling", false, "Review this repository for code quality, security, and architecture.\ncheck error handling"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, err := svc.Create(ctx, CreateSessionRequest{
				RepoURL:     "https://github.com/test/repo.git",
				Prompt:      tt.prompt,
				SessionType: tt.sessionType,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if sess.Prompt != tt.wantPrompt {
				t.Errorf("prompt = %q, want %q", sess.Prompt, tt.wantPrompt)
			}
		})
	}
}

func TestStartReviewAsync_FromCompleted(t *testing.T) {
	svc, rdb := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusCompleted)

	got, err := svc.StartReviewAsync(ctx, sess.ID, "claude-code", "test-model")
	if err != nil {
		t.Fatalf("StartReviewAsync: %v", err)
	}
	if got.Status != StatusReviewing {
		t.Errorf("status = %s, want reviewing", got.Status)
	}

	// Verify session is in queue
	qLen, err := rdb.Unwrap().LLen(ctx, rdb.Key("queue:test-tasks")).Result()
	if err != nil {
		t.Fatalf("LLen: %v", err)
	}
	// Queue should have at least 1 entry (the review enqueue).
	// The original create also pushed to queue, but we consumed nothing.
	if qLen < 1 {
		t.Errorf("queue length = %d, want >= 1", qLen)
	}
}

func TestStartReviewAsync_FromAwaitingInstruction(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusAwaitingInstruction)

	got, err := svc.StartReviewAsync(ctx, sess.ID, "", "")
	if err != nil {
		t.Fatalf("StartReviewAsync: %v", err)
	}
	if got.Status != StatusReviewing {
		t.Errorf("status = %s, want reviewing", got.Status)
	}
}

func TestStartReviewAsync_FromRunning_Conflict(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusRunning)

	_, err := svc.StartReviewAsync(ctx, sess.ID, "", "")
	if err == nil {
		t.Fatal("expected error for running session")
	}
	if !isConflictError(err) {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestStartReviewAsync_FromFailed_Validation(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusFailed)

	_, err := svc.StartReviewAsync(ctx, sess.ID, "", "")
	if err == nil {
		t.Fatal("expected error for failed session")
	}
}

func TestStartReviewAsync_FromReviewing_Conflict(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusReviewing)

	_, err := svc.StartReviewAsync(ctx, sess.ID, "", "")
	if err == nil {
		t.Fatal("expected error for already reviewing session")
	}
	if !isConflictError(err) {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestStartReviewAsync_NotFound(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	_, err := svc.StartReviewAsync(ctx, "nonexistent-task-id", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestStartReviewAsync_StoresReviewParams(t *testing.T) {
	svc, rdb := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusCompleted)

	_, err := svc.StartReviewAsync(ctx, sess.ID, "codex", "o3")
	if err != nil {
		t.Fatalf("StartReviewAsync: %v", err)
	}

	// Verify review params stored in Redis hash
	stateKey := rdb.Key("session", sess.ID, "state")
	cli, err := rdb.Unwrap().HGet(ctx, stateKey, "review_cli").Result()
	if err != nil {
		t.Fatalf("HGet review_cli: %v", err)
	}
	if cli != "codex" {
		t.Errorf("review_cli = %q, want codex", cli)
	}

	model, err := rdb.Unwrap().HGet(ctx, stateKey, "review_model").Result()
	if err != nil {
		t.Fatalf("HGet review_model: %v", err)
	}
	if model != "o3" {
		t.Errorf("review_model = %q, want o3", model)
	}
}

func TestSetResult_AccumulatesUsage(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	sess := createTestSession(t, svc, StatusCompleted)

	// First iteration usage
	if err := svc.SetResult(ctx, sess.ID, "first result", nil, &UsageInfo{
		InputTokens:         100,
		OutputTokens:        50,
		CacheReadTokens:     1000,
		CacheCreationTokens: 200,
		CostUSD:             0.25,
		DurationSeconds:     30,
	}); err != nil {
		t.Fatalf("SetResult first: %v", err)
	}

	// Second iteration usage must accumulate on top of the first
	if err := svc.SetResult(ctx, sess.ID, "second result", nil, &UsageInfo{
		InputTokens:         40,
		OutputTokens:        20,
		CacheReadTokens:     500,
		CacheCreationTokens: 100,
		CostUSD:             0.125,
		DurationSeconds:     15,
	}); err != nil {
		t.Fatalf("SetResult second: %v", err)
	}

	got, err := svc.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Usage == nil {
		t.Fatal("usage is nil, want accumulated usage")
	}

	want := UsageInfo{
		InputTokens:         140,
		OutputTokens:        70,
		CacheReadTokens:     1500,
		CacheCreationTokens: 300,
		CostUSD:             0.375,
		DurationSeconds:     45,
	}
	if *got.Usage != want {
		t.Errorf("accumulated usage = %+v, want %+v", *got.Usage, want)
	}
}

// isConflictError checks if an error is a 409 conflict.
func isConflictError(err error) bool {
	return err != nil && (contains(err.Error(), "cannot start review") || contains(err.Error(), "cannot be reviewed"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
