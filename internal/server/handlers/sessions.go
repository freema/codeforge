package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/prompt"
	"github.com/freema/codeforge/internal/review"
	"github.com/freema/codeforge/internal/server/middleware"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tenant"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tool/runner"
)

var validate = validator.New()

// Canceller can cancel a running session.
type Canceller interface {
	Cancel(sessionID string) error
}

// tenantSessionCounter counts a tenant's in-flight sessions for concurrency limits.
// Implemented by *session.Service; kept as an interface so handler tests can fake it.
type tenantSessionCounter interface {
	CountActiveByTenant(ctx context.Context, tenantID string) (int, error)
}

// SessionHandler handles session-related HTTP endpoints.
type SessionHandler struct {
	service         *session.Service
	prService       *session.PRService
	canceller       Canceller
	cliRegistry     *runner.Registry
	keyRegistry     keys.Registry
	providerDomains map[string]string
	tenantService   *tenant.Service      // optional, nil = subscription disabled
	sessionCounter  tenantSessionCounter // optional, nil = concurrency limit not enforced
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(service *session.Service, prService *session.PRService, canceller Canceller, cliRegistry *runner.Registry, keyRegistry keys.Registry, providerDomains map[string]string, tenantService *tenant.Service) *SessionHandler {
	h := &SessionHandler{service: service, prService: prService, canceller: canceller, cliRegistry: cliRegistry, keyRegistry: keyRegistry, providerDomains: providerDomains, tenantService: tenantService}
	if service != nil {
		h.sessionCounter = service
	}
	return h
}

// List handles GET /api/v1/sessions.
// Supports optional ?status= filter and ?limit=&offset= pagination.
func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	opts := session.ListOptions{
		Status: r.URL.Query().Get("status"),
	}
	// Subscription tenants see only their own sessions.
	if tnt := middleware.TenantFromContext(r.Context()); tnt != nil {
		opts.TenantID = tnt.ID
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Offset = n
		}
	}

	sessions, total, err := h.service.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"total":    total,
	})
}

// Create handles POST /api/v1/sessions.
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req session.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validate.Struct(req); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			fields := make(map[string]string)
			for _, e := range validationErrs {
				fields[e.Field()] = formatValidationError(e)
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": fields,
			})
			return
		}
		writeError(w, http.StatusBadRequest, "validation failed")
		return
	}

	// Validate session_type
	if req.SessionType != "" {
		if !prompt.ValidSessionType(req.SessionType) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": map[string]string{"session_type": fmt.Sprintf("unknown session type: %s", req.SessionType)},
			})
			return
		}
	}

	// pr_review sessions require pr_number in config
	if req.SessionType == "pr_review" {
		if req.Config == nil || req.Config.PRNumber <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": map[string]string{"pr_number": "pr_number is required for pr_review sessions"},
			})
			return
		}
	}

	// Validate CLI name against registry
	if req.Config != nil && req.Config.CLI != "" {
		if _, err := h.cliRegistry.Get(req.Config.CLI); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": map[string]string{"cli": fmt.Sprintf("unknown CLI: %s", req.Config.CLI)},
			})
			return
		}
	}

	// Subscription tenants: enforce tier limits + assign a managed key from the pool.
	// req.TenantID is json:"-" so it can only be set server-side by applyTenant.
	if tnt := middleware.TenantFromContext(r.Context()); tnt != nil {
		if status, msg := h.applyTenant(r.Context(), &req, tnt); status != 0 {
			writeError(w, status, msg)
			return
		}
	}

	t, err := h.service.Create(r.Context(), req)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         t.ID,
		"status":     t.Status,
		"created_at": t.CreatedAt,
	})
}

// OwnershipMiddleware enforces tenant ownership of a session for any route with a
// {sessionID} URL param. When the request is authenticated as a subscription
// tenant, the target session must belong to that tenant (session metadata
// tenant_id); operator/no-tenant requests pass unconditionally. A mismatch returns
// 404 (not 403) so a tenant cannot probe other tenants' session IDs. Routes without
// a sessionID (List, Create) pass through and enforce their own scoping.
func (h *SessionHandler) OwnershipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tnt := middleware.TenantFromContext(r.Context())
		sessionID := chi.URLParam(r, "sessionID")
		if tnt == nil || sessionID == "" {
			next.ServeHTTP(w, r)
			return
		}
		t, err := h.service.Get(r.Context(), sessionID)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if t.TenantID != tnt.ID {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// applyTenant enforces a subscription tenant's tier limits and assigns a managed
// API key from the operator pool when the request brings no BYOK key. Returns a
// non-zero HTTP status + message on rejection, or (0, "") to proceed.
func (h *SessionHandler) applyTenant(ctx context.Context, req *session.CreateSessionRequest, tnt *tenant.Tenant) (int, string) {
	if h.tenantService == nil {
		return 0, ""
	}

	cli := h.cliRegistry.DefaultCLI()
	if req.Config != nil && req.Config.CLI != "" {
		cli = req.Config.CLI
	}

	// Tier: allowed CLIs
	if !stringInJSONList(tnt.AllowedCLIs, cli) {
		return http.StatusForbidden, fmt.Sprintf("CLI %q is not allowed for the %q subscription tier", cli, tnt.Tier)
	}

	// Tier: allowed models (only enforced when the tenant restricts models)
	if tnt.AllowedModels != nil && req.Config != nil && req.Config.AIModel != "" {
		if !stringInJSONList(*tnt.AllowedModels, req.Config.AIModel) {
			return http.StatusForbidden, fmt.Sprintf("model %q is not allowed for the %q subscription tier", req.Config.AIModel, tnt.Tier)
		}
	}

	// Tier: daily session quota (-1 = unlimited). Fail closed on a store error so a
	// transient DB problem cannot silently waive the quota.
	if tnt.MaxSessionsPerDay >= 0 {
		used, err := h.tenantService.Store().CountDailySessions(ctx, tnt.ID)
		if err != nil {
			return http.StatusServiceUnavailable, "could not verify session quota, try again"
		}
		if used >= tnt.MaxSessionsPerDay {
			return http.StatusTooManyRequests, fmt.Sprintf("daily session limit reached (%d/%d) for the %q tier", used, tnt.MaxSessionsPerDay, tnt.Tier)
		}
	}

	// Tier: concurrent in-flight sessions. Also bounds the daily-quota burst window,
	// since at most MaxConcurrentSessions sessions can run before completing (and
	// completed ones count toward the daily limit).
	if h.sessionCounter != nil && tnt.MaxConcurrentSessions > 0 {
		active, err := h.sessionCounter.CountActiveByTenant(ctx, tnt.ID)
		if err != nil {
			return http.StatusServiceUnavailable, "could not verify concurrency limit, try again"
		}
		if active >= tnt.MaxConcurrentSessions {
			return http.StatusTooManyRequests, fmt.Sprintf("concurrent session limit reached (%d/%d) for the %q tier", active, tnt.MaxConcurrentSessions, tnt.Tier)
		}
	}

	// Record the owning tenant on a server-side field (never client-trusted) for
	// ownership scoping and usage attribution.
	req.TenantID = tnt.ID

	// Per-session budget cap from the tier (unless the caller set a tighter one).
	if tnt.MaxBudgetUSDPerSession > 0 {
		if req.Config == nil {
			req.Config = &session.Config{}
		}
		if req.Config.MaxBudgetUSD == 0 || req.Config.MaxBudgetUSD > tnt.MaxBudgetUSDPerSession {
			req.Config.MaxBudgetUSD = tnt.MaxBudgetUSDPerSession
		}
	}

	// Assign a managed key from the pool when the tenant did not bring its own (BYOK).
	if req.Config == nil || req.Config.AIApiKey == "" {
		provider := "anthropic"
		if _, meta, err := h.cliRegistry.GetWithMeta(cli); err == nil && meta.AIProvider != "" {
			provider = meta.AIProvider
		}
		key, err := h.tenantService.ResolveKeyFromPool(ctx, provider)
		if err != nil || key == "" {
			return http.StatusServiceUnavailable, fmt.Sprintf("no managed API key available for provider %q — contact the operator", provider)
		}
		if req.Config == nil {
			req.Config = &session.Config{}
		}
		req.Config.AIApiKey = key
	}

	return 0, ""
}

// stringInJSONList reports whether target is allowed by a JSON array allow-list like
// `["claude-code","codex"]`. An empty/whitespace list means "no restriction" (allow).
// A NON-empty but malformed list fails CLOSED (deny) — a corrupt restriction must not
// silently grant access to everything.
func stringInJSONList(list, target string) bool {
	if strings.TrimSpace(list) == "" {
		return true // no restriction configured
	}
	var items []string
	if err := json.Unmarshal([]byte(list), &items); err != nil {
		return false // malformed restriction → deny (fail closed)
	}
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// Get handles GET /api/v1/sessions/{sessionID}.
// Supports ?include=iterations to load full iteration history.
func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	t, err := h.service.Get(r.Context(), sessionID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	// Load iterations if requested
	if r.URL.Query().Get("include") == "iterations" {
		iterations, err := h.service.GetIterations(r.Context(), sessionID)
		if err == nil {
			t.Iterations = iterations
		}
	}

	writeJSON(w, http.StatusOK, t)
}

// Instruct handles POST /api/v1/sessions/{sessionID}/instruct.
func (h *SessionHandler) Instruct(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	var req struct {
		Prompt string `json:"prompt" validate:"required,max=102400"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "prompt is required and must be under 100KB")
		return
	}

	t, err := h.service.Instruct(r.Context(), sessionID, req.Prompt)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        t.ID,
		"status":    t.Status,
		"iteration": t.Iteration,
	})
}

// Cancel handles POST /api/v1/sessions/{sessionID}/cancel.
func (h *SessionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	// Load session to check status
	t, err := h.service.Get(r.Context(), sessionID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	// Queued but not yet picked up — cancel directly; the stale queue entry
	// is skipped by the worker's shouldProcess guard.
	if t.Status == session.StatusPending {
		if err := h.service.UpdateStatus(r.Context(), sessionID, session.StatusCanceled); err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"id":      sessionID,
			"status":  string(session.StatusCanceled),
			"message": "queued session canceled",
		})
		return
	}

	if t.Status != session.StatusRunning && t.Status != session.StatusCloning && t.Status != session.StatusReviewing {
		writeError(w, http.StatusConflict, fmt.Sprintf("session is not running (status: %s)", t.Status))
		return
	}

	if err := h.canceller.Cancel(sessionID); err != nil {
		writeError(w, http.StatusConflict, "session is not currently running")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      sessionID,
		"status":  "canceling",
		"message": "session cancellation requested",
	})
}

// CreatePR handles POST /api/v1/sessions/{sessionID}/create-pr.
func (h *SessionHandler) CreatePR(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	var req session.CreatePRRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	result, err := h.prService.CreatePR(r.Context(), sessionID, req)
	if err != nil {
		// Determine status code from error message
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			writeError(w, http.StatusNotFound, errMsg)
		case strings.Contains(errMsg, "must be in completed or pr_created status"):
			writeError(w, http.StatusConflict, errMsg)
		case strings.Contains(errMsg, "no changes"), strings.Contains(errMsg, "nothing to commit"):
			writeError(w, http.StatusBadRequest, "No new changes to create PR for. Run another instruction first.")
		case strings.Contains(errMsg, "no changes to create PR"):
			writeError(w, http.StatusBadRequest, errMsg)
		case strings.Contains(errMsg, "not supported"):
			writeError(w, http.StatusBadRequest, errMsg)
		default:
			writeError(w, http.StatusInternalServerError, errMsg)
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// PushToPR handles POST /api/v1/sessions/{sessionID}/push.
func (h *SessionHandler) PushToPR(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	result, err := h.prService.PushToPR(r.Context(), sessionID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			writeError(w, http.StatusNotFound, errMsg)
		case strings.Contains(errMsg, "must be in completed or pr_created status"):
			writeError(w, http.StatusConflict, errMsg)
		case strings.Contains(errMsg, "no new changes to push"):
			writeError(w, http.StatusBadRequest, errMsg)
		case strings.Contains(errMsg, "no existing PR"):
			writeError(w, http.StatusBadRequest, errMsg)
		default:
			writeError(w, http.StatusInternalServerError, errMsg)
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Review handles POST /api/v1/sessions/{sessionID}/review.
func (h *SessionHandler) Review(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	var req struct {
		CLI   string `json:"cli,omitempty"`
		Model string `json:"model,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	// Validate CLI name if provided
	if req.CLI != "" {
		if _, err := h.cliRegistry.Get(req.CLI); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  "validation_error",
				"fields": map[string]string{"cli": fmt.Sprintf("unknown CLI: %s", req.CLI)},
			})
			return
		}
	}

	t, err := h.service.StartReviewAsync(r.Context(), sessionID, req.CLI, req.Model)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"id":     t.ID,
		"status": t.Status,
	})
}

// PostReviewComments handles POST /api/v1/sessions/{sessionID}/post-review.
// Posts the session's ReviewResult as comments to the associated PR/MR.
func (h *SessionHandler) PostReviewComments(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	var req struct {
		PRNumber int `json:"pr_number,omitempty"` // override; defaults to session config
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	t, err := h.service.Get(r.Context(), sessionID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	if t.ReviewResult == nil {
		writeError(w, http.StatusBadRequest, "session has no review result — run a review first")
		return
	}

	// Resolve PR number
	prNumber := req.PRNumber
	if prNumber <= 0 && t.Config != nil {
		prNumber = t.Config.PRNumber
	}
	if prNumber <= 0 {
		prNumber = t.PRNumber
	}
	if prNumber <= 0 {
		writeError(w, http.StatusBadRequest, "pr_number is required (not set on session and not provided in request)")
		return
	}

	// Resolve token
	token := t.AccessToken
	if token == "" && t.ProviderKey != "" && h.keyRegistry != nil {
		resolved, _, resolveErr := h.keyRegistry.ResolveByName(r.Context(), t.ProviderKey)
		if resolveErr != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to resolve provider key: %v", resolveErr))
			return
		}
		token = resolved
	}
	if token == "" {
		writeError(w, http.StatusBadRequest, "no access token available — set provider_key on session")
		return
	}

	repo, err := gitpkg.ParseRepoURL(t.RepoURL, h.providerDomains)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse repo URL: %v", err))
		return
	}

	result, err := gitpkg.PostReviewComments(
		r.Context(), repo, token, prNumber, t.ReviewResult,
		review.FormatSummaryBody, review.FormatIssueComment,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to post review comments: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"review_url":      result.ReviewURL,
		"comments_posted": result.CommentsPosted,
		"pr_number":       prNumber,
	})
}

// GetPRStatus handles GET /api/v1/sessions/{sessionID}/pr-status.
func (h *SessionHandler) GetPRStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	status, err := h.prService.GetPRStatus(r.Context(), sessionID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			writeError(w, http.StatusNotFound, errMsg)
		} else if strings.Contains(errMsg, "has no PR") {
			writeError(w, http.StatusBadRequest, errMsg)
		} else {
			writeError(w, http.StatusBadGateway, errMsg)
		}
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ListSessionTypes handles GET /api/v1/session-types.
func (h *SessionHandler) ListSessionTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_types": prompt.SessionTypes(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

func writeAppError(w http.ResponseWriter, err error) {
	status := apperror.HTTPStatus(err)
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, status, map[string]interface{}{
			"error":   http.StatusText(status),
			"message": appErr.Message,
			"fields":  appErr.Fields,
		})
		return
	}
	writeError(w, status, "internal server error")
}

func formatValidationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "field is required"
	case "url":
		return "must be a valid URL"
	case "max":
		return "exceeds maximum length"
	default:
		return "invalid value"
	}
}
