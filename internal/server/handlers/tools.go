package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/tools"
)

// ToolHandler handles tool configuration endpoints.
type ToolHandler struct {
	registry tools.Registry
}

// NewToolHandler creates a new tool handler.
func NewToolHandler(registry tools.Registry) *ToolHandler {
	return &ToolHandler{registry: registry}
}

// Create handles POST /api/v1/tools.
func (h *ToolHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string              `json:"name" validate:"required"`
		Type           tools.ToolType      `json:"type" validate:"required"`
		Description    string              `json:"description"`
		Version        string              `json:"version"`
		MCPPackage     string              `json:"mcp_package"`
		MCPCommand     string              `json:"mcp_command"`
		MCPArgs        []string            `json:"mcp_args"`
		RequiredConfig []tools.ConfigField `json:"required_config"`
		OptionalConfig []tools.ConfigField `json:"optional_config"`
		Capabilities   []string            `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	if !validName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must contain only alphanumeric characters, hyphens, and underscores")
		return
	}

	def := tools.ToolDefinition{
		Name:           req.Name,
		Type:           req.Type,
		Description:    req.Description,
		Version:        req.Version,
		MCPPackage:     req.MCPPackage,
		MCPCommand:     req.MCPCommand,
		MCPArgs:        req.MCPArgs,
		RequiredConfig: req.RequiredConfig,
		OptionalConfig: req.OptionalConfig,
		Capabilities:   req.Capabilities,
	}

	if err := h.registry.Create(r.Context(), "global", def); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":    req.Name,
		"message": "tool registered",
	})
}

// List handles GET /api/v1/tools.
func (h *ToolHandler) List(w http.ResponseWriter, r *http.Request) {
	defs, err := h.registry.List(r.Context(), "global")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tools")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tools": defs,
	})
}

// Catalog handles GET /api/v1/tools/catalog.
func (h *ToolHandler) Catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tools": tools.BuiltinCatalog(),
	})
}

// Get handles GET /api/v1/tools/{name}.
func (h *ToolHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "tool name is required")
		return
	}

	def, err := h.registry.Get(r.Context(), "global", name)
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, def)
}

// Delete handles DELETE /api/v1/tools/{name}.
func (h *ToolHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "tool name is required")
		return
	}

	if err := h.registry.Delete(r.Context(), "global", name); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "tool deleted",
	})
}
