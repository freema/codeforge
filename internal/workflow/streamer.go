package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
)

// Streamer publishes workflow-level events to Redis Pub/Sub and history.
type Streamer struct {
	redis      *redisclient.Client
	historyTTL time.Duration
}

// NewStreamer creates a new workflow event streamer.
func NewStreamer(redis *redisclient.Client, historyTTL time.Duration) *Streamer {
	return &Streamer{
		redis:      redis,
		historyTTL: historyTTL,
	}
}

type workflowEvent struct {
	Type  string          `json:"type"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
	TS    string          `json:"ts"`
}

// EmitSystem publishes a system-level workflow event.
func (s *Streamer) EmitSystem(ctx context.Context, runID, event string, data interface{}) error {
	raw, _ := json.Marshal(data)
	return s.emit(ctx, runID, workflowEvent{
		Type:  "system",
		Event: event,
		Data:  raw,
		TS:    time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// EmitDone publishes the workflow completion signal.
func (s *Streamer) EmitDone(ctx context.Context, runID string, status RunStatus) error {
	data, _ := json.Marshal(map[string]interface{}{
		"run_id": runID,
		"status": status,
	})

	doneKey := s.redis.Key("workflow", runID, "done")
	historyKey := s.redis.Key("workflow", runID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, doneKey, string(data))
	pipe.Expire(ctx, historyKey, s.historyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Streamer) emit(ctx context.Context, runID string, evt workflowEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := string(data)

	streamKey := s.redis.Key("workflow", runID, "stream")
	historyKey := s.redis.Key("workflow", runID, "history")

	pipe := s.redis.Unwrap().Pipeline()
	pipe.Publish(ctx, streamKey, msg)
	pipe.RPush(ctx, historyKey, msg)
	_, err = pipe.Exec(ctx)
	return err
}
