package handlers

import (
	"net/http"
	"sort"

	"github.com/freema/codeforge/internal/tool/runner"
)

// CLIInfo describes a registered CLI runner for API responses.
type CLIInfo struct {
	Name         string `json:"name"`
	BinaryPath   string `json:"binary_path"`
	DefaultModel string `json:"default_model,omitempty"`
}

// CLIHandler handles CLI-related HTTP endpoints.
type CLIHandler struct {
	registry *runner.Registry
	configs  map[string]CLIInfo
}

// NewCLIHandler creates a new CLI handler.
func NewCLIHandler(registry *runner.Registry, configs map[string]CLIInfo) *CLIHandler {
	return &CLIHandler{registry: registry, configs: configs}
}

// List handles GET /api/v1/cli — returns all registered CLIs with availability status.
func (h *CLIHandler) List(w http.ResponseWriter, r *http.Request) {
	type cliEntry struct {
		Name         string `json:"name"`
		BinaryPath   string `json:"binary_path"`
		DefaultModel string `json:"default_model,omitempty"`
		Available    bool   `json:"available"`
		IsDefault    bool   `json:"is_default"`
	}

	names := h.registry.Available()
	sort.Strings(names)

	entries := make([]cliEntry, 0, len(names))
	for _, name := range names {
		info := h.configs[name]
		entries = append(entries, cliEntry{
			Name:         name,
			BinaryPath:   info.BinaryPath,
			DefaultModel: info.DefaultModel,
			Available:    runner.CheckBinary(info.BinaryPath),
			IsDefault:    name == h.registry.DefaultCLI(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cli": entries,
	})
}

// Health handles GET /api/v1/cli/health — 200 if default CLI binary is available, 503 otherwise.
func (h *CLIHandler) Health(w http.ResponseWriter, r *http.Request) {
	defaultName := h.registry.DefaultCLI()
	info, ok := h.configs[defaultName]
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status":  "error",
			"cli":     defaultName,
			"message": "default CLI not configured",
		})
		return
	}

	available := runner.CheckBinary(info.BinaryPath)
	if !available {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status":  "unavailable",
			"cli":     defaultName,
			"binary":  info.BinaryPath,
			"message": "default CLI binary not found in PATH",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"cli":    defaultName,
		"binary": info.BinaryPath,
	})
}
