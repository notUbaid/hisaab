package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/audit"
	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
	"github.com/ubaid/hisaab/internal/recon"
	"github.com/ubaid/hisaab/internal/settlement"
)

func setupTestServer(t *testing.T) http.Handler {
	t.Helper()
	mem := ledger.NewMemoryLedger()
	if err := mem.SeedStandardAccounts(); err != nil {
		t.Fatalf("Failed to seed standard accounts: %v", err)
	}
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountLiability, "INR", false)
	_, _ = mem.RegisterAccount("sa:tolerance_adjustment", ledger.AccountExpense, "INR", false)
	_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
	_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	cal := calendar.New(nil)
	taxAdapter := &defaultTaxAdapter{cfg: nptoolkit.DefaultIndiaTaxConfig()}
	orch := settlement.NewOrchestrator(mem, matcher, cal, taxAdapter, nil)

	keyRegistry := auth.NewMemoryKeyRegistry()
	_, saPriv, _ := auth.GenerateKeyPair()
	signer := auth.NewRequestSigner("sa.ondc.org", "sa-key-1", saPriv, keyRegistry)

	settleHandler := settlement.NewHandler(settlement.HandlerConfig{VerifySignatures: false}, signer, matcher)
	settleHandler.SetOrchestrator(orch)

	reconRepo := recon.NewMemoryRepository()
	// Pre-seed an exception
	_ = reconRepo.SaveExceptions(context.Background(), []recon.ReconException{
		{
			ID:             "EXC-TEST-001",
			NetworkOrderID: "ORD-9999",
			Class:          recon.ExceptionAmountMismatch,
			ExpectedPaise:  100000,
			ActualPaise:    99000,
			Status:         recon.ReconStatusOpen,
			Details:        "Buyer claims 100000, seller claims 99000",
		},
	})

	rootStore := audit.NewMemoryRootStore()
	_ = rootStore.SaveRoot(context.Background(), audit.SignedRoot{
		BatchID:    "BATCH-20261031-001",
		MerkleRoot: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		LeafCount:  2,
		Algorithm:  "SHA-256",
		NPPairID:   "BUYER_NP::SELLER_NP",
	})

	registryClient := auth.NewONDCRegistryClient("https://registry.example.com", nil, keyRegistry)
	limiter := auth.NewInProcessLimiter()
	health := &HealthState{}
	health.SetReady(true)

	return buildServerMux(mem, orch, settleHandler, reconRepo, rootStore, registryClient, limiter, nil, time.Now().UTC(), health)
}

func TestAPI_ScenariosEndpoint(t *testing.T) {
	handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/scenarios", nil)
	req.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count     int `json:"count"`
		Scenarios []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if resp.Count < 10 {
		t.Errorf("Expected at least 10 scenarios loaded from spec/scenarios, got %d", resp.Count)
	}
}

func TestAPI_ExceptionsAndApprove(t *testing.T) {
	handler := setupTestServer(t)

	// 1. List exceptions
	req := httptest.NewRequest(http.MethodGet, "/v1/exceptions", nil)
	req.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var listResp struct {
		Count      int                    `json:"count"`
		Exceptions []recon.ReconException `json:"exceptions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Failed to decode exceptions: %v", err)
	}
	if listResp.Count != 1 || listResp.Exceptions[0].ID != "EXC-TEST-001" {
		t.Errorf("Expected EXC-TEST-001, got %+v", listResp.Exceptions)
	}

	// 2. Approve exception with RoleCollectorRSP
	body, _ := json.Marshal(map[string]string{
		"exception_id": "EXC-TEST-001",
		"remedy":       "Approved 1000 paise tolerance",
		"approved_by":  "lead_operator",
	})
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/recon/approve", bytes.NewReader(body))
	approveReq.Header.Set("Content-Type", "application/json")
	approveReq.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
	wApprove := httptest.NewRecorder()
	handler.ServeHTTP(wApprove, approveReq)

	if wApprove.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for approval, got %d: %s", wApprove.Code, wApprove.Body.String())
	}

	// 3. Attempt approval with Buyer App role (must be 403 Forbidden by RBAC)
	buyerReq := httptest.NewRequest(http.MethodPost, "/v1/recon/approve", bytes.NewReader(body))
	buyerReq.Header.Set("Content-Type", "application/json")
	buyerReq.Header.Set("X-Hisaab-Role", "BUYER_APP_BAP")
	wBuyer := httptest.NewRecorder()
	handler.ServeHTTP(wBuyer, buyerReq)

	if wBuyer.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for non-operator role, got %d", wBuyer.Code)
	}
}

func TestAPI_TelemetryEndpoint(t *testing.T) {
	handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/worker/telemetry", nil)
	req.Header.Set("X-Hisaab-Role", "AUDITOR")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var telem struct {
		UptimeSeconds int64 `json:"uptime_seconds"`
		Timestamp     time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &telem); err != nil {
		t.Fatalf("Failed to parse telemetry: %v", err)
	}
	if telem.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp in telemetry response")
	}
}

func TestAPI_AuditRootsEndpoint(t *testing.T) {
	handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/roots", nil)
	req.Header.Set("X-Hisaab-Role", "AUDITOR")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var rootsResp struct {
		Count int                `json:"count"`
		Roots []audit.SignedRoot `json:"roots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rootsResp); err != nil {
		t.Fatalf("Failed to decode roots: %v", err)
	}
	if rootsResp.Count != 1 || rootsResp.Roots[0].BatchID != "BATCH-20261031-001" {
		t.Errorf("Expected BATCH-20261031-001, got %+v", rootsResp.Roots)
	}
}

func TestAPI_RateLimiting(t *testing.T) {
	handler := setupTestServer(t)

	// Approval has a limit of 5 req/min. Let's make 6 requests from the same client ID.
	body, _ := json.Marshal(map[string]string{
		"exception_id": "EXC-TEST-001",
		"remedy":       "Approved",
	})

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/recon/approve", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
		req.Header.Set("X-Hisaab-Client-ID", "test-client-rate-limit")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Request %d should be allowed, got status %d: %s", i, w.Code, w.Body.String())
		}
	}

	// 6th request should exceed quota and return HTTP 429 Too Many Requests
	req6 := httptest.NewRequest(http.MethodPost, "/v1/recon/approve", bytes.NewReader(body))
	req6.Header.Set("Content-Type", "application/json")
	req6.Header.Set("X-Hisaab-Role", "COLLECTOR_RSP")
	req6.Header.Set("X-Hisaab-Client-ID", "test-client-rate-limit")
	w6 := httptest.NewRecorder()
	handler.ServeHTTP(w6, req6)

	if w6.Code != http.StatusTooManyRequests {
		t.Fatalf("Request 6 should be rate-limited with 429, got %d: %s", w6.Code, w6.Body.String())
	}
	if w6.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("Expected X-RateLimit-Remaining to be 0, got %s", w6.Header().Get("X-RateLimit-Remaining"))
	}
}
