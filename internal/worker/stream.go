package worker

import (
	"context"
	"encoding/json"
	"time"

	gitpkg "github.com/freema/codeforge/internal/git"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/task"
)

// StreamEvent is a structured event published to Redis Pub/Sub.
type StreamEvent struct {
	Type  string          `json:"type"`  // system, git, cli, stream, result
	Event string          `json:"event"` // event name
	Data  json.RawMessage `json:"data"`  // event-specific payload
	TS    string          `json:"ts"`    // ISO 8601 timestamp
}

// Streamer publishes task events to Redis Pub/Sub and persists to history.
type Streamer struct {
	redis      *redisclient.Client
	historyTTL time.Duration
}

// NewStreamer creates a new event streamer.
func NewStreamer(redis *redisclient.Client, historyTTL time.Duration) *Streamer {
	return &Streamer{
		redis:      redis,
		historyTTL: historyTTL,
	}
}

// Emit publishes an event to the task's stream channel and persists to history.
func (s *Streamer) Emit(ctx context.Context, taskID string, evt StreamEvent) error {
	evt.TS = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := string(data)

	streamKey := s.redis.Key("task", taskID, "stream")
	historyKey := s.redis.Key("task", taskID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, streamKey, msg)
	pipe.RPush(ctx, historyKey, msg)
	_, err = pipe.Exec(ctx)
	return err
}

// EmitSystem publishes a system event.
func (s *Streamer) EmitSystem(ctx context.Context, taskID, event string, data interface{}) error {
	return s.emitTyped(ctx, taskID, "system", event, data)
}

// EmitGit publishes a git event.
func (s *Streamer) EmitGit(ctx context.Context, taskID, event string, data interface{}) error {
	return s.emitTyped(ctx, taskID, "git", event, data)
}

// EmitCLI publishes a cli event.
func (s *Streamer) EmitCLI(ctx context.Context, taskID, event string, data interface{}) error {
	return s.emitTyped(ctx, taskID, "cli", event, data)
}

// EmitCLIOutput forwards a raw Claude Code stream-json line.
func (s *Streamer) EmitCLIOutput(ctx context.Context, taskID string, rawEvent json.RawMessage) error {
	return s.Emit(ctx, taskID, StreamEvent{
		Type:  "stream",
		Event: "output",
		Data:  rawEvent,
	})
}

// EmitResult publishes a result event.
func (s *Streamer) EmitResult(ctx context.Context, taskID, event string, data interface{}) error {
	return s.emitTyped(ctx, taskID, "result", event, data)
}

// EmitDone publishes completion signal on the done channel and sets history TTL.
func (s *Streamer) EmitDone(ctx context.Context, taskID string, status task.TaskStatus, summary *gitpkg.ChangesSummary) error {
	data, _ := json.Marshal(map[string]interface{}{
		"task_id":         taskID,
		"status":          status,
		"changes_summary": summary,
	})

	doneKey := s.redis.Key("task", taskID, "done")
	historyKey := s.redis.Key("task", taskID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, doneKey, string(data))
	pipe.Expire(ctx, historyKey, s.historyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Streamer) emitTyped(ctx context.Context, taskID, eventType, event string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.Emit(ctx, taskID, StreamEvent{
		Type:  eventType,
		Event: event,
		Data:  raw,
	})
}
