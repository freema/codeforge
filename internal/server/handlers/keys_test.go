package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/freema/codeforge/internal/apperror"
	"github.com/freema/codeforge/internal/keys"
)

// mockRegistry implements keys.Registry for testing.
type mockRegistry struct {
	keys    []keys.Key
	created []keys.Key
	err     error
}

func (m *mockRegistry) Create(_ context.Context, key keys.Key) error {
	if m.err != nil {
		return m.err
	}
	for _, k := range m.created {
		if k.Name == key.Name && k.Provider == key.Provider {
			return apperror.Conflict("key '%s' already exists", key.Name)
		}
	}
	m.created = append(m.created, key)
	return nil
}

func (m *mockRegistry) List(_ context.Context) ([]keys.Key, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.keys, nil
}

func (m *mockRegistry) Delete(_ context.Context, name string) error {
	if m.err != nil {
		return m.err
	}
	for _, k := range m.keys {
		if k.Name == name {
			return nil
		}
	}
	return apperror.NotFound("key '%s' not found", name)
}

func (m *mockRegistry) Resolve(_ context.Context, provider, name string) (string, error) {
	return "", nil
}

func (m *mockRegistry) Verify(_ context.Context, name string) (*keys.VerifyResult, string, error) {
	for _, k := range m.keys {
		if k.Name == name {
			return &keys.VerifyResult{Valid: true, Username: "testuser"}, k.Provider, nil
		}
	}
	return nil, "", apperror.NotFound("key '%s' not found", name)
}

func (m *mockRegistry) ResolveByName(_ context.Context, name string) (string, string, error) {
	return "", "", nil
}

func (m *mockRegistry) ResolveFullByName(_ context.Context, name string) (string, string, string, error) {
	return "", "", "", nil
}

func TestKeyHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "valid key",
			body:       `{"name":"my-key","provider":"github","token":"ghp_secret"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name",
			body:       `{"provider":"github","token":"ghp_secret"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing token",
			body:       `{"name":"my-key","provider":"github"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid name characters",
			body:       `{"name":"my key!","provider":"github","token":"tok"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &mockRegistry{}
			h := NewKeyHandler(reg)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/keys", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestKeyHandler_Create_Duplicate(t *testing.T) {
	reg := &mockRegistry{
		created: []keys.Key{{Name: "existing", Provider: "github"}},
	}
	h := NewKeyHandler(reg)

	body := `{"name":"existing","provider":"github","token":"tok"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409", w.Code)
	}
}

func TestKeyHandler_List(t *testing.T) {
	reg := &mockRegistry{
		keys: []keys.Key{
			{Name: "gh-key", Provider: "github"},
			{Name: "gl-key", Provider: "gitlab"},
		},
	}
	h := NewKeyHandler(reg)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/keys", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}

	var resp struct {
		Keys []keys.Key `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Keys) != 2 {
		t.Errorf("keys count: got %d, want 2", len(resp.Keys))
	}
}

func TestKeyHandler_Delete(t *testing.T) {
	reg := &mockRegistry{
		keys: []keys.Key{{Name: "to-delete", Provider: "github"}},
	}
	h := NewKeyHandler(reg)

	// chi context needed for URL params
	r := chi.NewRouter()
	r.Delete("/api/v1/keys/{name}", h.Delete)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/keys/to-delete", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestKeyHandler_Delete_NotFound(t *testing.T) {
	reg := &mockRegistry{}
	h := NewKeyHandler(reg)

	r := chi.NewRouter()
	r.Delete("/api/v1/keys/{name}", h.Delete)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/keys/nonexistent", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

func TestKeyHandler_Verify(t *testing.T) {
	reg := &mockRegistry{
		keys: []keys.Key{{Name: "my-key", Provider: "github"}},
	}
	h := NewKeyHandler(reg)

	r := chi.NewRouter()
	r.Get("/api/v1/keys/{name}/verify", h.Verify)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/keys/my-key/verify", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["valid"] != true {
		t.Error("expected valid=true")
	}
	if resp["username"] != "testuser" {
		t.Errorf("username: got %v, want testuser", resp["username"])
	}
}

func TestKeyHandler_Verify_NotFound(t *testing.T) {
	reg := &mockRegistry{}
	h := NewKeyHandler(reg)

	r := chi.NewRouter()
	r.Get("/api/v1/keys/{name}/verify", h.Verify)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/keys/nonexistent/verify", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}
