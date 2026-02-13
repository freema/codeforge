package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/keys"
)

// KeyHandler handles key-related HTTP endpoints.
type KeyHandler struct {
	registry *keys.Registry
}

// NewKeyHandler creates a new key handler.
func NewKeyHandler(registry *keys.Registry) *KeyHandler {
	return &KeyHandler{registry: registry}
}

// Create handles POST /api/v1/keys.
func (h *KeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name" validate:"required"`
		Provider string `json:"provider" validate:"required"`
		Token    string `json:"token" validate:"required"`
		Scope    string `json:"scope,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "name, provider, and token are required")
		return
	}

	key := keys.Key{
		Name:     req.Name,
		Provider: req.Provider,
		Token:    req.Token,
		Scope:    req.Scope,
	}

	if err := h.registry.Create(r.Context(), key); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":     req.Name,
		"provider": req.Provider,
		"message":  "key registered",
	})
}

// List handles GET /api/v1/keys.
func (h *KeyHandler) List(w http.ResponseWriter, r *http.Request) {
	keyList, err := h.registry.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys": keyList,
	})
}

// Delete handles DELETE /api/v1/keys/{name}.
func (h *KeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "key name is required")
		return
	}

	if err := h.registry.Delete(r.Context(), name); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "key deleted",
	})
}
