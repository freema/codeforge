package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/metrics"
)

// PrometheusMetrics records HTTP request metrics.
func PrometheusMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()

		// Use chi route pattern for path label (avoids cardinality explosion)
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = "unknown"
		}

		metrics.HTTPRequests.WithLabelValues(r.Method, routePattern, strconv.Itoa(ww.statusCode)).Inc()
		metrics.HTTPDuration.WithLabelValues(r.Method, routePattern).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
