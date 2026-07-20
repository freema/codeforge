package handlers

import (
	"net/http"

	"github.com/freema/codeforge/internal/tools"
)

// ToolHandler serves the built-in tool catalog. Custom tools are attached to
// sessions via MCP servers, not registered through the API.
type ToolHandler struct{}

// NewToolHandler creates a new tool handler.
func NewToolHandler() *ToolHandler {
	return &ToolHandler{}
}

// Catalog handles GET /api/v1/tools/catalog.
func (h *ToolHandler) Catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tools": tools.BuiltinCatalog(),
	})
}
