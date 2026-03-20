package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/keys"
)

// sentryRegions maps region names to their base API URLs.
var sentryRegions = map[string]string{
	"us": "https://sentry.io",
	"eu": "https://de.sentry.io",
}

// SentryHandler proxies requests to the Sentry API using stored auth tokens.
type SentryHandler struct {
	keyRegistry keys.Registry
	client      *http.Client
}

// NewSentryHandler creates a new Sentry proxy handler.
func NewSentryHandler(keyRegistry keys.Registry) *SentryHandler {
	return &SentryHandler{
		keyRegistry: keyRegistry,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListOrganizations handles GET /api/v1/sentry/organizations.
// Queries all Sentry regions (US + EU) and merges results, adding a "region"
// field to each org so subsequent calls can target the correct endpoint.
func (h *SentryHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	keyName := r.URL.Query().Get("key_name")
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "key_name query param is required")
		return
	}

	token, provider, err := h.keyRegistry.ResolveByName(r.Context(), keyName)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if provider != "sentry" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("key '%s' is a %s key, not a sentry key", keyName, provider))
		return
	}

	type sentryOrg struct {
		Slug   string `json:"slug"`
		Name   string `json:"name"`
		Region string `json:"region"`
	}

	var allOrgs []sentryOrg
	for region, baseURL := range sentryRegions {
		body, fetchErr := h.sentryGetFrom(r.Context(), token, baseURL, "/api/0/organizations/")
		if fetchErr != nil {
			continue
		}
		var orgs []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		}
		if jsonErr := json.Unmarshal(body, &orgs); jsonErr != nil {
			continue
		}
		for _, o := range orgs {
			allOrgs = append(allOrgs, sentryOrg{Slug: o.Slug, Name: o.Name, Region: region})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"organizations": allOrgs})
}

// ListProjects handles GET /api/v1/sentry/projects.
func (h *SentryHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if org == "" {
		writeError(w, http.StatusBadRequest, "org query param is required")
		return
	}
	h.proxyGet(w, r, fmt.Sprintf("/api/0/organizations/%s/projects/", org), "projects")
}

// ListIssues handles GET /api/v1/sentry/issues.
func (h *SentryHandler) ListIssues(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	project := r.URL.Query().Get("project")
	if org == "" || project == "" {
		writeError(w, http.StatusBadRequest, "org and project query params are required")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		query = "is:unresolved"
	}
	sort := r.URL.Query().Get("sort")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "50"
	}

	path := fmt.Sprintf("/api/0/projects/%s/%s/issues/?query=%s&limit=%s",
		org, project, query, limit)
	if sort != "" {
		path += "&sort=" + sort
	}

	h.proxyGet(w, r, path, "issues")
}

// GetIssue handles GET /api/v1/sentry/issues/{issueID}.
func (h *SentryHandler) GetIssue(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueID")
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issueID is required")
		return
	}
	h.proxyGet(w, r, fmt.Sprintf("/api/0/issues/%s/", issueID), "")
}

// GetLatestEvent handles GET /api/v1/sentry/issues/{issueID}/latest-event.
func (h *SentryHandler) GetLatestEvent(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueID")
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issueID is required")
		return
	}
	h.proxyGet(w, r, fmt.Sprintf("/api/0/issues/%s/events/latest/", issueID), "")
}

// regionBaseURL resolves the base URL for the given region query param.
// Falls back to US if unknown or empty.
func regionBaseURL(r *http.Request) string {
	region := r.URL.Query().Get("region")
	if base, ok := sentryRegions[region]; ok {
		return base
	}
	return sentryRegions["us"]
}

// proxyGet resolves a Sentry auth token from key_name, proxies a GET request
// to the Sentry API, and forwards the response. If wrapKey is non-empty, the
// response array is wrapped in {"<wrapKey>": <body>}.
func (h *SentryHandler) proxyGet(w http.ResponseWriter, r *http.Request, sentryPath string, wrapKey string) {
	keyName := r.URL.Query().Get("key_name")
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "key_name query param is required")
		return
	}

	token, provider, err := h.keyRegistry.ResolveByName(r.Context(), keyName)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if provider != "sentry" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("key '%s' is a %s key, not a sentry key", keyName, provider))
		return
	}

	baseURL := regionBaseURL(r)
	body, err := h.sentryGetFrom(r.Context(), token, baseURL, sentryPath)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if wrapKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var raw json.RawMessage = body
	wrapped := map[string]*json.RawMessage{
		wrapKey: &raw,
	}
	_ = json.NewEncoder(w).Encode(wrapped)
}

// sentryGetFrom makes an authenticated GET request to a specific Sentry region.
func (h *SentryHandler) sentryGetFrom(ctx context.Context, token, baseURL, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sentry API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading sentry response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := string(body)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("sentry API returned %d: %s", resp.StatusCode, msg)
	}

	return body, nil
}
