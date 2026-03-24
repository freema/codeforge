package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/freema/codeforge/internal/ai"
	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/logger"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server"
	"github.com/freema/codeforge/internal/server/handlers"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tool/mcp"
	"github.com/freema/codeforge/internal/tool/runner"
	"github.com/freema/codeforge/internal/tools"
	"github.com/freema/codeforge/internal/tracing"
	"github.com/freema/codeforge/internal/webhook"
	"github.com/freema/codeforge/internal/worker"
	"github.com/freema/codeforge/internal/workflow"
	"github.com/freema/codeforge/internal/workspace"
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

	// Initialize tracing
	tracingShutdown, err := tracing.Setup(context.Background(), tracing.Config{
		Enabled:      cfg.Tracing.Enabled,
		Endpoint:     cfg.Tracing.Endpoint,
		SamplingRate: cfg.Tracing.SamplingRate,
		ServiceName:  "codeforge",
		Version:      version,
	})
	if err != nil {
		return fmt.Errorf("initializing tracing: %w", err)
	}
	defer func() { _ = tracingShutdown(context.Background()) }()

	// Connect to Redis
	rdb, err := redisclient.New(cfg.Redis.URL, cfg.Redis.Prefix)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	slog.Info("redis connected", "url", cfg.Redis.URL)

	// Open SQLite database
	sqliteDB, err := database.Open(cfg.SQLite.Path)
	if err != nil {
		return fmt.Errorf("opening sqlite: %w", err)
	}
	defer func() { _ = sqliteDB.Close() }()

	if err := database.Migrate(context.Background(), sqliteDB.Unwrap()); err != nil {
		return fmt.Errorf("running sqlite migrations: %w", err)
	}
	slog.Info("sqlite connected", "path", cfg.SQLite.Path)

	// Initialize crypto service
	cryptoSvc, err := crypto.NewService(cfg.Encryption.Key)
	if err != nil {
		return fmt.Errorf("initializing crypto: %w", err)
	}

	// Initialize session service
	sessionService := session.NewService(
		rdb,
		cryptoSvc,
		sqliteDB.Unwrap(),
		cfg.Workers.QueueName,
		time.Duration(cfg.Sessions.StateTTL)*time.Second,
		time.Duration(cfg.Sessions.ResultTTL)*time.Second,
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

	// Auto-populate provider domains from GITLAB_URL / GITHUB_URL env vars
	// so self-hosted instances are recognized for PR creation without manual config.
	cfg.Git.ProviderDomains = keys.MergeEnvProviderDomains(cfg.Git.ProviderDomains)

	// Initialize key registry and resolver
	sqliteKeyRegistry := keys.NewSQLiteRegistry(sqliteDB.Unwrap(), cryptoSvc)
	keyRegistry := keys.NewEnvAwareRegistry(sqliteKeyRegistry)
	keyResolver := keys.NewResolver(keyRegistry, cfg.Git.ProviderDomains)

	// Initialize MCP registry and installer
	mcpRegistry := mcp.NewSQLiteRegistry(sqliteDB.Unwrap())
	mcpInstaller := mcp.NewInstaller(mcpRegistry)

	// Initialize tool registry and resolver
	toolRegistry := tools.NewSQLiteRegistry(sqliteDB.Unwrap())
	if err := tools.SeedBuiltins(context.Background(), toolRegistry); err != nil {
		return fmt.Errorf("seeding builtin tools: %w", err)
	}
	toolResolver := tools.NewResolver(toolRegistry, keyRegistry)

	// Initialize workspace manager
	workspaceMgr := workspace.NewManager(
		cfg.Sessions.WorkspaceBase,
		rdb,
		time.Duration(cfg.Sessions.WorkspaceTTL)*time.Second,
	)

	// Initialize CLI registry
	cliRegistry := runner.NewRegistry(cfg.CLI.Default)
	cliRegistry.Register("claude-code", runner.NewClaudeRunner(cfg.CLI.ClaudeCode.Path))
	cliRegistry.Register("codex", runner.NewCodexRunner(cfg.CLI.Codex.Path))

	// Log availability of registered CLI runners
	for _, name := range []string{cfg.CLI.ClaudeCode.Path, cfg.CLI.Codex.Path} {
		if _, err := exec.LookPath(name); err != nil {
			slog.Warn("CLI runner not found on PATH — sessions using this CLI will fail", "cli", name)
		}
	}

	// Build CLI info map for HTTP handler
	cliConfigs := map[string]handlers.CLIInfo{
		"claude-code": {Name: "claude-code", BinaryPath: cfg.CLI.ClaudeCode.Path, DefaultModel: cfg.CLI.ClaudeCode.DefaultModel, Models: cfg.CLI.ClaudeCode.Models},
		"codex":       {Name: "codex", BinaryPath: cfg.CLI.Codex.Path, DefaultModel: cfg.CLI.Codex.DefaultModel, Models: cfg.CLI.Codex.Models},
	}

	// Initialize streamer
	streamer := worker.NewStreamer(rdb, time.Duration(cfg.Sessions.WorkspaceTTL)*time.Second)

	// Initialize executor
	executor := worker.NewExecutor(
		sessionService,
		cliRegistry,
		streamer,
		webhookSender,
		keyResolver,
		mcpInstaller,
		toolResolver,
		workspaceMgr,
		worker.ExecutorConfig{
			WorkspaceBase:   cfg.Sessions.WorkspaceBase,
			DefaultTimeout:  cfg.Sessions.DefaultTimeout,
			MaxTimeout:      cfg.Sessions.MaxTimeout,
			ProviderDomains: cfg.Git.ProviderDomains,
			DefaultModels: map[string]string{
				"claude-code": cfg.CLI.ClaudeCode.DefaultModel,
				"codex":       cfg.CLI.Codex.DefaultModel,
			},
		},
	)

	// Initialize worker pool
	pool := worker.NewPool(
		rdb,
		executor,
		sessionService,
		cfg.Workers.QueueName,
		cfg.Workers.Concurrency,
	)

	// Initialize AI helper client (for PR metadata, commit messages)
	aiClient := ai.NewClientFromRegistry(context.Background(), keyResolver)

	// Initialize prompt analyzer
	analyzer := runner.NewAnalyzer(aiClient)

	// Initialize PR service
	prService := session.NewPRService(sessionService, analyzer, workspaceMgr, keyResolver, session.PRServiceConfig{
		WorkspaceBase:   cfg.Sessions.WorkspaceBase,
		BranchPrefix:    cfg.Git.BranchPrefix,
		CommitAuthor:    cfg.Git.CommitAuthor,
		CommitEmail:     cfg.Git.CommitEmail,
		ProviderDomains: cfg.Git.ProviderDomains,
	}, aiClient)

	// Initialize Redis input listener
	listener := session.NewListener(rdb, sessionService, "input:sessions")

	// Initialize workspace cleaner
	wsCleaner := workspace.NewCleaner(workspaceMgr, sessionService, workspace.CleanerConfig{
		Interval:              10 * time.Minute,
		DiskWarningThreshold:  int64(cfg.Sessions.DiskWarningThresholdGB) * 1024 * 1024 * 1024,
		DiskCriticalThreshold: int64(cfg.Sessions.DiskCriticalThresholdGB) * 1024 * 1024 * 1024,
	})

	// Initialize workflow subsystem
	workflowRegistry := workflow.NewSQLiteRegistry(sqliteDB.Unwrap())
	workflowRunStore := workflow.NewSQLiteRunStore(sqliteDB.Unwrap())
	workflowConfigStore := workflow.NewSQLiteConfigStore(sqliteDB.Unwrap())

	if err := workflow.SeedBuiltins(context.Background(), workflowRegistry); err != nil {
		return fmt.Errorf("seeding builtin workflows: %w", err)
	}

	wfStreamer := workflow.NewStreamer(rdb, time.Duration(cfg.Workflow.ContextTTLHours)*time.Hour)
	fetchExecutor := workflow.NewFetchExecutor(keyRegistry)
	sessionExecutor := workflow.NewSessionExecutor(sessionService, rdb, keyRegistry)
	actionExecutor := workflow.NewActionExecutor(prService)

	orchestrator := workflow.NewOrchestrator(
		workflowRegistry,
		workflowRunStore,
		fetchExecutor,
		sessionExecutor,
		actionExecutor,
		wfStreamer,
		rdb,
		workflow.OrchestratorConfig{
			ContextTTLHours:   cfg.Workflow.ContextTTLHours,
			MaxRunDurationSec: cfg.Workflow.MaxRunDurationSec,
		},
	)

	// Initialize webhook receiver handler for PR review
	var webhookReceiverHandler *handlers.WebhookReceiverHandler
	if cfg.CodeReview.WebhookSecrets.GitHub != "" || cfg.CodeReview.WebhookSecrets.GitLab != "" {
		webhookReceiverHandler = handlers.NewWebhookReceiverHandler(sessionService, rdb, cfg.CodeReview)
	}

	srv := server.New(cfg, rdb, sqliteDB, sessionService, prService, pool, keyRegistry, mcpRegistry, toolRegistry, workspaceMgr, workflowRegistry, workflowRunStore, orchestrator, orchestrator, workflowConfigStore, cliRegistry, cliConfigs, webhookReceiverHandler, version)

	// Start background services
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	pool.Start(appCtx)
	go listener.Start(appCtx)
	go wsCleaner.Start(appCtx)
	go orchestrator.Start(appCtx)

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

	_ = rdb.Close()
	slog.Info("shutdown complete")
	return nil
}
