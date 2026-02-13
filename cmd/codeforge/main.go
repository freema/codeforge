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

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/logger"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server"
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
	redis, err := redisclient.New(cfg.Redis.URL, cfg.Redis.Prefix)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer redis.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redis.Ping(ctx); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	slog.Info("redis connected", "url", cfg.Redis.URL)

	// Create and start HTTP server
	srv := server.New(cfg, redis, version)

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
		return fmt.Errorf("server shutdown: %w", err)
	}

	// TODO: stop worker pool, stop redis listener

	redis.Close()
	slog.Info("shutdown complete")
	return nil
}
