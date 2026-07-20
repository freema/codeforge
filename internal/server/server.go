package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/freema/codeforge/api"
	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/database"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server/handlers"
	"github.com/freema/codeforge/internal/server/middleware"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tenant"
	"github.com/freema/codeforge/internal/tool/mcp"
	"github.com/freema/codeforge/internal/tool/runner"
	"github.com/freema/codeforge/internal/workflow"
	"github.com/freema/codeforge/internal/workspace"
)

// Server is the HTTP server.
type Server struct {
	httpServer *http.Server
	health     *handlers.HealthHandler
}

// New creates and configures the HTTP server with all routes and middleware.
func New(cfg *config.Config, redis *redisclient.Client, sqliteDB *database.DB, sessionService *session.Service, prService *session.PRService, canceller handlers.Canceller, keyRegistry keys.Registry, mcpRegistry mcp.Registry, workspaceMgr *workspace.Manager, workflowRegistry workflow.Registry, workflowConfigStore workflow.ConfigStore, cliRegistry *runner.Registry, cliConfigs map[string]handlers.CLIInfo, webhookReceiverHandler *handlers.WebhookReceiverHandler, tenantHandler *handlers.TenantHandler, tenantService *tenant.Service, scheduleHandler *handlers.ScheduleHandler, version string) *Server {
	r := chi.NewRouter()

	// Global middleware (timeout applied per-route-group, not globally, for SSE support)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.PrometheusMetrics)
	r.Use(chimw.Recoverer)

	// Rate limiter
	var rateLimitMw func(http.Handler) http.Handler
	if cfg.RateLimit.Enabled && cfg.RateLimit.SessionsPerMinute > 0 {
		rl := middleware.NewRateLimiter(redis, cfg.RateLimit.SessionsPerMinute, time.Minute)
		rateLimitMw = rl.Middleware()
	}

	// Health endpoints (no auth)
	healthHandler := handlers.NewHealthHandler(redis, sqliteDB, workspaceMgr, version)
	r.Get("/", healthHandler.Info)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// Prometheus metrics endpoint (no auth)
	r.Handle("/metrics", promhttp.Handler())

	// API docs (no auth)
	docsHandler := handlers.NewDocsHandler(api.OpenAPISpec)
	r.Get("/api/docs", docsHandler.SwaggerUI)
	r.Get("/api/docs/openapi.yaml", docsHandler.OpenAPISpec)

	// Webhook receiver endpoints (no Bearer auth — verified via webhook secrets)
	if webhookReceiverHandler != nil {
		r.Post("/api/v1/webhooks/github", webhookReceiverHandler.GitHubWebhook)
		r.Post("/api/v1/webhooks/gitlab", webhookReceiverHandler.GitLabWebhook)
	}

	// Handlers
	sessionHandler := handlers.NewSessionHandler(sessionService, prService, canceller, cliRegistry, keyRegistry, cfg.Git.ProviderDomains, tenantService)
	cliHandler := handlers.NewCLIHandler(cliRegistry, cliConfigs)
	streamHandler := handlers.NewStreamHandler(sessionService, redis)
	keyHandler := handlers.NewKeyHandler(keyRegistry)
	mcpHandler := handlers.NewMCPHandler(mcpRegistry)
	toolHandler := handlers.NewToolHandler()
	wsHandler := handlers.NewWorkspaceHandler(workspaceMgr, sessionService)
	repoHandler := handlers.NewRepoHandler(keyRegistry)
	sentryHandler := handlers.NewSentryHandler(keyRegistry)
	workflowHandler := handlers.NewWorkflowHandler(workflowRegistry, sessionService, keyRegistry)
	workflowConfigHandler := handlers.NewWorkflowConfigHandler(workflowConfigStore, workflowRegistry, sessionService, keyRegistry)

	// Protected API routes.
	// Dual-auth when the subscription model is enabled: operator token OR tenant
	// API token. Otherwise the original static operator-token auth (unchanged).
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.Subscription.Enabled && tenantService != nil {
			r.Use(middleware.TenantAuth(cfg.Server.AuthToken, tenantService.Store()))
		} else {
			r.Use(middleware.BearerAuth(cfg.Server.AuthToken))
		}

		// Auth verification endpoint
		r.Get("/auth/verify", healthHandler.AuthVerify)

		// Alias of the root /health for API clients (the UI reaches the server
		// only through the /api prefix, so the root endpoint is out of its reach)
		r.Get("/health", healthHandler.Health)

		// SSE stream endpoints — no timeout middleware (long-lived connection)
		r.With(sessionHandler.OwnershipMiddleware).Get("/sessions/{sessionID}/stream", streamHandler.Stream)

		// All other routes — with timeout
		r.Group(func(r chi.Router) {
			r.Use(chimw.Timeout(60 * time.Second))

			r.Route("/sessions", func(r chi.Router) {
				r.Use(sessionHandler.OwnershipMiddleware) // tenant may touch only its own {sessionID} routes
				r.Get("/", sessionHandler.List)
				if rateLimitMw != nil {
					r.With(rateLimitMw).Post("/", sessionHandler.Create)
				} else {
					r.Post("/", sessionHandler.Create)
				}
				r.Get("/{sessionID}", sessionHandler.Get)
				r.Post("/{sessionID}/instruct", sessionHandler.Instruct)
				r.Post("/{sessionID}/cancel", sessionHandler.Cancel)
				r.Post("/{sessionID}/review", sessionHandler.Review)
				r.Post("/{sessionID}/post-review", sessionHandler.PostReviewComments)
				r.Post("/{sessionID}/create-pr", sessionHandler.CreatePR)
				r.Post("/{sessionID}/push", sessionHandler.PushToPR)
				r.Get("/{sessionID}/pr-status", sessionHandler.GetPRStatus)
			})

			r.Get("/session-types", sessionHandler.ListSessionTypes)

			// Caller identity + self-service usage — available to both roles
			// (tenants get their own scope, operators are directed to /admin).
			if tenantHandler != nil {
				r.Get("/me", tenantHandler.Me)
				r.Get("/me/usage", tenantHandler.MeUsage)
			}

			r.Route("/cli", func(r chi.Router) {
				r.Get("/", cliHandler.List)
				r.Get("/health", cliHandler.Health)
			})

			// Operator-management subsystems — operator token only. Subscription
			// tenants must NOT manage keys, tools, MCP servers, workspaces, or
			// workflows. OperatorOnly is a no-op under plain BearerAuth (no tenant
			// in context), so operator access is unaffected when subscription is off.
			r.Group(func(r chi.Router) {
				r.Use(middleware.OperatorOnly)

				r.Route("/keys", func(r chi.Router) {
					r.Post("/", keyHandler.Create)
					r.Get("/", keyHandler.List)
					r.Get("/{name}/verify", keyHandler.Verify)
					r.Delete("/{name}", keyHandler.Delete)
				})

				r.Route("/mcp/servers", func(r chi.Router) {
					r.Post("/", mcpHandler.CreateGlobal)
					r.Get("/", mcpHandler.ListGlobal)
					r.Delete("/{name}", mcpHandler.DeleteGlobal)
				})

				r.Get("/tools/catalog", toolHandler.Catalog)

				r.Route("/workspaces", func(r chi.Router) {
					r.Get("/", wsHandler.List)
					r.Delete("/{sessionID}", wsHandler.Delete)
				})

				r.Get("/repositories", repoHandler.List)
				r.Get("/branches", repoHandler.ListBranches)
				r.Get("/pull-requests", repoHandler.ListPullRequests)

				r.Route("/sentry", func(r chi.Router) {
					r.Get("/organizations", sentryHandler.ListOrganizations)
					r.Get("/projects", sentryHandler.ListProjects)
					r.Get("/issues", sentryHandler.ListIssues)
					r.Get("/issues/{issueID}", sentryHandler.GetIssue)
					r.Get("/issues/{issueID}/latest-event", sentryHandler.GetLatestEvent)
				})

				r.Route("/workflows", func(r chi.Router) {
					r.Post("/", workflowHandler.CreateWorkflow)
					r.Get("/", workflowHandler.ListWorkflows)
					r.Get("/{name}", workflowHandler.GetWorkflow)
					r.Delete("/{name}", workflowHandler.DeleteWorkflow)
					r.Post("/{name}/run", workflowHandler.RunWorkflow)
				})

				r.Route("/workflow-configs", func(r chi.Router) {
					r.Post("/", workflowConfigHandler.Create)
					r.Get("/", workflowConfigHandler.List)
					r.Get("/{id}", workflowConfigHandler.Get)
					r.Delete("/{id}", workflowConfigHandler.Delete)
					r.Post("/{id}/run", workflowConfigHandler.Run)
				})

				if scheduleHandler != nil {
					r.Route("/schedules", func(r chi.Router) {
						r.Post("/", scheduleHandler.Create)
						r.Get("/", scheduleHandler.List)
						r.Get("/{scheduleID}", scheduleHandler.Get)
						r.Patch("/{scheduleID}", scheduleHandler.Update)
						r.Delete("/{scheduleID}", scheduleHandler.Delete)
						r.Post("/{scheduleID}/run", scheduleHandler.Run)
					})
				}
			})

			if tenantHandler != nil {
				// Admin routes are operator-only — tenant tokens are rejected.
				r.Route("/admin/tenants", func(r chi.Router) {
					r.Use(middleware.OperatorOnly)
					r.Post("/", tenantHandler.Create)
					r.Get("/", tenantHandler.List)
					r.Get("/{tenantID}", tenantHandler.Get)
					r.Patch("/{tenantID}", tenantHandler.Update)
					r.Delete("/{tenantID}", tenantHandler.Delete)
					r.Get("/{tenantID}/usage", tenantHandler.Usage)
				})

				r.Route("/admin/key-pool", func(r chi.Router) {
					r.Use(middleware.OperatorOnly)
					r.Post("/", tenantHandler.AddKeyPool)
					r.Get("/", tenantHandler.ListKeyPool)
					r.Delete("/{keyID}", tenantHandler.DeleteKeyPool)
				})
			}
		})
	})

	// Wrap with OpenTelemetry HTTP instrumentation.
	// SSE endpoints bypass otelhttp because its response writer wrapper
	// does not support http.Flusher, which breaks real-time streaming.
	otelHandler := otelhttp.NewHandler(r, "codeforge",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/health" && r.URL.Path != "/ready" && r.URL.Path != "/metrics"
		}),
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/stream") {
			r.ServeHTTP(w, req) // bypass otelhttp for SSE
			return
		}
		otelHandler.ServeHTTP(w, req)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disabled — SSE handler manages deadlines via ResponseController
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: srv,
		health:     healthHandler,
	}
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	slog.Info("http server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.health.SetReady(false)
	return s.httpServer.Shutdown(ctx)
}
