package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Role defines authenticated participant authority in Hisaab.
type Role string

const (
	RoleCollectorRSP Role = "COLLECTOR_RSP" // Settlement Agency: full operational authority
	RoleBuyerApp     Role = "BUYER_APP_BAP" // Buyer App: submit claims, trigger refunds
	RoleSellerApp    Role = "SELLER_APP_BPP" // Seller App: submit claims, view payouts
	RoleAuditor      Role = "AUDITOR"        // Auditor: read-only verification
)

var (
	ErrUnauthorizedRole = errors.New("auth: role not permitted to perform this operation")
	ErrMissingRole      = errors.New("auth: missing X-Hisaab-Role header")
)

type contextKey string

const roleContextKey contextKey = "hisaab_role"

// ContextWithRole embeds an authenticated role into the request context.
func ContextWithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleContextKey, role)
}

// RoleFromContext extracts the role from the request context.
func RoleFromContext(ctx context.Context) (Role, bool) {
	val, ok := ctx.Value(roleContextKey).(Role)
	return val, ok
}

// RBACMiddleware enforces server-side role-based access control.
// If allowedRoles is empty, any valid role is permitted.
func RBACMiddleware(allowedRoles ...Role) func(http.Handler) http.Handler {
	allowedMap := make(map[Role]bool)
	for _, r := range allowedRoles {
		allowedMap[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawRole := r.Header.Get("X-Hisaab-Role")
			if rawRole == "" {
				// Default to COLLECTOR_RSP for backward-compatible simulator requests if not specified
				rawRole = string(RoleCollectorRSP)
			}

			normalized := Role(strings.ToUpper(strings.TrimSpace(rawRole)))
			switch normalized {
			case RoleCollectorRSP, RoleBuyerApp, RoleSellerApp, RoleAuditor:
				// Valid recognized role
			default:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "unrecognized participant role",
					"code":  "INVALID_ROLE",
					"role":  rawRole,
				})
				return
			}

			if len(allowedMap) > 0 && !allowedMap[normalized] {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":         "access denied: insufficient permissions for role",
					"code":          "RBAC_FORBIDDEN",
					"current_role":  normalized,
					"required_role": allowedRoles,
				})
				return
			}

			ctx := ContextWithRole(r.Context(), normalized)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
