package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ubaid/hisaab/internal/auth"
)

func TestRBACMiddleware_PermittedRole(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := auth.RoleFromContext(r.Context())
		if !ok || role == "" {
			http.Error(w, "missing role in ctx", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(string(role)))
	})

	handler := auth.RBACMiddleware(auth.RoleCollectorRSP, auth.RoleBuyerApp)(baseHandler)

	// Permitted role: Collector RSP
	req1 := httptest.NewRequest(http.MethodPost, "/settle", nil)
	req1.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}

	// Permitted role: Buyer App
	req2 := httptest.NewRequest(http.MethodPost, "/settle", nil)
	req2.Header.Set("X-Hisaab-Role", "BUYER_APP_BAP")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
}

func TestRBACMiddleware_DeniedRole(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Only Collector RSP permitted
	handler := auth.RBACMiddleware(auth.RoleCollectorRSP)(baseHandler)

	// Auditor attempts mutation
	req := httptest.NewRequest(http.MethodPost, "/v1/recon/approve", nil)
	req.Header.Set("X-Hisaab-Role", "AUDITOR")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Auditor on mutation endpoint, got %d", rec.Code)
	}
}

func TestRBACMiddleware_InvalidRole(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.RBACMiddleware()(baseHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/reports", nil)
	req.Header.Set("X-Hisaab-Role", "MALICIOUS_ATTACKER_ROLE")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unrecognized role, got %d", rec.Code)
	}
}
