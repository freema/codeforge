package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/freema/codeforge/internal/session"
)

// StuckSweeper periodically fails sessions that claim to be actively
// processing (running/cloning) long past any possible execution timeout —
// their worker is gone (crash, pre-recovery restart, lost requeue). Without
// it such sessions would sit in "running" forever.
type StuckSweeper struct {
	sessionService *session.Service
	interval       time.Duration
	maxAge         time.Duration
}

// NewStuckSweeper creates a sweeper. maxAge should comfortably exceed the
// maximum session timeout so a legitimately long run is never touched.
func NewStuckSweeper(sessionService *session.Service, interval, maxAge time.Duration) *StuckSweeper {
	return &StuckSweeper{
		sessionService: sessionService,
		interval:       interval,
		maxAge:         maxAge,
	}
}

// Start runs the sweep loop until ctx is canceled. Call in a goroutine.
func (s *StuckSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// One sweep shortly after boot to clear leftovers from before the
	// reliable-queue era (or a failed recovery).
	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *StuckSweeper) sweep(ctx context.Context) {
	ids, err := s.sessionService.ListStuck(ctx, time.Now().Add(-s.maxAge))
	if err != nil {
		slog.Warn("stuck sweeper: listing failed", "error", err)
		return
	}

	for _, id := range ids {
		// UpdateStatus validates the transition against the live Redis state,
		// so a session that moved on since the SQLite query is left alone.
		if err := s.sessionService.UpdateStatus(ctx, id, session.StatusFailed); err != nil {
			slog.Info("stuck sweeper: session skipped", "session_id", id, "error", err)
			continue
		}
		if err := s.sessionService.SetError(ctx, id, "session stuck without a worker — marked failed by the stuck sweeper"); err != nil {
			slog.Warn("stuck sweeper: storing error failed", "session_id", id, "error", err)
		}
		slog.Warn("stuck sweeper: session marked failed", "session_id", id, "older_than", s.maxAge)
	}
}
