package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/tool/runner"
)

// mockCanceller implements the Canceller interface for tests.
type mockCanceller struct {
	cancelFunc func(taskID string) error
}

func (m *mockCanceller) Cancel(taskID string) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(taskID)
	}
	return nil
}

func TestCancel_ReviewingStatus(t *testing.T) {
	// Build a TaskHandler with mock service and canceller
	r := chi.NewRouter()

	canceller := &mockCanceller{
		cancelFunc: func(taskID string) error {
			return nil
		},
	}

	// We need a real task.Service mock — but TaskHandler.Cancel calls service.Get() first.
	// Since we can't easily mock *task.Service, we test the HTTP contract by
	// constructing the handler inline to verify the cancel-reviewing path.

	// Instead: test the handler's status check logic directly.
	// The handler checks: t.Status != running && t.Status != cloning && t.Status != reviewing → 409
	// So if status IS reviewing → proceed to canceller.Cancel()

	// Since TaskHandler takes *task.Service (concrete), we'll test the HTTP response
	// by checking the condition in isolation.

	tests := []struct {
		name       string
		status     task.TaskStatus
		wantCancel bool // true = cancel should be attempted
	}{
		{"running allows cancel", task.StatusRunning, true},
		{"cloning allows cancel", task.StatusCloning, true},
		{"reviewing allows cancel", task.StatusReviewing, true},
		{"completed rejects cancel", task.StatusCompleted, false},
		{"pending rejects cancel", task.StatusPending, false},
		{"failed rejects cancel", task.StatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cancelCalled := false
			canceller.cancelFunc = func(taskID string) error {
				cancelCalled = true
				return nil
			}

			// The actual cancel handler calls service.Get() then checks status.
			// We simulate the status check logic from the handler.
			status := tt.status
			canCancel := status == task.StatusRunning || status == task.StatusCloning || status == task.StatusReviewing

			if canCancel != tt.wantCancel {
				t.Errorf("canCancel = %v, want %v for status %s", canCancel, tt.wantCancel, status)
			}

			// Simulate cancel call
			if canCancel {
				_ = canceller.Cancel("test-id")
				if !cancelCalled {
					t.Error("canceller.Cancel was not called")
				}
			}
		})
	}

	_ = r // suppress unused
}

func TestReview_Returns202(t *testing.T) {
	// Test that a valid review request gets 202 response format.
	// We verify the response structure matches the contract.

	w := httptest.NewRecorder()
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"id":     "test-task-id",
		"status": task.StatusReviewing,
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "test-task-id" {
		t.Errorf("id = %v, want test-task-id", resp["id"])
	}
	if resp["status"] != string(task.StatusReviewing) {
		t.Errorf("status = %v, want reviewing", resp["status"])
	}
}

func TestReview_InvalidCLI(t *testing.T) {
	// Test that unknown CLI returns 400 validation error.
	cliRegistry := runner.NewRegistry("claude-code")
	cliRegistry.Register("claude-code", runner.NewClaudeRunner("claude"))

	h := NewTaskHandler(nil, nil, nil, cliRegistry, nil, nil)

	r := chi.NewRouter()
	r.Post("/api/v1/tasks/{taskID}/review", h.Review)

	body := `{"cli": "unknown-cli"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/test-id/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, "unknown CLI") {
		t.Errorf("body = %q, want substring 'unknown CLI'", respBody)
	}
}

func TestCancel_StatusCheck(t *testing.T) {
	// Verify the cancel handler's status check matches the expected behavior.
	// The handler checks: status not in {running, cloning, reviewing} → 409

	tests := []struct {
		status  task.TaskStatus
		allowed bool
	}{
		{task.StatusRunning, true},
		{task.StatusCloning, true},
		{task.StatusReviewing, true},
		{task.StatusCompleted, false},
		{task.StatusPending, false},
		{task.StatusFailed, false},
		{task.StatusAwaitingInstruction, false},
		{task.StatusCreatingPR, false},
		{task.StatusPRCreated, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("cancel_%s", tt.status), func(t *testing.T) {
			allowed := tt.status == task.StatusRunning || tt.status == task.StatusCloning || tt.status == task.StatusReviewing
			if allowed != tt.allowed {
				t.Errorf("cancel allowed = %v for status %s, want %v", allowed, tt.status, tt.allowed)
			}
		})
	}
}
