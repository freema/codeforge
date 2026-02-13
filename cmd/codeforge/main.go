package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/freema/codeforge/internal/cli"
	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/logger"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server"
	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/webhook"
	"github.com/freema/codeforge/internal/worker"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("codeforge", version)
		return
	}

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Load config
	configPath := os.Getenv("CODEFORGE_CONFIG")
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Setup logger
	logger.Setup(cfg.Logging.Level, cfg.Logging.Format)
	slog.Info("starting codeforge", "version", version)

	// Connect to Redis
	rdb, err := redisclient.New(cfg.Redis.URL, cfg.Redis.Prefix)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	slog.Info("redis connected", "url", cfg.Redis.URL)

	// Initialize crypto service
	cryptoSvc, err := crypto.NewService(cfg.Encryption.Key)
	if err != nil {
		return fmt.Errorf("initializing crypto: %w", err)
	}

	// Initialize task service
	taskService := task.NewService(
		rdb,
		cryptoSvc,
		cfg.Workers.QueueName,
		time.Duration(cfg.Tasks.StateTTL)*time.Second,
		time.Duration(cfg.Tasks.ResultTTL)*time.Second,
	)

	// Initialize webhook sender
	var webhookSender *webhook.Sender
	if cfg.Webhooks.HMACSecret != "" {
		webhookSender = webhook.NewSender(
			cfg.Webhooks.HMACSecret,
			cfg.Webhooks.RetryCount,
			cfg.Webhooks.RetryDelay,
		)
	}

	// Initialize CLI runner
	runner := cli.NewClaudeRunner(cfg.CLI.ClaudeCode.Path)

	// Initialize streamer
	streamer := worker.NewStreamer(rdb, time.Duration(cfg.Tasks.WorkspaceTTL)*time.Second)

	// Initialize executor
	executor := worker.NewExecutor(
		taskService,
		runner,
		streamer,
		webhookSender,
		worker.ExecutorConfig{
			WorkspaceBase:  cfg.Tasks.WorkspaceBase,
			DefaultTimeout: cfg.Tasks.DefaultTimeout,
			MaxTimeout:     cfg.Tasks.MaxTimeout,
			DefaultModel:   cfg.CLI.ClaudeCode.DefaultModel,
		},
	)

	// Initialize worker pool
	pool := worker.NewPool(
		rdb,
		executor,
		taskService,
		cfg.Workers.QueueName,
		cfg.Workers.Concurrency,
	)

	// Initialize Redis input listener
	listener := task.NewListener(rdb, taskService, "input:tasks")

	// Create and start HTTP server
	srv := server.New(cfg, rdb, taskService, version)

	// Start background services
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	pool.Start(appCtx)
	go listener.Start(appCtx)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	slog.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	appCancel() // Signal workers and listener to stop
	pool.Stop() // Wait for workers to drain

	rdb.Close()
	slog.Info("shutdown complete")
	return nil
}
