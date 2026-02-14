package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/task"
)

// StreamHandler handles SSE streaming for task events.
type StreamHandler struct {
	service *task.Service
	redis   *redisclient.Client
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler(service *task.Service, redis *redisclient.Client) *StreamHandler {
	return &StreamHandler{service: service, redis: redis}
}

// Stream handles GET /api/v1/tasks/{taskID}/stream.
// Streams task events via Server-Sent Events (SSE).
// First replays historical events, then subscribes to live events via Redis Pub/Sub.
func (h *StreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	// Verify task exists and get current status
	t, err := h.service.Get(r.Context(), taskID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Use ResponseController for write deadline management
	rc := http.NewResponseController(w)

	flush := func() { flusher.Flush() }

	isTerminal := t.Status == task.StatusCompleted ||
		t.Status == task.StatusFailed ||
		t.Status == task.StatusPRCreated

	// Subscribe to live channels BEFORE reading history to avoid missing events.
	// For terminal tasks we skip subscription entirely.
	streamKey := h.redis.Key("task", taskID, "stream")
	doneKey := h.redis.Key("task", taskID, "done")

	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()

	var msgCh <-chan *redis.Message
	if !isTerminal {
		pubsub := h.redis.Unwrap().Subscribe(subCtx, streamKey, doneKey)
		defer pubsub.Close()
		msgCh = pubsub.Channel()
	}

	// Send connected event with current task state
	writeSSE(w, "connected", map[string]interface{}{
		"task_id": t.ID,
		"status":  t.Status,
	})
	flush()

	// Replay history
	historyKey := h.redis.Key("task", taskID, "history")
	history, err := h.redis.Unwrap().LRange(r.Context(), historyKey, 0, -1).Result()
	if err == nil && len(history) > 0 {
		for _, msg := range history {
			fmt.Fprintf(w, "data: %s\n\n", msg)
		}
		flush()
	}

	// For terminal tasks, send done and close immediately
	if isTerminal {
		writeSSE(w, "done", map[string]interface{}{
			"task_id": t.ID,
			"status":  t.Status,
		})
		flush()
		return
	}

	// Stream live events
	maxDuration := 10 * time.Minute
	deadline := time.After(maxDuration)
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	slog.Debug("SSE stream started", "task_id", taskID)

	for {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))

		select {
		case <-r.Context().Done():
			slog.Debug("SSE client disconnected", "task_id", taskID)
			return

		case <-deadline:
			writeSSE(w, "timeout", map[string]string{
				"message": "stream closed after 10 minutes",
			})
			flush()
			slog.Debug("SSE stream timed out", "task_id", taskID)
			return

		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flush()

		case msg, ok := <-msgCh:
			if !ok {
				return
			}

			if msg.Channel == doneKey {
				// Forward the done payload as a named event
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", msg.Payload)
				flush()
				return
			}

			// Regular stream event
			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flush()
		}
	}
}

// writeSSE writes a named SSE event with JSON data.
func writeSSE(w http.ResponseWriter, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
}
