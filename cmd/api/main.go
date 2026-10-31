package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/audit"
	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/crypto"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
	"github.com/ubaid/hisaab/internal/recon"
	"github.com/ubaid/hisaab/internal/scenariodsl"
	"github.com/ubaid/hisaab/internal/settlement"
)

type defaultTaxAdapter struct {
	cfg nptoolkit.TaxConfig
}

func (a *defaultTaxAdapter) CalculateOrderTax(orderAmount, commission, bff ledger.AmountPaise) (*settlement.TaxBreakdown, error) {
	res, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, a.cfg)
	if err != nil {
		return nil, fmt.Errorf("tax calculation failed: %w", err)
	}
	return &settlement.TaxBreakdown{
		OrderAmountPaise: res.OrderAmountPaise,
		CommissionPaise:  res.CommissionPaise,
		BFFPaise:         res.BFFPaise,
		TDSPaise:         res.TDSPaise,
		GSTTCSPaise:      res.GSTTCSPaise,
		TotalTaxPaise:    res.TotalTaxPaise,
	}, nil
}

func (a *defaultTaxAdapter) CalculateOrderTaxPaise(orderAmount, commission, bff ledger.AmountPaise) ledger.AmountPaise {
	res, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, a.cfg)
	if err != nil {
		slog.Error("CRITICAL: Legacy tax calculation failed, propagating zero may violate statutory rules", "error", err)
		return 0
	}
	return res.TotalTaxPaise
}

// corsMiddleware injects CORS headers for cross-origin browser access from Console.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Hisaab-Role, X-Hisaab-Client-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildServerMux constructs and wires all HTTP routes with rate limiting, RBAC, and persistence adapters.
func buildServerMux(
	eng ledger.Engine,
	orch *settlement.Orchestrator,
	settleHandler *settlement.Handler,
	reconRepo recon.Repository,
	rootStore audit.RootStore,
	registryClient *auth.ONDCRegistryClient,
	limiter auth.RateLimiter,
	pool *pgxpool.Pool,
	startTime time.Time,
	health *HealthState,
) http.Handler {
	rateLimitKeyFunc := func(r *http.Request) string {
		if clientID := r.Header.Get("X-Hisaab-Client-ID"); clientID != "" {
			return clientID
		}
		return r.RemoteAddr
	}

	settleRate := auth.Rate{Limit: 10, Window: time.Minute}
	queryRate := auth.Rate{Limit: 60, Window: time.Minute}
	approveRate := auth.Rate{Limit: 5, Window: time.Minute}

	withSettleLimit := auth.RateLimitMiddleware(limiter, rateLimitKeyFunc, settleRate)
	withQueryLimit := auth.RateLimitMiddleware(limiter, rateLimitKeyFunc, queryRate)
	withApproveLimit := auth.RateLimitMiddleware(limiter, rateLimitKeyFunc, approveRate)

	withSettleRBAC := auth.RBACMiddleware(auth.RoleCollectorRSP, auth.RoleBuyerApp, auth.RoleSellerApp)
	withApproveRBAC := auth.RBACMiddleware(auth.RoleCollectorRSP)
	withGeneralRBAC := auth.RBACMiddleware(auth.RoleCollectorRSP, auth.RoleBuyerApp, auth.RoleSellerApp, auth.RoleAuditor)

	mux := http.NewServeMux()

	// Operational & health probes
	if health != nil {
		mux.HandleFunc("GET /healthz", health.HandleLiveness)
		mux.HandleFunc("GET /readyz", health.HandleReadiness)
	}

	// ONDC RSF 2.0 Beckn Endpoints with Rate Limiting and RBAC
	if settleHandler != nil {
		mux.Handle("POST /settle", withSettleLimit(withSettleRBAC(http.HandlerFunc(settleHandler.HandleSettle))))
		mux.Handle("POST /on_settle", withSettleLimit(withSettleRBAC(http.HandlerFunc(settleHandler.HandleOnSettle))))
	}

	// V1 Query APIs for Console and CLI
	mux.Handle("GET /v1/batches", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batches := orch.ListBatches()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":   len(batches),
			"batches": batches,
		})
	}))))

	mux.Handle("GET /v1/batches/{id}", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batchID := r.PathValue("id")
		batch, ok := orch.GetBatch(batchID)
		if !ok {
			http.Error(w, "Batch not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(batch)
	}))))

	mux.Handle("GET /v1/trial-balance", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accounts := []string{
			"collector:ondc_settlement",
			"receiver:ondc_settlement",
			"tax:gst_tcs_payable",
			"tax:tds_payable",
			"sa:tolerance_adjustment",
			"sa:suspense",
		}
		type BalRow struct {
			Account string             `json:"account"`
			Balance ledger.AmountPaise `json:"balance_paise"`
		}
		rows := make([]BalRow, 0, len(accounts))
		for _, acc := range accounts {
			bal, _ := eng.Balance(r.Context(), acc)
			rows = append(rows, BalRow{Account: acc, Balance: bal})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"timestamp": time.Now().UTC(),
			"accounts":  rows,
		})
	}))))

	mux.Handle("GET /v1/exceptions", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusFilter := r.URL.Query().Get("status")
		exceptions, err := reconRepo.ListExceptions(r.Context(), statusFilter)
		if err != nil || len(exceptions) == 0 {
			batches := orch.ListBatches()
			unmatched := make([]settlement.MatchedOrder, 0)
			for _, b := range batches {
				unmatched = append(unmatched, b.UnmatchedOrders...)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count":      len(unmatched),
				"exceptions": unmatched,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":      len(exceptions),
			"exceptions": exceptions,
		})
	}))))

	// Tolerance Approval / Resolution endpoint with RBAC (RoleCollectorRSP only) and rate limit (5 req/min)
	type ReconApproveRequest struct {
		ExceptionID string `json:"exception_id"`
		Remedy      string `json:"remedy"`
		ApprovedBy  string `json:"approved_by"`
	}
	mux.Handle("POST /v1/recon/approve", withApproveLimit(withApproveRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ReconApproveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}
		if req.ExceptionID == "" {
			http.Error(w, `{"error":"exception_id is required"}`, http.StatusBadRequest)
			return
		}
		remedy := req.Remedy
		if remedy == "" {
			remedy = "Tolerance approval granted by operator"
		}
		if err := reconRepo.ResolveException(r.Context(), req.ExceptionID, remedy); err != nil {
			if errors.Is(err, recon.ErrExceptionNotFound) {
				http.Error(w, `{"error":"exception not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"failed to resolve exception: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "RESOLVED",
			"exception_id": req.ExceptionID,
			"remedy":       remedy,
			"resolved_at":  time.Now().UTC(),
		})
	}))))

	// ONDC Registry key lookup endpoint
	mux.Handle("GET /v1/registry/lookup", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subID := r.URL.Query().Get("subscriber_id")
		ukID := r.URL.Query().Get("unique_key_id")
		if subID == "" || ukID == "" {
			http.Error(w, `{"error":"subscriber_id and unique_key_id query params are required"}`, http.StatusBadRequest)
			return
		}
		pubKey, err := registryClient.LookupPublicKey(subID, ukID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"lookup failed: %v"}`, err), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriber_id": subID,
			"unique_key_id": ukID,
			"public_key":    hex.EncodeToString(pubKey),
			"verified":      true,
		})
	}))))

	// Merkle Root Store query endpoints
	mux.Handle("GET /v1/audit/roots", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roots, err := rootStore.ListRoots(r.Context(), 50)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to list roots: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count": len(roots),
			"roots": roots,
		})
	}))))

	mux.Handle("GET /v1/audit/roots/{id}", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batchID := r.PathValue("id")
		root, err := rootStore.GetRoot(r.Context(), batchID)
		if err != nil {
			http.Error(w, `{"error":"batch root not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(root)
	}))))

	// Dynamic scenario list endpoint
	type ScenarioSummaryItem struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		BRDSection     string `json:"brd_section"`
		SpecVersion    string `json:"spec_version"`
		NPPair         string `json:"np_pair"`
		CollectorAcc   string `json:"collector_account"`
		BalancePaise   int64  `json:"collector_balance_paise"`
		HasBuyerClaim  bool   `json:"has_buyer_claim"`
		HasSellerClaim bool   `json:"has_seller_claim"`
	}

	mux.Handle("GET /v1/scenarios", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		patternCandidates := []string{
			filepath.Join("spec", "scenarios", "*.yaml"),
			filepath.Join("scenarios", "*.yaml"),
			filepath.Join("..", "spec", "scenarios", "*.yaml"),
			filepath.Join("..", "..", "spec", "scenarios", "*.yaml"),
			filepath.Join("/app", "spec", "scenarios", "*.yaml"),
		}
		var files []string
		for _, pat := range patternCandidates {
			if matches, _ := filepath.Glob(pat); len(matches) > 0 {
				files = matches
				break
			}
		}

		var summaries []ScenarioSummaryItem
		for _, f := range files {
			sc, err := scenariodsl.LoadScenarioFile(f)
			if err != nil {
				continue
			}
			summaries = append(summaries, ScenarioSummaryItem{
				ID:             sc.ID,
				Title:          sc.Title,
				BRDSection:     sc.BRDSection,
				SpecVersion:    sc.SpecVersion,
				NPPair:         sc.Given.NPPair,
				CollectorAcc:   sc.Given.CollectorAccount,
				BalancePaise:   sc.Given.CollectorBalancePaise,
				HasBuyerClaim:  sc.Given.BuyerClaim != nil,
				HasSellerClaim: sc.Given.SellerClaim != nil,
			})
		}
		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].ID < summaries[j].ID
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":     len(summaries),
			"scenarios": summaries,
		})
	}))))

	// Live scenario execution endpoint for Ops Console workbench
	mux.Handle("POST /v1/scenarios/{id}/run", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scenarioID := r.PathValue("id")
		scenarioIDLower := strings.ToLower(scenarioID)

		// Locate scenario YAML file
		scenarioPath := fmt.Sprintf("spec/scenarios/%s.yaml", scenarioIDLower)
		sc, err := scenariodsl.LoadScenarioFile(scenarioPath)
		if err != nil {
			sc, err = scenariodsl.LoadScenarioFile(fmt.Sprintf("%s.yaml", scenarioIDLower))
			if err != nil {
				http.Error(w, fmt.Sprintf("Scenario file for %s not found: %v", scenarioID, err), http.StatusNotFound)
				return
			}
		}

		runner := scenariodsl.NewRunner()
		result, err := runner.Run(r.Context(), sc)
		if err != nil {
			http.Error(w, fmt.Sprintf("Scenario execution error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))))

	// Worker Telemetry & Outbox Queue health endpoint
	mux.Handle("GET /v1/worker/telemetry", withQueryLimit(withGeneralRBAC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uptime := int64(time.Since(startTime).Seconds())
		pendingEvents := 0
		dispatchedEvents := 0
		failedEvents := 0
		dbConnected := pool != nil

		if pool != nil {
			rows, qErr := pool.Query(r.Context(), `SELECT status, COUNT(*) FROM outbox_events GROUP BY status`)
			if qErr == nil {
				defer rows.Close()
				for rows.Next() {
					var status string
					var count int
					if scanErr := rows.Scan(&status, &count); scanErr == nil {
						switch status {
						case "PENDING":
							pendingEvents = count
						case "DISPATCHED":
							dispatchedEvents = count
						case "FAILED":
							failedEvents = count
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uptime_seconds":           uptime,
			"database_connected":       dbConnected,
			"pending_outbox_events":    pendingEvents,
			"dispatched_outbox_events": dispatchedEvents,
			"failed_outbox_events":     failedEvents,
			"total_batches_cached":     len(orch.ListBatches()),
			"timestamp":                time.Now().UTC(),
		})
	}))))

	return corsMiddleware(mux)
}

func main() {
	startTime := time.Now().UTC()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	verifySigs := os.Getenv("HISAAB_VERIFY_SIGNATURES") == "true"

	// 1. Initialize Key Registry & Signer for SA Simulator
	keyRegistry := auth.NewMemoryKeyRegistry()
	_, saPriv, err := auth.GenerateKeyPair()
	if err != nil {
		slog.Error("Failed to generate SA keypair", "error", err)
		os.Exit(1)
	}
	signer := auth.NewRequestSigner("sa.ondc.org", "sa-key-1", saPriv, keyRegistry)

	// 2. Initialize ONDC Registry Client (ADR-001)
	registryURL := os.Getenv("ONDC_REGISTRY_URL")
	if registryURL == "" {
		registryURL = "https://staging.registry.ondc.org"
	}
	registryClient := auth.NewONDCRegistryClient(registryURL, nil, keyRegistry)

	// 3. Initialize Field Envelope Encryptor (ADR-002)
	kekHex := os.Getenv("HISAAB_MASTER_KEK")
	var kek []byte
	if len(kekHex) == 64 {
		var decodeErr error
		kek, decodeErr = hex.DecodeString(kekHex)
		if decodeErr != nil {
			slog.Warn("Failed to decode HISAAB_MASTER_KEK hex, generating random key", "error", decodeErr)
			kek = nil
		}
	}
	if len(kek) != 32 {
		var genErr error
		kek, genErr = crypto.GenerateRandomKEK()
		if genErr != nil {
			slog.Error("Failed to generate master KEK", "error", genErr)
			os.Exit(1)
		}
		slog.Info("Initialized AES-256 envelope encryptor with ephemeral master KEK")
	}
	encryptor, err := crypto.NewEnvelopeEncryptor(kek)
	if err != nil {
		slog.Error("Failed to initialize envelope encryptor", "error", err)
		os.Exit(1)
	}
	_ = encryptor // Ready for field encryption of bank accounts and PII

	// 4. Initialize In-Process Sliding Window Rate Limiter (ADR-003)
	limiter := auth.NewInProcessLimiter()

	// 5. Initialize Core Engine Spine (PostgreSQL with graceful In-Memory fallback)
	dbURL := os.Getenv("HISAAB_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	var eng ledger.Engine
	var pool *pgxpool.Pool
	var reconRepo recon.Repository
	var rootStore audit.RootStore

	if dbURL != "" {
		ctxInit, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
		p, err := pgxpool.New(ctxInit, dbURL)
		cancelInit()
		if err != nil {
			slog.Warn("Failed to connect to PostgreSQL; falling back to memory ledger", "error", err)
			mem := ledger.NewMemoryLedger()
			_ = mem.SeedStandardAccounts()
			_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountLiability, "INR", false)
			_, _ = mem.RegisterAccount("sa:tolerance_adjustment", ledger.AccountExpense, "INR", false)
			_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
			_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
			eng = mem
			reconRepo = recon.NewMemoryRepository()
			rootStore = audit.NewMemoryRootStore()
		} else {
			slog.Info("Connected to PostgreSQL ledger", "db_configured", true)
			pool = p
			pgLedger := ledger.New(pool)
			ctxSeed, cancelSeed := context.WithTimeout(context.Background(), 5*time.Second)
			if err := pgLedger.SeedStandardAccounts(ctxSeed); err != nil {
				slog.Error("Failed to seed standard accounts in PostgreSQL", "error", err)
			}
			_ = pgLedger.RegisterAccount(ctxSeed, "sa:suspense", ledger.AccountLiability, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "sa:tolerance_adjustment", ledger.AccountExpense, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "tax:tds_payable", ledger.AccountLiability, "INR", false)
			cancelSeed()
			eng = pgLedger
			reconRepo = recon.NewPostgresRepository(pool)
			rootStore = audit.NewPostgresRootStore(pool)
		}
	} else {
		slog.Info("HISAAB_DB_URL not configured; running in-memory ledger engine")
		mem := ledger.NewMemoryLedger()
		_ = mem.SeedStandardAccounts()
		_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountLiability, "INR", false)
		_, _ = mem.RegisterAccount("sa:tolerance_adjustment", ledger.AccountExpense, "INR", false)
		_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
		_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
		eng = mem
		reconRepo = recon.NewMemoryRepository()
		rootStore = audit.NewMemoryRootStore()
	}

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	cal := calendar.New(nil)
	taxAdapter := &defaultTaxAdapter{cfg: nptoolkit.DefaultIndiaTaxConfig()}

	orch := settlement.NewOrchestrator(eng, matcher, cal, taxAdapter, nil)
	if pool != nil {
		orch.SetPool(pool)
	}

	// 6. HTTP Settlement Handler
	settleHandler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: verifySigs},
		signer,
		matcher,
	)
	settleHandler.SetOrchestrator(orch)

	health := &HealthState{}
	health.SetReady(true)

	handler := buildServerMux(
		eng, orch, settleHandler, reconRepo, rootStore, registryClient, limiter, pool, startTime, health,
	)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Hisaab SA Simulator API starting", "port", port, "verify_signatures", verifySigs)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGINT or SIGTERM
	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("Server failed unexpectedly", "error", err)
		os.Exit(1)
	case sig := <-shutdownSig:
		slog.Info("Shutdown signal received", "signal", sig.String())
		health.SetReady(false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if pool != nil {
			pool.Close()
		}

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
		} else {
			slog.Info("Server stopped gracefully")
		}
	}
}
