package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/session"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tool/runner"
)

// StreamEvent is a structured event published to Redis Pub/Sub.
type StreamEvent struct {
	Type  string          `json:"type"`  // system, git, cli, stream, result
	Event string          `json:"event"` // event name
	Data  json.RawMessage `json:"data"`  // event-specific payload
	TS    string          `json:"ts"`    // ISO 8601 timestamp
}

// Streamer publishes session events to Redis Pub/Sub and persists to history.
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

// Emit publishes an event to the session's stream channel and persists to history.
func (s *Streamer) Emit(ctx context.Context, sessionID string, evt StreamEvent) error {
	evt.TS = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := string(data)

	streamKey := s.redis.Key("session", sessionID, "stream")
	historyKey := s.redis.Key("session", sessionID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, streamKey, msg)
	pipe.RPush(ctx, historyKey, msg)
	_, err = pipe.Exec(ctx)
	return err
}

// EmitSystem publishes a system event.
func (s *Streamer) EmitSystem(ctx context.Context, sessionID, event string, data interface{}) error {
	return s.emitTyped(ctx, sessionID, "system", event, data)
}

// EmitGit publishes a git event.
func (s *Streamer) EmitGit(ctx context.Context, sessionID, event string, data interface{}) error {
	return s.emitTyped(ctx, sessionID, "git", event, data)
}

// EmitNormalized publishes a normalized CLI event.
func (s *Streamer) EmitNormalized(ctx context.Context, sessionID string, evt *runner.NormalizedEvent) error {
	raw, _ := json.Marshal(evt)
	return s.Emit(ctx, sessionID, StreamEvent{
		Type:  "stream",
		Event: string(evt.Type),
		Data:  raw,
	})
}

// EmitCLIOutput forwards a raw Claude Code stream-json line.
func (s *Streamer) EmitCLIOutput(ctx context.Context, sessionID string, rawEvent json.RawMessage) error {
	return s.Emit(ctx, sessionID, StreamEvent{
		Type:  "stream",
		Event: "output",
		Data:  rawEvent,
	})
}

// EmitResult publishes a result event.
func (s *Streamer) EmitResult(ctx context.Context, sessionID, event string, data interface{}) error {
	return s.emitTyped(ctx, sessionID, "result", event, data)
}

// EmitDone publishes completion signal on the done channel and sets history TTL.
func (s *Streamer) EmitDone(ctx context.Context, sessionID string, status session.Status, summary *gitpkg.ChangesSummary) error {
	data, _ := json.Marshal(map[string]interface{}{
		"task_id":         sessionID,
		"status":          status,
		"changes_summary": summary,
	})

	doneKey := s.redis.Key("session", sessionID, "done")
	historyKey := s.redis.Key("session", sessionID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, doneKey, string(data))
	pipe.Expire(ctx, historyKey, s.historyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Streamer) emitTyped(ctx context.Context, sessionID, eventType, event string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.Emit(ctx, sessionID, StreamEvent{
		Type:  eventType,
		Event: event,
		Data:  raw,
	})
}
