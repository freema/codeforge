package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/metrics"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/session"
)

// errCanceledByUser marks a session context canceled via the cancel API,
// as opposed to a pool-wide shutdown. The executor uses the distinction to
// pick between the canceled status (user intent) and a restart requeue.
var errCanceledByUser = errors.New("canceled by user")

// Pool is a worker pool that consumes sessions from a Redis queue.
//
// Reliability: sessions are moved atomically from the queue into a processing
// list (BLMOVE) while being worked on and removed only after the executor
// returns. Entries left behind by a crash or shutdown are recovered on the
// next Start — non-terminal sessions are requeued, terminal ones dropped.
type Pool struct {
	redis          *redisclient.Client
	executor       *Executor
	sessionService *session.Service
	queueName      string
	concurrency    int
	wg             sync.WaitGroup
	cancel         context.CancelFunc
	activeCount    atomic.Int32
	cancels        map[string]context.CancelCauseFunc
	cancelsMu      sync.RWMutex
}

// NewPool creates a new worker pool.
func NewPool(
	redis *redisclient.Client,
	executor *Executor,
	sessionService *session.Service,
	queueName string,
	concurrency int,
) *Pool {
	return &Pool{
		redis:          redis,
		executor:       executor,
		sessionService: sessionService,
		queueName:      queueName,
		concurrency:    concurrency,
		cancels:        make(map[string]context.CancelCauseFunc),
	}
}

func (p *Pool) queueKey() string {
	return p.redis.Key(p.queueName)
}

func (p *Pool) processingKey() string {
	return p.redis.Key(p.queueName + ":processing")
}

// Start recovers sessions orphaned by the previous run, then launches workers.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	slog.Info("starting worker pool", "concurrency", p.concurrency, "queue", p.queueName)

	p.recoverProcessing(ctx)

	metrics.WorkersTotal.Set(float64(p.concurrency))

	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// recoverProcessing requeues sessions that were mid-flight when the previous
// process died (crash or shutdown). Interrupted running/cloning sessions are
// reset to pending; terminal or unknown entries are dropped from the list.
func (p *Pool) recoverProcessing(ctx context.Context) {
	ids, err := p.redis.Unwrap().LRange(ctx, p.processingKey(), 0, -1).Result()
	if err != nil {
		slog.Error("queue recovery: reading processing list failed", "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	slog.Info("queue recovery: found in-flight sessions from previous run", "count", len(ids))

	for _, id := range ids {
		p.recoverOne(ctx, id)
	}
}

func (p *Pool) recoverOne(ctx context.Context, sessionID string) {
	log := slog.With("session_id", sessionID)
	dropEntry := func() {
		if err := p.redis.Unwrap().LRem(ctx, p.processingKey(), 1, sessionID).Err(); err != nil {
			log.Warn("queue recovery: dropping processing entry failed", "error", err)
		}
	}

	t, err := p.sessionService.Get(ctx, sessionID)
	if err != nil {
		log.Warn("queue recovery: session not found, dropping entry", "error", err)
		dropEntry()
		return
	}

	switch t.Status {
	case session.StatusRunning, session.StatusCloning:
		// Interrupted mid-execution — back to pending so shouldProcess accepts it.
		if err := p.sessionService.UpdateStatus(ctx, sessionID, session.StatusPending); err != nil {
			log.Error("queue recovery: resetting session to pending failed, dropping", "error", err)
			dropEntry()
			return
		}
	case session.StatusPending, session.StatusAwaitingInstruction, session.StatusReviewing:
		// Dequeued but not started (or an interrupted review) — requeue as is.
	default:
		// Terminal — nothing to do.
		dropEntry()
		return
	}

	// Move back to the FRONT of the queue so interrupted work resumes first.
	pipe := p.redis.Unwrap().Pipeline()
	pipe.LRem(ctx, p.processingKey(), 1, sessionID)
	pipe.LPush(ctx, p.queueKey(), sessionID)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Error("queue recovery: requeue failed", "error", err)
		return
	}
	log.Info("queue recovery: session requeued", "status", t.Status)
}

// Stop signals workers to stop and waits for them to finish. In-flight
// sessions are interrupted; the executor resets them to pending and their
// processing-list entries make the next Start requeue them.
func (p *Pool) Stop() {
	slog.Info("stopping worker pool...")
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("worker pool stopped")
}

// Cancel cancels a running session by its ID (user-initiated).
func (p *Pool) Cancel(sessionID string) error {
	p.cancelsMu.RLock()
	cancelFn, ok := p.cancels[sessionID]
	p.cancelsMu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s is not currently running", sessionID)
	}
	cancelFn(errCanceledByUser)
	return nil
}

// shouldProcess returns true if a session status is actionable by the worker pool.
// Sessions in other states are stale queue entries that should be skipped.
func shouldProcess(s session.Status) bool {
	switch s {
	case session.StatusPending, session.StatusAwaitingInstruction, session.StatusReviewing:
		return true
	}
	return false
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	log := slog.With("worker", id)
	log.Info("worker started")

	queueKey := p.queueKey()
	processingKey := p.processingKey()

	for {
		// Atomically move the next session into the processing list so it
		// survives a crash between dequeue and completion.
		sessionID, err := p.redis.Unwrap().BLMove(ctx, queueKey, processingKey, "LEFT", "RIGHT", 5*time.Second).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // timeout, try again
			}
			if ctx.Err() != nil {
				log.Info("worker shutting down")
				return
			}
			log.Error("queue pop failed", "error", err)
			time.Sleep(1 * time.Second) // backoff on error
			continue
		}

		log.Info("picked up session", "session_id", sessionID)
		p.activeCount.Add(1)
		metrics.WorkersActive.Set(float64(p.activeCount.Load()))

		// Update queue depth (approximate)
		if qLen, err := p.redis.Unwrap().LLen(ctx, queueKey).Result(); err == nil {
			metrics.QueueDepth.Set(float64(qLen))
		}

		p.processOne(ctx, sessionID, log)

		p.activeCount.Add(-1)
		metrics.WorkersActive.Set(float64(p.activeCount.Load()))
	}
}

func (p *Pool) processOne(ctx context.Context, sessionID string, log *slog.Logger) {
	// Load session from Redis
	t, err := p.sessionService.Get(ctx, sessionID)
	if err != nil {
		log.Warn("failed to load session, skipping", "session_id", sessionID, "error", err)
		p.finishProcessing(sessionID, log)
		return
	}

	// Guard against stale/duplicate queue entries — only actionable states proceed
	if !shouldProcess(t.Status) {
		log.Warn("skipping stale queue entry", "session_id", sessionID, "status", t.Status)
		p.finishProcessing(sessionID, log)
		return
	}

	// Session-specific context, cancellable with a cause so the executor can
	// tell a user cancel apart from a pool shutdown.
	sessionCtx, sessionCancel := context.WithCancelCause(ctx)

	p.cancelsMu.Lock()
	p.cancels[sessionID] = sessionCancel
	p.cancelsMu.Unlock()

	p.executor.Execute(sessionCtx, t)

	p.cancelsMu.Lock()
	delete(p.cancels, sessionID)
	p.cancelsMu.Unlock()
	sessionCancel(nil) // clean up context resources

	if ctx.Err() != nil {
		// Shutdown interrupted this session: keep the processing-list entry so
		// the next start requeues it (the executor has reset it to pending).
		log.Info("session interrupted by shutdown, leaving in processing list", "session_id", sessionID)
		return
	}
	p.finishProcessing(sessionID, log)
}

// finishProcessing acknowledges a dequeued session by removing it from the
// processing list. Uses a detached context — this must succeed even mid-shutdown.
func (p *Pool) finishProcessing(sessionID string, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.redis.Unwrap().LRem(ctx, p.processingKey(), 1, sessionID).Err(); err != nil {
		log.Warn("failed to ack processing entry", "session_id", sessionID, "error", err)
	}
}
