package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/freema/codeforge/internal/session"
)

// SessionCreator creates sessions — implemented by *session.Service.
type SessionCreator interface {
	Create(ctx context.Context, req session.CreateSessionRequest) (*session.Session, error)
}

// Scheduler fires enabled schedules whose cron expression is due.
// One firing per schedule per tick — a backlog of missed occurrences
// (server downtime) collapses into a single catch-up run.
type Scheduler struct {
	store    *Store
	creator  SessionCreator
	interval time.Duration
}

// NewScheduler creates a scheduler that checks for due schedules every interval.
func NewScheduler(store *Store, creator SessionCreator, interval time.Duration) *Scheduler {
	return &Scheduler{store: store, creator: creator, interval: interval}
}

// Start runs the scheduling loop until ctx is canceled. Call in a goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunDue(ctx, time.Now())
		}
	}
}

// RunDue fires every enabled schedule whose next occurrence (after its last
// run) is not in the future. Exported for tests and deterministic invocation.
func (s *Scheduler) RunDue(ctx context.Context, now time.Time) {
	items, err := s.store.ListEnabled(ctx)
	if err != nil {
		slog.Warn("scheduler: listing schedules failed", "error", err)
		return
	}

	for _, sch := range items {
		spec, err := ParseCron(sch.Cron)
		if err != nil {
			slog.Warn("scheduler: skipping schedule with invalid cron", "schedule_id", sch.ID, "cron", sch.Cron)
			continue
		}

		base := sch.CreatedAt
		if sch.LastRunAt != nil {
			base = *sch.LastRunAt
		}
		if spec.Next(base).After(now) {
			continue
		}

		if _, err := s.Fire(ctx, sch, now); err != nil {
			slog.Error("scheduler: firing schedule failed", "schedule_id", sch.ID, "name", sch.Name, "error", err)
		}
	}
}

// Fire creates the schedule's session and records the run. Also used by the
// run-now API endpoint.
func (s *Scheduler) Fire(ctx context.Context, sch *Schedule, now time.Time) (*session.Session, error) {
	var req session.CreateSessionRequest
	if err := json.Unmarshal(sch.SessionRequest, &req); err != nil {
		return nil, fmt.Errorf("decoding session request: %w", err)
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["schedule_id"] = sch.ID
	req.Metadata["schedule_name"] = sch.Name

	t, err := s.creator.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	if err := s.store.MarkRun(ctx, sch.ID, now, t.ID); err != nil {
		slog.Warn("scheduler: recording run failed", "schedule_id", sch.ID, "error", err)
	}

	slog.Info("scheduler: schedule fired", "schedule_id", sch.ID, "name", sch.Name, "session_id", t.ID)
	return t, nil
}
