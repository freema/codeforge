package handlers

import (
	"net/http"
	"strconv"

	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/keys"
)

// RepoHandler handles repository listing endpoints.
type RepoHandler struct {
	keyRegistry keys.Registry
}

// NewRepoHandler creates a new repository handler.
func NewRepoHandler(keyRegistry keys.Registry) *RepoHandler {
	return &RepoHandler{keyRegistry: keyRegistry}
}

// List handles GET /api/v1/repositories.
// Supports two auth modes:
//   - ?provider_key=my-github — uses token from key registry
//   - ?provider=github + X-Provider-Token header — inline token
func (h *RepoHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	var token string
	var provider gitpkg.Provider

	providerKey := r.URL.Query().Get("provider_key")
	providerParam := r.URL.Query().Get("provider")
	inlineToken := r.Header.Get("X-Provider-Token")

	switch {
	case providerKey != "":
		// Mode 1: resolve from key registry
		t, p, err := h.keyRegistry.ResolveByName(ctx, providerKey)
		if err != nil {
			writeAppError(w, err)
			return
		}
		token = t
		provider = gitpkg.Provider(p)

	case providerParam != "" && inlineToken != "":
		// Mode 2: inline token
		provider = gitpkg.Provider(providerParam)
		token = inlineToken

	default:
		writeError(w, http.StatusBadRequest, "provide either provider_key query param, or provider query param with X-Provider-Token header")
		return
	}

	if provider != gitpkg.ProviderGitHub && provider != gitpkg.ProviderGitLab {
		writeError(w, http.StatusBadRequest, "provider must be 'github' or 'gitlab'")
		return
	}

	repos, err := gitpkg.ListRepos(ctx, provider, token, page, perPage)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repositories": repos,
		"count":        len(repos),
		"provider":     string(provider),
		"page":         page,
		"per_page":     perPage,
	})
}
