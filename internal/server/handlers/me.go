package handlers

import (
	"net/http"
	"time"

	"github.com/freema/codeforge/internal/server/middleware"
)

// Me returns the caller's identity: operator, or tenant with its profile/limits.
// Available to both roles so the UI can adapt its navigation.
func (h *TenantHandler) Me(w http.ResponseWriter, r *http.Request) {
	t := middleware.TenantFromContext(r.Context())
	if t == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"role": "operator"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"role": "tenant", "tenant": t})
}

// MeUsage returns the authenticated tenant's own usage summary, today's session
// count, and effective limits. Operators have no tenant scope → 404 (they read
// per-tenant usage via the admin API instead).
func (h *TenantHandler) MeUsage(w http.ResponseWriter, r *http.Request) {
	t := middleware.TenantFromContext(r.Context())
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "usage is available only for tenant tokens"})
		return
	}

	period := r.URL.Query().Get("period")
	var since time.Time
	switch period {
	case "30d":
		since = time.Now().AddDate(0, 0, -30)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	default:
		period = "7d"
		since = time.Now().AddDate(0, 0, -7)
	}

	summary, err := h.service.Store().GetUsageSummary(r.Context(), t.ID, since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sessionsToday, err := h.service.Store().CountDailySessions(r.Context(), t.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period":         period,
		"sessions_today": sessionsToday,
		"summary":        summary,
		"limits": map[string]interface{}{
			"tier":                       t.Tier,
			"max_sessions_per_day":       t.MaxSessionsPerDay,
			"max_concurrent_sessions":    t.MaxConcurrentSessions,
			"max_budget_usd_per_session": t.MaxBudgetUSDPerSession,
			"allowed_clis":               t.AllowedCLIs,
			"allowed_models":             t.AllowedModels,
		},
	})
}
