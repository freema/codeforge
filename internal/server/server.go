package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/server/handlers"
	"github.com/freema/codeforge/internal/server/middleware"
)

// Server is the HTTP server.
type Server struct {
	httpServer *http.Server
	health     *handlers.HealthHandler
}

// New creates and configures the HTTP server with all routes and middleware.
func New(cfg *config.Config, redis *redisclient.Client, version string) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))

	// Health endpoints (no auth)
	healthHandler := handlers.NewHealthHandler(redis, version)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// Protected API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.BearerAuth(cfg.Server.AuthToken))

		r.Route("/tasks", func(r chi.Router) {
			// TODO: Phase 1 handlers
			r.Post("/", notImplemented)
			r.Get("/{taskID}", notImplemented)
			r.Post("/{taskID}/instruct", notImplemented)
			r.Post("/{taskID}/cancel", notImplemented)
			r.Post("/{taskID}/create-pr", notImplemented)
		})

		r.Route("/keys", func(r chi.Router) {
			// TODO: Phase 4 handlers
			r.Post("/", notImplemented)
			r.Get("/", notImplemented)
			r.Delete("/{name}", notImplemented)
		})

		r.Route("/mcp/servers", func(r chi.Router) {
			// TODO: Phase 4 handlers
			r.Post("/", notImplemented)
			r.Get("/", notImplemented)
			r.Delete("/{name}", notImplemented)
		})
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
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
