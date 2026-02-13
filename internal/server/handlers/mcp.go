package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/mcp"
)

// MCPHandler handles MCP server configuration endpoints.
type MCPHandler struct {
	registry *mcp.Registry
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(registry *mcp.Registry) *MCPHandler {
	return &MCPHandler{registry: registry}
}

// CreateGlobal handles POST /api/v1/mcp/servers.
func (h *MCPHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string            `json:"name" validate:"required"`
		Package string            `json:"package" validate:"required"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "name and package are required")
		return
	}

	srv := mcp.Server{
		Name:    req.Name,
		Package: req.Package,
		Args:    req.Args,
		Env:     req.Env,
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
