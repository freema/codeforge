package schedule

import (
	"context"
	"testing"
)

func TestStore_RunHistory(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "@daily", true)
	other := seedSchedule(t, store, "@daily", true)
	ctx := context.Background()

	runs := []*Run{
		{ScheduleID: sch.ID, SessionID: "sess-1", Trigger: TriggerCron, Status: RunFired},
		{ScheduleID: sch.ID, Trigger: TriggerCron, Status: RunFireFailed, Error: "boom"},
		{ScheduleID: sch.ID, SessionID: "sess-2", Trigger: TriggerManual, Status: RunFired},
		{ScheduleID: other.ID, SessionID: "sess-9", Trigger: TriggerCron, Status: RunFired},
	}
	for _, run := range runs {
		if err := store.InsertRun(ctx, run); err != nil {
			t.Fatalf("InsertRun: %v", err)
		}
		if run.ID == "" || run.CreatedAt.IsZero() {
			t.Fatalf("InsertRun did not assign id/timestamps: %+v", run)
		}
	}

	got, err := store.ListRuns(ctx, sch.ID, 50)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListRuns returned %d runs, want 3 (must exclude other schedules)", len(got))
	}
	// Newest first — the manual run was inserted last.
	if got[0].SessionID != "sess-2" || got[0].Trigger != TriggerManual {
		t.Errorf("newest run = %+v, want the manual sess-2 run", got[0])
	}
	if got[1].Status != RunFireFailed || got[1].Error != "boom" {
		t.Errorf("second run = %+v, want fire_failed with error", got[1])
	}

	// Limit applies after newest-first ordering.
	limited, err := store.ListRuns(ctx, sch.ID, 1)
	if err != nil {
		t.Fatalf("ListRuns limited: %v", err)
	}
	if len(limited) != 1 || limited[0].SessionID != "sess-2" {
		t.Errorf("limited ListRuns = %+v, want only the newest run", limited)
	}
}

func TestStore_UpdateRunBySession(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "@daily", true)
	ctx := context.Background()

	tests := []struct {
		name       string
		sessionID  string
		status     string
		errMsg     string
		wantStatus string
	}{
		{"session completed", "sess-1", RunSessionCompleted, "", RunSessionCompleted},
		{"session failed with error", "sess-1", RunSessionFailed, "clone failed", RunSessionFailed},
	}
	if err := store.InsertRun(ctx, &Run{ScheduleID: sch.ID, SessionID: "sess-1", Trigger: TriggerCron, Status: RunFired}); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.UpdateRunBySession(ctx, tt.sessionID, tt.status, tt.errMsg); err != nil {
				t.Fatalf("UpdateRunBySession: %v", err)
			}
			runs, err := store.ListRuns(ctx, sch.ID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if runs[0].Status != tt.wantStatus || runs[0].Error != tt.errMsg {
				t.Errorf("run after update = %+v, want status %s error %q", runs[0], tt.wantStatus, tt.errMsg)
			}
		})
	}

	// Unknown session id — silently a no-op.
	if err := store.UpdateRunBySession(ctx, "sess-unknown", RunSessionFailed, "x"); err != nil {
		t.Errorf("UpdateRunBySession for unknown session: %v", err)
	}
}

func TestStore_FailureTrackingAndDisable(t *testing.T) {
	store := newTestStore(t)
	sch := seedSchedule(t, store, "@daily", true)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		n, err := store.IncrementFailures(ctx, sch.ID)
		if err != nil {
			t.Fatalf("IncrementFailures: %v", err)
		}
		if n != i {
			t.Fatalf("IncrementFailures returned %d, want %d", n, i)
		}
	}

	if err := store.ResetFailures(ctx, sch.ID); err != nil {
		t.Fatalf("ResetFailures: %v", err)
	}
	got, err := store.Get(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures after reset = %d, want 0", got.ConsecutiveFailures)
	}

	if err := store.Disable(ctx, sch.ID, "auto-disabled after 5 consecutive failures"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	got, err = store.Get(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("schedule still enabled after Disable")
	}
	if got.DisabledReason != "auto-disabled after 5 consecutive failures" {
		t.Errorf("DisabledReason = %q", got.DisabledReason)
	}

	if _, err := store.IncrementFailures(ctx, "nope"); err != ErrNotFound {
		t.Errorf("IncrementFailures(unknown) = %v, want ErrNotFound", err)
	}
	if err := store.Disable(ctx, "nope", "r"); err != ErrNotFound {
		t.Errorf("Disable(unknown) = %v, want ErrNotFound", err)
	}
}

func TestStore_TimezoneRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	sch := seedSchedule(t, store, "0 3 * * *", true)
	sch.Timezone = "Europe/Prague"
	if err := store.Update(ctx, sch); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := store.Get(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Timezone != "Europe/Prague" {
		t.Errorf("Timezone = %q, want Europe/Prague", got.Timezone)
	}

	// Update also persists failure state (used by the re-enable clean slate).
	got.ConsecutiveFailures = 0
	got.DisabledReason = ""
	got.Enabled = true
	if err := store.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, err := store.Get(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ConsecutiveFailures != 0 || again.DisabledReason != "" || !again.Enabled {
		t.Errorf("re-enable clean slate not persisted: %+v", again)
	}

	// Deleting the schedule removes its run history.
	if err := store.InsertRun(ctx, &Run{ScheduleID: sch.ID, Trigger: TriggerCron, Status: RunFired}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, sch.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListRuns(ctx, sch.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("run history survived schedule deletion: %+v", runs)
	}
}
