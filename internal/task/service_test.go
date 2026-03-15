//go:build integration

package task

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

func createTestTask(t *testing.T, svc *Service, status TaskStatus) *Task {
	t.Helper()
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateTaskRequest{
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
		if err := svc.UpdateStatus(ctx, task.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
	case StatusAwaitingInstruction:
		if err := svc.UpdateStatus(ctx, task.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusAwaitingInstruction); err != nil {
			t.Fatalf("UpdateStatus awaiting: %v", err)
		}
	case StatusRunning:
		if err := svc.UpdateStatus(ctx, task.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
	case StatusFailed:
		if err := svc.UpdateStatus(ctx, task.ID, StatusFailed); err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}
	case StatusReviewing:
		if err := svc.UpdateStatus(ctx, task.ID, StatusCloning); err != nil {
			t.Fatalf("UpdateStatus cloning: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusRunning); err != nil {
			t.Fatalf("UpdateStatus running: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus completed: %v", err)
		}
		if err := svc.UpdateStatus(ctx, task.ID, StatusReviewing); err != nil {
			t.Fatalf("UpdateStatus reviewing: %v", err)
		}
	}

	return task
}

func TestStartReviewAsync_FromCompleted(t *testing.T) {
	svc, rdb := setupTestService(t)
	ctx := context.Background()

	task := createTestTask(t, svc, StatusCompleted)

	got, err := svc.StartReviewAsync(ctx, task.ID, "claude-code", "test-model")
	if err != nil {
		t.Fatalf("StartReviewAsync: %v", err)
	}
	if got.Status != StatusReviewing {
		t.Errorf("status = %s, want reviewing", got.Status)
	}

	// Verify task is in queue
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

	task := createTestTask(t, svc, StatusAwaitingInstruction)

	got, err := svc.StartReviewAsync(ctx, task.ID, "", "")
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

	task := createTestTask(t, svc, StatusRunning)

	_, err := svc.StartReviewAsync(ctx, task.ID, "", "")
	if err == nil {
		t.Fatal("expected error for running task")
	}
	if !isConflictError(err) {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestStartReviewAsync_FromFailed_Validation(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	task := createTestTask(t, svc, StatusFailed)

	_, err := svc.StartReviewAsync(ctx, task.ID, "", "")
	if err == nil {
		t.Fatal("expected error for failed task")
	}
}

func TestStartReviewAsync_FromReviewing_Conflict(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	task := createTestTask(t, svc, StatusReviewing)

	_, err := svc.StartReviewAsync(ctx, task.ID, "", "")
	if err == nil {
		t.Fatal("expected error for already reviewing task")
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
		t.Fatal("expected error for nonexistent task")
	}
}

func TestStartReviewAsync_StoresReviewParams(t *testing.T) {
	svc, rdb := setupTestService(t)
	ctx := context.Background()

	task := createTestTask(t, svc, StatusCompleted)

	_, err := svc.StartReviewAsync(ctx, task.ID, "codex", "o3")
	if err != nil {
		t.Fatalf("StartReviewAsync: %v", err)
	}

	// Verify review params stored in Redis hash
	stateKey := rdb.Key("task", task.ID, "state")
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
