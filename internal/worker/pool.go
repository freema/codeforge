package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/task"
)

// Pool is a worker pool that consumes tasks from a Redis queue.
type Pool struct {
	redis       *redisclient.Client
	executor    *Executor
	taskService *task.Service
	queueName   string
	concurrency int
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	activeCount atomic.Int32
}

// NewPool creates a new worker pool.
func NewPool(
	redis *redisclient.Client,
	executor *Executor,
	taskService *task.Service,
	queueName string,
	concurrency int,
) *Pool {
	return &Pool{
		redis:       redis,
		executor:    executor,
		taskService: taskService,
		queueName:   queueName,
		concurrency: concurrency,
	}
}

// Start launches all worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	slog.Info("starting worker pool", "concurrency", p.concurrency, "queue", p.queueName)

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

// ActiveCount returns the number of currently active workers.
func (p *Pool) ActiveCount() int32 {
	return p.activeCount.Load()
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

		taskID := result[1] // result[0] = key name

		log.Info("picked up task", "task_id", taskID)
		p.activeCount.Add(1)

		// Load task from Redis
		t, err := p.taskService.Get(ctx, taskID)
		if err != nil {
			log.Warn("failed to load task, skipping", "task_id", taskID, "error", err)
			p.activeCount.Add(-1)
			continue
		}

		// Execute the task
		p.executor.Execute(ctx, t)
		p.activeCount.Add(-1)
	}
}
