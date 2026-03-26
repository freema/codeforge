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

// Pool is a worker pool that consumes sessions from a Redis queue.
type Pool struct {
	redis       *redisclient.Client
	executor    *Executor
	sessionService *session.Service
	queueName   string
	concurrency int
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	activeCount atomic.Int32
	cancels     map[string]context.CancelFunc
	cancelsMu   sync.RWMutex
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
		redis:       redis,
		executor:    executor,
		sessionService: sessionService,
		queueName:   queueName,
		concurrency: concurrency,
		cancels:     make(map[string]context.CancelFunc),
	}
}

// Start launches all worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	slog.Info("starting worker pool", "concurrency", p.concurrency, "queue", p.queueName)

	metrics.WorkersTotal.Set(float64(p.concurrency))

	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// Stop signals workers to stop and waits for them to finish.
func (p *Pool) Stop() {
	slog.Info("stopping worker pool...")
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("worker pool stopped")
}

// Cancel cancels a running session by its ID.
func (p *Pool) Cancel(sessionID string) error {
	p.cancelsMu.RLock()
	cancelFn, ok := p.cancels[sessionID]
	p.cancelsMu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s is not currently running", sessionID)
	}
	cancelFn()
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

	queueKey := p.redis.Key(p.queueName)

	for {
		// BLPOP blocks until item available or timeout
		result, err := p.redis.Unwrap().BLPop(ctx, 5*time.Second, queueKey).Result()
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

		sessionID := result[1] // result[0] = key name

		log.Info("picked up session", "session_id", sessionID)
		p.activeCount.Add(1)
		metrics.WorkersActive.Set(float64(p.activeCount.Load()))

		// Update queue depth (approximate)
		if qLen, err := p.redis.Unwrap().LLen(ctx, queueKey).Result(); err == nil {
			metrics.QueueDepth.Set(float64(qLen))
		}

		// Load session from Redis
		t, err := p.sessionService.Get(ctx, sessionID)
		if err != nil {
			log.Warn("failed to load session, skipping", "session_id", sessionID, "error", err)
			p.activeCount.Add(-1)
			continue
		}

		// Guard against stale/duplicate queue entries — only actionable states proceed
		if !shouldProcess(t.Status) {
			log.Warn("skipping stale queue entry", "session_id", sessionID, "status", t.Status)
			p.activeCount.Add(-1)
			metrics.WorkersActive.Set(float64(p.activeCount.Load()))
			continue
		}

		// Create session-specific cancellable context
		sessionCtx, sessionCancel := context.WithCancel(ctx)

		// Register cancel func for this session
		p.cancelsMu.Lock()
		p.cancels[sessionID] = sessionCancel
		p.cancelsMu.Unlock()

		// Execute the session
		p.executor.Execute(sessionCtx, t)

		// Deregister cancel func
		p.cancelsMu.Lock()
		delete(p.cancels, sessionID)
		p.cancelsMu.Unlock()
		sessionCancel() // clean up context resources

		p.activeCount.Add(-1)
		metrics.WorkersActive.Set(float64(p.activeCount.Load()))
	}
}
