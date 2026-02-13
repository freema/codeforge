package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/mcp"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server/handlers"
	"github.com/freema/codeforge/internal/server/middleware"
	"github.com/freema/codeforge/internal/task"
	"github.com/freema/codeforge/internal/workspace"
)

// Server is the HTTP server.
type Server struct {
	httpServer *http.Server
	health     *handlers.HealthHandler
}

// New creates and configures the HTTP server with all routes and middleware.
func New(cfg *config.Config, redis *redisclient.Client, taskService *task.Service, prService *task.PRService, canceller handlers.Canceller, keyRegistry *keys.Registry, mcpRegistry *mcp.Registry, workspaceMgr *workspace.Manager, version string) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.PrometheusMetrics)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))

	// Rate limiter
	var rateLimitMw func(http.Handler) http.Handler
	if cfg.RateLimit.Enabled && cfg.RateLimit.TasksPerMinute > 0 {
		rl := middleware.NewRateLimiter(redis, cfg.RateLimit.TasksPerMinute, time.Minute)
		rateLimitMw = rl.Middleware()
	}

	// Health endpoints (no auth)
	healthHandler := handlers.NewHealthHandler(redis, workspaceMgr, version)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// Prometheus metrics endpoint (no auth)
	r.Handle("/metrics", promhttp.Handler())

	// Task handler
	taskHandler := handlers.NewTaskHandler(taskService, prService, canceller)

	// Key handler
	keyHandler := handlers.NewKeyHandler(keyRegistry)

	// MCP handler
	mcpHandler := handlers.NewMCPHandler(mcpRegistry)

	// Workspace handler
	wsHandler := handlers.NewWorkspaceHandler(workspaceMgr, taskService)

	// Protected API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.BearerAuth(cfg.Server.AuthToken))

		r.Route("/tasks", func(r chi.Router) {
			if rateLimitMw != nil {
				r.With(rateLimitMw).Post("/", taskHandler.Create)
			} else {
				r.Post("/", taskHandler.Create)
			}
			r.Get("/{taskID}", taskHandler.Get)
			r.Post("/{taskID}/instruct", taskHandler.Instruct)
			r.Post("/{taskID}/cancel", taskHandler.Cancel)
			r.Post("/{taskID}/create-pr", taskHandler.CreatePR)
		})

		r.Route("/keys", func(r chi.Router) {
			r.Post("/", keyHandler.Create)
			r.Get("/", keyHandler.List)
			r.Delete("/{name}", keyHandler.Delete)
		})

		r.Route("/mcp/servers", func(r chi.Router) {
			r.Post("/", mcpHandler.CreateGlobal)
			r.Get("/", mcpHandler.ListGlobal)
			r.Delete("/{name}", mcpHandler.DeleteGlobal)
		})

		r.Route("/workspaces", func(r chi.Router) {
			r.Get("/", wsHandler.List)
			r.Delete("/{taskID}", wsHandler.Delete)
		})
	})

	// Wrap with OpenTelemetry HTTP instrumentation
	handler := otelhttp.NewHandler(r, "codeforge",
		otelhttp.WithFilter(func(r *http.Request) bool {
			// Skip tracing for health/ready/metrics endpoints
			return r.URL.Path != "/health" && r.URL.Path != "/ready" && r.URL.Path != "/metrics"
		}),
	)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
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

func notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}
