package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/freema/codeforge/internal/tenant"
)

type ctxKey string

const tenantCtxKey ctxKey = "tenant"

// TenantLookup is the subset of the tenant store the auth layer needs.
type TenantLookup interface {
	GetTenantByTokenHash(ctx context.Context, hash string) (*tenant.Tenant, error)
}

// TenantAuth authenticates a request via EITHER the static operator token (full
// access, sets no tenant in context) OR a tenant API token ("cfk_..."), which is
// resolved to a tenant and injected into the request context. It augments — does
// not replace — operator-token behavior, so existing integrations keep working.
func TenantAuth(operatorToken string, lookup TenantLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if auth == token || token == "" {
				unauthorized(w)
				return
			}

			// Operator token → full access, no tenant attached.
			if operatorToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(operatorToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			// Tenant API token → resolve and attach the tenant.
			if lookup != nil && strings.HasPrefix(token, "cfk_") {
				if t, err := lookup.GetTenantByTokenHash(r.Context(), tenant.HashToken(token)); err == nil && t != nil {
					ctx := context.WithValue(r.Context(), tenantCtxKey, t)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			unauthorized(w)
		})
	}
}

// OperatorOnly rejects tenant-authenticated requests; only the operator token
// (no tenant in context) passes. Safe under plain BearerAuth too, where no tenant
// is ever attached, so operator access to admin routes is preserved.
func OperatorOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if TenantFromContext(r.Context()) != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "forbidden",
				"message": "operator token required for admin routes",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TenantFromContext returns the authenticated tenant, or nil for operator/no-tenant requests.
func TenantFromContext(ctx context.Context) *tenant.Tenant {
	t, _ := ctx.Value(tenantCtxKey).(*tenant.Tenant)
	return t
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthorized",
		"message": "missing or invalid Bearer token",
	})
}
