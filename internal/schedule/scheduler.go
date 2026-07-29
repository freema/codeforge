package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/freema/codeforge/internal/blueprint"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/notify"
	"github.com/freema/codeforge/internal/session"
)

// SessionCreator creates sessions — implemented by *session.Service.
type SessionCreator interface {
	Create(ctx context.Context, req session.CreateSessionRequest) (*session.Session, error)
}

// SessionGetter looks up session state for the overlap guard — implemented by
// *session.Service. Optional (nil = overlap guard disabled).
type SessionGetter interface {
	Get(ctx context.Context, sessionID string) (*session.Session, error)
}

// Notifier posts chat notifications for schedule failures — implemented by
// *notify.Notifier. Optional (nil = notifications disabled).
type Notifier interface {
	Notify(ctx context.Context, ev notify.Event)
}

// BlueprintSource resolves blueprints for blueprint-backed schedules —
// implemented by *blueprint.Store. Nil disables blueprint-backed firing
// (such schedules fail with a fire_failed run).
type BlueprintSource interface {
	Get(ctx context.Context, id string) (*blueprint.Blueprint, error)
}

// Scheduler fires enabled schedules whose cron expression is due.
// One firing per schedule per tick — a backlog of missed occurrences
// (server downtime) collapses into a single catch-up run.
type Scheduler struct {
	store      *Store
	creator    SessionCreator
	blueprints BlueprintSource // optional — blueprint-backed schedules
	keys       keys.Registry   // optional — tool_key_ref resolution in blueprint.Build
	sessions   SessionGetter   // optional
	notifier   Notifier        // optional
	interval   time.Duration
}

// NewScheduler creates a scheduler that checks for due schedules every
// interval. blueprints and keyReg feed blueprint.Build for blueprint-backed
// schedules; both may be nil in setups without blueprints.
func NewScheduler(store *Store, creator SessionCreator, blueprints BlueprintSource, keyReg keys.Registry, interval time.Duration) *Scheduler {
	return &Scheduler{store: store, creator: creator, blueprints: blueprints, keys: keyReg, interval: interval}
}

// SetSessionGetter wires session-state lookups for the overlap guard.
// Optional — when unset, occurrences fire even while the previous one runs.
func (s *Scheduler) SetSessionGetter(g SessionGetter) {
	s.sessions = g
}

// SetNotifier wires chat notifications for schedule failures.
// Optional — when unset, no notifications are sent.
func (s *Scheduler) SetNotifier(n Notifier) {
	s.notifier = n
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
		spec, err := sch.CronSchedule()
		if err != nil {
			slog.Warn("scheduler: skipping schedule with invalid cron", "schedule_id", sch.ID, "cron", sch.Cron, "timezone", sch.Timezone)
			continue
		}

		base := sch.CreatedAt
		if sch.LastRunAt != nil {
			base = *sch.LastRunAt
		}
		if spec.Next(base).After(now) {
			continue
		}

		if s.shouldSkipOverlap(ctx, sch) {
			s.recordOverlapSkip(ctx, sch, now)
			continue
		}

		if _, err := s.Fire(ctx, sch, now, TriggerCron); err != nil {
			slog.Error("scheduler: firing schedule failed", "schedule_id", sch.ID, "name", sch.Name, "error", err)
		}
	}
}

// shouldSkipOverlap reports whether the schedule's previous session is still
// being processed, in which case this occurrence must not fire on top of it.
func (s *Scheduler) shouldSkipOverlap(ctx context.Context, sch *Schedule) bool {
	if s.sessions == nil || sch.LastSessionID == "" {
		return false
	}
	prev, err := s.sessions.Get(ctx, sch.LastSessionID)
	if err != nil || prev == nil {
		// Unknown/expired session must never wedge the schedule.
		return false
	}
	return session.IsActive(prev.Status)
}

// recordOverlapSkip marks the occurrence consumed (so it does not re-fire
// every tick) and records a skipped_overlap run.
func (s *Scheduler) recordOverlapSkip(ctx context.Context, sch *Schedule, now time.Time) {
	if err := s.store.MarkRun(ctx, sch.ID, now, sch.LastSessionID); err != nil {
		slog.Warn("scheduler: marking skipped run failed", "schedule_id", sch.ID, "error", err)
	}
	if err := s.store.InsertRun(ctx, &Run{
		ScheduleID: sch.ID,
		SessionID:  sch.LastSessionID,
		Trigger:    TriggerCron,
		Status:     RunSkippedOverlap,
	}); err != nil {
		slog.Warn("scheduler: recording skipped run failed", "schedule_id", sch.ID, "error", err)
	}
	slog.Info("scheduler: occurrence skipped — previous session still active",
		"schedule_id", sch.ID, "name", sch.Name, "session_id", sch.LastSessionID)
}

// Fire creates the schedule's session and records the occurrence. trigger is
// TriggerCron (scheduler tick — shifts the cron base) or TriggerManual
// (run-now endpoint — records the session without shifting the cron base).
func (s *Scheduler) Fire(ctx context.Context, sch *Schedule, now time.Time, trigger string) (*session.Session, error) {
	req, err := s.buildRequest(ctx, sch)
	if err != nil {
		s.recordFireFailure(ctx, sch, trigger, err)
		return nil, err
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["schedule_id"] = sch.ID
	req.Metadata["schedule_name"] = sch.Name

	t, err := s.creator.Create(ctx, *req)
	if err != nil {
		err = fmt.Errorf("creating session: %w", err)
		s.recordFireFailure(ctx, sch, trigger, err)
		return nil, err
	}

	if trigger == TriggerCron {
		if err := s.store.MarkRun(ctx, sch.ID, now, t.ID); err != nil {
			slog.Warn("scheduler: recording run failed", "schedule_id", sch.ID, "error", err)
		}
	} else {
		if err := s.store.SetLastSession(ctx, sch.ID, t.ID); err != nil {
			slog.Warn("scheduler: recording manual run failed", "schedule_id", sch.ID, "error", err)
		}
	}
	if err := s.store.InsertRun(ctx, &Run{
		ScheduleID: sch.ID,
		SessionID:  t.ID,
		Trigger:    trigger,
		Status:     RunFired,
	}); err != nil {
		slog.Warn("scheduler: recording run history failed", "schedule_id", sch.ID, "error", err)
	}
	if err := s.store.ResetFailures(ctx, sch.ID); err != nil {
		slog.Warn("scheduler: resetting failures failed", "schedule_id", sch.ID, "error", err)
	}

	slog.Info("scheduler: schedule fired", "schedule_id", sch.ID, "name", sch.Name, "session_id", t.ID, "trigger", trigger)
	return t, nil
}

// buildRequest produces the session request for one occurrence: the rendered
// blueprint for blueprint-backed schedules, the stored inline template
// otherwise. Blueprint-backed requests get a blueprint_name metadata entry.
func (s *Scheduler) buildRequest(ctx context.Context, sch *Schedule) (*session.CreateSessionRequest, error) {
	if sch.BlueprintID == "" {
		var req session.CreateSessionRequest
		if err := json.Unmarshal(sch.SessionRequest, &req); err != nil {
			return nil, fmt.Errorf("decoding session request: %w", err)
		}
		return &req, nil
	}

	if s.blueprints == nil {
		return nil, fmt.Errorf("schedule references blueprint %s but no blueprint source is configured", sch.BlueprintID)
	}
	bp, err := s.blueprints.Get(ctx, sch.BlueprintID)
	if err != nil {
		return nil, fmt.Errorf("loading blueprint %s: %w", sch.BlueprintID, err)
	}
	req, err := blueprint.Build(ctx, bp, sch.BlueprintParams, s.keys)
	if err != nil {
		return nil, fmt.Errorf("building blueprint %q: %w", bp.Name, err)
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["blueprint_name"] = bp.Name
	return req, nil
}

// RecordSessionOutcome implements the worker's schedule feedback: it stamps
// the session's run row with the terminal status and drives the consecutive
// failure counter (auto-disabling the schedule at MaxConsecutiveFailures).
func (s *Scheduler) RecordSessionOutcome(ctx context.Context, scheduleID, sessionID string, succeeded bool, errMsg string) {
	status := RunSessionCompleted
	if !succeeded {
		status = RunSessionFailed
	}
	if err := s.store.UpdateRunBySession(ctx, sessionID, status, errMsg); err != nil {
		slog.Warn("scheduler: updating run outcome failed", "schedule_id", scheduleID, "session_id", sessionID, "error", err)
	}

	if succeeded {
		if err := s.store.ResetFailures(ctx, scheduleID); err != nil {
			slog.Warn("scheduler: resetting failures failed", "schedule_id", scheduleID, "error", err)
		}
		return
	}

	sch, err := s.store.Get(ctx, scheduleID)
	if err != nil {
		slog.Warn("scheduler: loading schedule for outcome failed", "schedule_id", scheduleID, "error", err)
		return
	}
	s.escalateFailure(ctx, sch, errMsg)
}

// recordFireFailure records a fire_failed run, notifies, and escalates the
// failure counter.
func (s *Scheduler) recordFireFailure(ctx context.Context, sch *Schedule, trigger string, fireErr error) {
	if err := s.store.InsertRun(ctx, &Run{
		ScheduleID: sch.ID,
		Trigger:    trigger,
		Status:     RunFireFailed,
		Error:      fireErr.Error(),
	}); err != nil {
		slog.Warn("scheduler: recording failed run failed", "schedule_id", sch.ID, "error", err)
	}
	s.notifyScheduleFailed(ctx, sch, fireErr.Error())
	s.escalateFailure(ctx, sch, fireErr.Error())
}

// escalateFailure bumps consecutive_failures and auto-disables the schedule
// once it reaches MaxConsecutiveFailures, sending one final notification.
func (s *Scheduler) escalateFailure(ctx context.Context, sch *Schedule, errMsg string) {
	n, err := s.store.IncrementFailures(ctx, sch.ID)
	if err != nil {
		slog.Warn("scheduler: incrementing failures failed", "schedule_id", sch.ID, "error", err)
		return
	}
	if n < MaxConsecutiveFailures {
		return
	}

	reason := fmt.Sprintf("auto-disabled after %d consecutive failures; last error: %s", n, errMsg)
	if err := s.store.Disable(ctx, sch.ID, reason); err != nil {
		slog.Warn("scheduler: auto-disabling schedule failed", "schedule_id", sch.ID, "error", err)
		return
	}
	slog.Warn("scheduler: schedule auto-disabled", "schedule_id", sch.ID, "name", sch.Name, "consecutive_failures", n)
	s.notifyScheduleFailed(ctx, sch, reason)
}

// notifyScheduleFailed posts a schedule_failed chat notification (nil-safe).
func (s *Scheduler) notifyScheduleFailed(ctx context.Context, sch *Schedule, errMsg string) {
	if s.notifier == nil {
		return
	}
	s.notifier.Notify(ctx, notify.Event{
		Type:         notify.EventScheduleFailed,
		ScheduleName: sch.Name,
		RepoURL:      repoURLFromRequest(sch.SessionRequest),
		Error:        errMsg,
	})
}

// repoURLFromRequest extracts repo_url from the stored session request template.
func repoURLFromRequest(raw json.RawMessage) string {
	var req struct {
		RepoURL string `json:"repo_url"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return ""
	}
	return req.RepoURL
}
