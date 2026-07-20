package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
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
