package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/crypto"
	"github.com/freema/codeforge/internal/tenant"
)

// TenantHandler handles tenant admin HTTP endpoints.
type TenantHandler struct {
	service   *tenant.Service
	cryptoSvc *crypto.Service
}

// NewTenantHandler creates a new tenant handler.
func NewTenantHandler(service *tenant.Service, cryptoSvc *crypto.Service) *TenantHandler {
	return &TenantHandler{service: service, cryptoSvc: cryptoSvc}
}

// Create creates a new tenant and returns the API token (shown once).
func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name" validate:"required"`
		Slug string `json:"slug" validate:"required"`
		Tier string `json:"tier" validate:"required,oneof=free pro enterprise"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Struct(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := h.service.CreateTenant(r.Context(), req.Name, req.Slug, req.Tier)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// List returns all tenants.
func (h *TenantHandler) List(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.service.Store().ListTenants(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tenants == nil {
		tenants = []*tenant.Tenant{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tenants": tenants})
}

// Get returns a single tenant by ID.
func (h *TenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	t, err := h.service.Store().GetTenant(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Update modifies a tenant's mutable fields.
func (h *TenantHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	t, err := h.service.Store().GetTenant(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	var req struct {
		Name                  *string  `json:"name"`
		Tier                  *string  `json:"tier"`
		MaxSessionsPerDay     *int     `json:"max_sessions_per_day"`
		MaxConcurrentSessions *int     `json:"max_concurrent_sessions"`
		MaxBudgetUSDPerSession *float64 `json:"max_budget_usd_per_session"`
		AllowedCLIs           *string  `json:"allowed_clis"`
		AllowedModels         *string  `json:"allowed_models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Tier != nil {
		t.Tier = *req.Tier
	}
	if req.MaxSessionsPerDay != nil {
		t.MaxSessionsPerDay = *req.MaxSessionsPerDay
	}
	if req.MaxConcurrentSessions != nil {
		t.MaxConcurrentSessions = *req.MaxConcurrentSessions
	}
	if req.MaxBudgetUSDPerSession != nil {
		t.MaxBudgetUSDPerSession = *req.MaxBudgetUSDPerSession
	}
	if req.AllowedCLIs != nil {
		t.AllowedCLIs = *req.AllowedCLIs
	}
	if req.AllowedModels != nil {
		t.AllowedModels = req.AllowedModels
	}

	if err := h.service.Store().UpdateTenant(r.Context(), t); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Delete removes a tenant.
func (h *TenantHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	if err := h.service.Store().DeleteTenant(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Usage returns aggregated usage statistics for a tenant.
func (h *TenantHandler) Usage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	period := r.URL.Query().Get("period")

	var since time.Time
	switch period {
	case "30d":
		since = time.Now().AddDate(0, 0, -30)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	default:
		since = time.Now().AddDate(0, 0, -7)
	}

	summary, err := h.service.Store().GetUsageSummary(r.Context(), id, since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// AddKeyPool adds a managed key to the operator key pool.
func (h *TenantHandler) AddKeyPool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider" validate:"required"`
		Token    string `json:"token" validate:"required"`
		Weight   int    `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validate.Struct(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	encrypted, err := h.cryptoSvc.Encrypt(req.Token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	weight := req.Weight
	if weight <= 0 {
		weight = 1
	}

	entry := &tenant.KeyPoolEntry{
		ID:             generateKeyPoolID(),
		Provider:       req.Provider,
		EncryptedToken: encrypted,
		Weight:         weight,
		Active:         true,
	}

	if err := h.service.Store().AddKeyPoolEntry(r.Context(), entry); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// ListKeyPool lists keys in the operator key pool.
func (h *TenantHandler) ListKeyPool(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	entries, err := h.service.Store().ListKeyPool(r.Context(), provider)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []*tenant.KeyPoolEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": entries})
}

// DeleteKeyPool removes a key from the pool.
func (h *TenantHandler) DeleteKeyPool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "keyID")
	if err := h.service.Store().DeleteKeyPoolEntry(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generateKeyPoolID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
