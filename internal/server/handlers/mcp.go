package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/tool/mcp"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// MCPHandler handles MCP server configuration endpoints.
type MCPHandler struct {
	registry mcp.Registry
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(registry mcp.Registry) *MCPHandler {
	return &MCPHandler{registry: registry}
}

// CreateGlobal handles POST /api/v1/mcp/servers.
func (h *MCPHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name" validate:"required"`
		Transport string `json:"transport,omitempty"` // "stdio" (default) or "http"
		// stdio fields
		Package string            `json:"package,omitempty"`
		Command string            `json:"command,omitempty"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
		// http fields
		URL     string            `json:"url,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must contain only alphanumeric characters, hyphens, and underscores")
		return
	}

	transport := req.Transport
	if transport == "" {
		transport = "stdio"
	}

	switch transport {
	case "http":
		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required for http transport")
			return
		}
	case "stdio":
		if req.Package == "" {
			writeError(w, http.StatusBadRequest, "package is required for stdio transport")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "transport must be 'stdio' or 'http'")
		return
	}

	srv := mcp.Server{
		Name:      req.Name,
		Transport: transport,
		Command:   req.Command,
		Package:   req.Package,
		Args:      req.Args,
		Env:       req.Env,
		URL:       req.URL,
		Headers:   req.Headers,
	}

	if err := h.registry.CreateGlobal(r.Context(), srv); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":    req.Name,
		"message": "MCP server registered",
	})
}

// ListGlobal handles GET /api/v1/mcp/servers.
func (h *MCPHandler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	servers, err := h.registry.ListGlobal(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"servers": servers,
	})
}

// DeleteGlobal handles DELETE /api/v1/mcp/servers/{name}.
func (h *MCPHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "server name is required")
		return
	}

	if err := h.registry.DeleteGlobal(r.Context(), name); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "MCP server deleted",
	})
}
