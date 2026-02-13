package handlers

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
)

// HealthHandler serves /health and /ready endpoints.
type HealthHandler struct {
	redis     *redisclient.Client
	startTime time.Time
	version   string
	ready     *atomic.Bool
}

// NewHealthHandler creates a health handler.
func NewHealthHandler(redis *redisclient.Client, version string) *HealthHandler {
	ready := &atomic.Bool{}
	ready.Store(true)
	return &HealthHandler{
		redis:     redis,
		startTime: time.Now(),
		version:   version,
		ready:     ready,
	}
}

// SetReady sets the readiness state (false during shutdown).
func (h *HealthHandler) SetReady(v bool) {
	h.ready.Store(v)
}

type healthResponse struct {
	Status  string `json:"status"`
	Redis   string `json:"redis"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// Health checks Redis connectivity and returns system health.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status:  "ok",
		Redis:   "connected",
		Version: h.version,
		Uptime:  time.Since(h.startTime).Round(time.Second).String(),
	}
	statusCode := http.StatusOK

	if err := h.redis.Ping(r.Context()); err != nil {
		resp.Status = "error"
		resp.Redis = "disconnected"
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// Ready returns 200 if the server is accepting traffic, 503 during shutdown.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if !h.ready.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "shutting_down"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
