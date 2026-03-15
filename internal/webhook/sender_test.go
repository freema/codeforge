package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSender_Send_Success(t *testing.T) {
	var received atomic.Bool
	var gotSig, gotEvent, gotTraceID string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		gotSig = r.Header.Get("X-Signature-256")
		gotEvent = r.Header.Get("X-CodeForge-Event")
		gotTraceID = r.Header.Get("X-Trace-ID")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := NewSender("my-secret", 0, time.Millisecond)
	err := sender.Send(context.Background(), srv.URL, Payload{
		TaskID:  "task-1",
		Status:  "completed",
		TraceID: "trace-123",
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !received.Load() {
		t.Fatal("webhook was not received")
	}
	if gotEvent != "task.completed" {
		t.Errorf("event: got %q, want %q", gotEvent, "task.completed")
	}
	if gotTraceID != "trace-123" {
		t.Errorf("trace ID: got %q, want %q", gotTraceID, "trace-123")
	}

	// Verify HMAC signature
	mac := hmac.New(sha256.New, []byte("my-secret"))
	mac.Write(gotBody)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != expectedSig {
		t.Errorf("signature mismatch: got %q, want %q", gotSig, expectedSig)
	}
}

func TestSender_Send_NoTraceID(t *testing.T) {
	var gotTraceID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get("X-Trace-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := NewSender("secret", 0, time.Millisecond)
	_ = sender.Send(context.Background(), srv.URL, Payload{
		TaskID: "task-1",
		Status: "failed",
	})

	if gotTraceID != "" {
		t.Errorf("expected empty trace ID header, got %q", gotTraceID)
	}
}

func TestSender_Send_Retry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := NewSender("secret", 3, time.Millisecond) // fast retries for test
	err := sender.Send(context.Background(), srv.URL, Payload{
		TaskID: "task-1",
		Status: "completed",
	})

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts: got %d, want 3", attempts.Load())
	}
}

func TestSender_Send_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sender := NewSender("secret", 1, time.Millisecond)
	err := sender.Send(context.Background(), srv.URL, Payload{
		TaskID: "task-1",
		Status: "failed",
	})

	if err == nil {
		t.Fatal("expected error after all retries fail")
	}
}

func TestSender_Send_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	sender := NewSender("secret", 3, time.Second)
	err := sender.Send(ctx, srv.URL, Payload{
		TaskID: "task-1",
		Status: "completed",
	})

	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestSender_PayloadJSON(t *testing.T) {
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := NewSender("secret", 0, time.Millisecond)
	now := time.Now().UTC().Truncate(time.Second)
	_ = sender.Send(context.Background(), srv.URL, Payload{
		TaskID:     "task-42",
		Status:     "completed",
		Result:     "all good",
		FinishedAt: now,
	})

	var payload Payload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TaskID != "task-42" {
		t.Errorf("task_id: got %q, want %q", payload.TaskID, "task-42")
	}
	if payload.Result != "all good" {
		t.Errorf("result: got %q, want %q", payload.Result, "all good")
	}
}
