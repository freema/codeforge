package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/notify"
	"github.com/freema/codeforge/internal/session"
)

type fakeCreator struct {
	calls []session.CreateSessionRequest
	next  int
}

func (f *fakeCreator) Create(_ context.Context, req session.CreateSessionRequest) (*session.Session, error) {
	f.calls = append(f.calls, req)
	f.next++
	return &session.Session{ID: "sess-" + string(rune('a'+f.next-1))}, nil
}

type failingCreator struct {
	calls int
}

func (f *failingCreator) Create(_ context.Context, _ session.CreateSessionRequest) (*session.Session, error) {
	f.calls++
	return nil, errors.New("queue unavailable")
}

type fakeGetter struct {
	statuses map[string]session.Status
	err      error
}

func (f *fakeGetter) Get(_ context.Context, id string) (*session.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	status, ok := f.statuses[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return &session.Session{ID: id, Status: status}, nil
}

type fakeNotifier struct {
	events []notify.Event
}

func (f *fakeNotifier) Notify(_ context.Context, ev notify.Event) {
	f.events = append(f.events, ev)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func seedSchedule(t *testing.T, store *Store, cronExpr string, enabled bool) *Schedule {
	t.Helper()
	sch := &Schedule{
		Name:           "nightly",
		Cron:           cronExpr,
		Enabled:        enabled,
		SessionRequest: json.RawMessage(`{"repo_url":"https://example.com/acme/widget.git","prompt":"update deps"}`),
	}
	if err := store.Create(context.Background(), sch); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	return sch
}

func TestParseCron(t *testing.T) {
	for _, ok := range []string{"0 3 * * *", "*/5 * * * *", "@daily", "@every 1h"} {
		if _, err := ParseCron(ok); err != nil {
			t.Errorf("ParseCron(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "not a cron", "61 3 * * *"} {
		if _, err := ParseCron(bad); err == nil {
			t.Errorf("ParseCron(%q) expected error", bad)
		}
	}
}

func TestRunDue_FiresDueSchedule(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "*/5 * * * *", true)
	creator := &fakeCreator{}
	s := NewScheduler(store, creator, time.Minute)

	// Next occurrence after CreatedAt is within 5 minutes — jump past it.
	s.RunDue(context.Background(), time.Now().Add(10*time.Minute))

	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 session created, got %d", len(creator.calls))
	}
	if creator.calls[0].RepoURL != "https://example.com/acme/widget.git" {
		t.Errorf("unexpected repo: %s", creator.calls[0].RepoURL)
	}
	if creator.calls[0].Metadata["schedule_id"] != sch.ID {
		t.Errorf("schedule_id metadata missing: %v", creator.calls[0].Metadata)
	}

	// Run marked: next RunDue at the same instant must NOT fire again.
	s.RunDue(context.Background(), time.Now().Add(10*time.Minute))
	if len(creator.calls) != 1 {
		t.Fatalf("catch-up backlog fired more than once: %d", len(creator.calls))
	}

	got, err := store.Get(context.Background(), sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastRunAt == nil || got.LastSessionID == "" {
		t.Errorf("run not recorded: %+v", got)
	}

	runs, err := store.ListRuns(context.Background(), sch.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != RunFired || runs[0].Trigger != TriggerCron || runs[0].SessionID == "" {
		t.Errorf("run history = %+v, want one fired cron run with session id", runs)
	}
}

func TestRunDue_SkipsFutureAndDisabled(t *testing.T) {
	store := newTestStore(t)
	seedSchedule(t, store, "0 3 1 1 *", true) // once a year — not due now
	seedSchedule(t, store, "*/5 * * * *", false)
	creator := &fakeCreator{}
	s := NewScheduler(store, creator, time.Minute)

	s.RunDue(context.Background(), time.Now())

	if len(creator.calls) != 0 {
		t.Fatalf("expected no sessions, got %d", len(creator.calls))
	}
}

func TestStore_UpdateAndDelete(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "@daily", true)

	sch.Enabled = false
	sch.Name = "renamed"
	if err := store.Update(context.Background(), sch); err != nil {
		t.Fatalf("update: %v", err)
	}

	enabled, err := store.ListEnabled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Errorf("disabled schedule still listed as enabled")
	}

	if err := store.Delete(context.Background(), sch.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(context.Background(), sch.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if _, err := store.Get(context.Background(), sch.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound on get, got %v", err)
	}
}

func TestShouldSkipOverlap(t *testing.T) {
	tests := []struct {
		name          string
		lastSessionID string
		getter        SessionGetter
		want          bool
	}{
		{"no previous session", "", &fakeGetter{}, false},
		{"getter not wired", "sess-1", nil, false},
		{"lookup error", "sess-1", &fakeGetter{err: errors.New("redis down")}, false},
		{"previous unknown", "sess-1", &fakeGetter{statuses: map[string]session.Status{}}, false},
		{"previous pending", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusPending}}, true},
		{"previous cloning", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusCloning}}, true},
		{"previous running", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusRunning}}, true},
		{"previous reviewing", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusReviewing}}, true},
		{"previous creating_pr", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusCreatingPR}}, true},
		{"previous completed", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusCompleted}}, false},
		{"previous awaiting_instruction", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusAwaitingInstruction}}, false},
		{"previous failed", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusFailed}}, false},
		{"previous canceled", "sess-1", &fakeGetter{statuses: map[string]session.Status{"sess-1": session.StatusCanceled}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scheduler{sessions: tt.getter}
			sch := &Schedule{ID: "sch-1", LastSessionID: tt.lastSessionID}
			if got := s.shouldSkipOverlap(context.Background(), sch); got != tt.want {
				t.Errorf("shouldSkipOverlap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunDue_OverlapSkipsAndRecords(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "*/5 * * * *", true)
	creator := &fakeCreator{}
	getter := &fakeGetter{statuses: map[string]session.Status{}}
	s := NewScheduler(store, creator, time.Minute)
	s.SetSessionGetter(getter)

	// First occurrence fires normally → sess-a.
	now1 := time.Now().Add(10 * time.Minute)
	s.RunDue(context.Background(), now1)
	if len(creator.calls) != 1 {
		t.Fatalf("expected first occurrence to fire, calls = %d", len(creator.calls))
	}

	// Previous session still running → next occurrence is skipped once.
	getter.statuses["sess-a"] = session.StatusRunning
	now2 := now1.Add(10 * time.Minute)
	s.RunDue(context.Background(), now2)
	if len(creator.calls) != 1 {
		t.Fatalf("overlap not skipped, calls = %d", len(creator.calls))
	}
	runs, err := store.ListRuns(context.Background(), sch.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Status != RunSkippedOverlap || runs[0].SessionID != "sess-a" {
		t.Fatalf("run history = %+v, want newest skipped_overlap for sess-a", runs)
	}

	// Skip consumed the occurrence — the same instant must not re-skip every tick.
	s.RunDue(context.Background(), now2)
	runs, _ = store.ListRuns(context.Background(), sch.ID, 10)
	if len(runs) != 2 {
		t.Fatalf("skip re-recorded every tick: %d runs", len(runs))
	}

	// Previous session finished → next occurrence fires again.
	getter.statuses["sess-a"] = session.StatusCompleted
	s.RunDue(context.Background(), now2.Add(10*time.Minute))
	if len(creator.calls) != 2 {
		t.Fatalf("schedule did not resume after previous session finished, calls = %d", len(creator.calls))
	}
}

func TestFillNextRun_Timezone(t *testing.T) {
	mustTime := func(v string) time.Time {
		t.Helper()
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}

	tests := []struct {
		name     string
		cron     string
		timezone string
		enabled  bool
		now      string
		want     string // empty = expect nil NextRunAt
	}{
		{"utc noon", "0 12 * * *", "UTC", true, "2026-01-15T00:00:00Z", "2026-01-15T12:00:00Z"},
		{"new york winter (EST, UTC-5)", "0 12 * * *", "America/New_York", true, "2026-01-15T00:00:00Z", "2026-01-15T17:00:00Z"},
		{"new york summer (EDT, UTC-4)", "0 12 * * *", "America/New_York", true, "2026-07-15T00:00:00Z", "2026-07-15T16:00:00Z"},
		{"tokyo (UTC+9)", "0 9 * * *", "Asia/Tokyo", true, "2026-01-15T12:00:00Z", "2026-01-16T00:00:00Z"},
		{"descriptor daily in prague (CET, UTC+1)", "@daily", "Europe/Prague", true, "2026-01-15T12:00:00Z", "2026-01-15T23:00:00Z"},
		{"invalid timezone", "0 12 * * *", "Not/AZone", true, "2026-01-15T00:00:00Z", ""},
		{"disabled", "0 12 * * *", "UTC", false, "2026-01-15T00:00:00Z", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sch := &Schedule{Cron: tt.cron, Timezone: tt.timezone, Enabled: tt.enabled}
			sch.FillNextRun(mustTime(tt.now))
			if tt.want == "" {
				if sch.NextRunAt != nil {
					t.Fatalf("NextRunAt = %v, want nil", sch.NextRunAt)
				}
				return
			}
			if sch.NextRunAt == nil {
				t.Fatal("NextRunAt = nil, want a value")
			}
			if want := mustTime(tt.want); !sch.NextRunAt.Equal(want) {
				t.Errorf("NextRunAt = %v, want %v", sch.NextRunAt.UTC(), want)
			}
		})
	}
}

func TestValidateTimezone(t *testing.T) {
	for _, ok := range []string{"", "UTC", "Europe/Prague", "America/New_York"} {
		if err := ValidateTimezone(ok); err != nil {
			t.Errorf("ValidateTimezone(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"Not/AZone", "CET+1junk", "prague"} {
		if err := ValidateTimezone(bad); err == nil {
			t.Errorf("ValidateTimezone(%q) expected error", bad)
		}
	}
}

func TestFire_FailureEscalationAutoDisables(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "*/5 * * * *", true)
	creator := &failingCreator{}
	notifier := &fakeNotifier{}
	s := NewScheduler(store, creator, time.Minute)
	s.SetNotifier(notifier)

	// Failed fires never MarkRun, so the schedule stays due and retries each
	// tick — the 5th consecutive failure must auto-disable it.
	now := time.Now().Add(10 * time.Minute)
	for i := 0; i < MaxConsecutiveFailures; i++ {
		s.RunDue(context.Background(), now)
	}

	if creator.calls != MaxConsecutiveFailures {
		t.Fatalf("creator called %d times, want %d", creator.calls, MaxConsecutiveFailures)
	}

	got, err := store.Get(context.Background(), sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("schedule still enabled after 5 consecutive fire failures")
	}
	if got.ConsecutiveFailures != MaxConsecutiveFailures {
		t.Errorf("ConsecutiveFailures = %d, want %d", got.ConsecutiveFailures, MaxConsecutiveFailures)
	}
	if got.DisabledReason == "" {
		t.Error("DisabledReason empty after auto-disable")
	}

	runs, err := store.ListRuns(context.Background(), sch.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != MaxConsecutiveFailures {
		t.Fatalf("run history has %d rows, want %d fire_failed", len(runs), MaxConsecutiveFailures)
	}
	for _, run := range runs {
		if run.Status != RunFireFailed || run.Error == "" {
			t.Errorf("run = %+v, want fire_failed with error", run)
		}
	}

	// 5 failure notifications + 1 final auto-disable notification.
	if len(notifier.events) != MaxConsecutiveFailures+1 {
		t.Fatalf("notifications = %d, want %d", len(notifier.events), MaxConsecutiveFailures+1)
	}
	for _, ev := range notifier.events {
		if ev.Type != notify.EventScheduleFailed {
			t.Errorf("notification type = %q, want %q", ev.Type, notify.EventScheduleFailed)
		}
		if ev.ScheduleName != "nightly" || ev.RepoURL == "" {
			t.Errorf("notification missing schedule name/repo: %+v", ev)
		}
	}
	if last := notifier.events[len(notifier.events)-1]; last.Error == "" || got.DisabledReason != last.Error {
		t.Errorf("final notification error = %q, want disable reason %q", last.Error, got.DisabledReason)
	}

	// Disabled schedule never fires again.
	s.RunDue(context.Background(), now.Add(time.Hour))
	if creator.calls != MaxConsecutiveFailures {
		t.Errorf("disabled schedule fired again, calls = %d", creator.calls)
	}
}

func TestRecordSessionOutcome(t *testing.T) {
	tests := []struct {
		name         string
		outcomes     []bool // succeeded, in order
		wantEnabled  bool
		wantFailures int
		wantStatus   string // final run row status
	}{
		{"single success resets", []bool{true}, true, 0, RunSessionCompleted},
		{"failures below threshold keep enabled", []bool{false, false, false, false}, true, 4, RunSessionFailed},
		{"success resets the streak", []bool{false, false, false, true, false}, true, 1, RunSessionFailed},
		{"five consecutive failures disable", []bool{false, false, false, false, false}, false, 5, RunSessionFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			sch := seedSchedule(t, store, "@daily", true)
			notifier := &fakeNotifier{}
			s := NewScheduler(store, &fakeCreator{}, time.Minute)
			s.SetNotifier(notifier)

			if err := store.InsertRun(context.Background(), &Run{
				ScheduleID: sch.ID, SessionID: "sess-1", Trigger: TriggerCron, Status: RunFired,
			}); err != nil {
				t.Fatal(err)
			}

			for _, succeeded := range tt.outcomes {
				errMsg := ""
				if !succeeded {
					errMsg = "CLI execution failed"
				}
				s.RecordSessionOutcome(context.Background(), sch.ID, "sess-1", succeeded, errMsg)
			}

			got, err := store.Get(context.Background(), sch.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.wantEnabled)
			}
			if got.ConsecutiveFailures != tt.wantFailures {
				t.Errorf("ConsecutiveFailures = %d, want %d", got.ConsecutiveFailures, tt.wantFailures)
			}
			if !tt.wantEnabled && got.DisabledReason == "" {
				t.Error("DisabledReason empty after auto-disable")
			}

			runs, err := store.ListRuns(context.Background(), sch.ID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || runs[0].Status != tt.wantStatus {
				t.Errorf("run row = %+v, want status %s", runs, tt.wantStatus)
			}

			// Only the auto-disable sends a schedule notification here (plain
			// session failures already produce session_failed notifications).
			wantNotifs := 0
			if !tt.wantEnabled {
				wantNotifs = 1
			}
			if len(notifier.events) != wantNotifs {
				t.Errorf("notifications = %d, want %d", len(notifier.events), wantNotifs)
			}
		})
	}
}
