package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
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
		return 0
	}
	return res.TotalTaxPaise
}

func main() {
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

	// 2. Initialize Core Engine Spine (PostgreSQL with graceful In-Memory fallback)
	dbURL := os.Getenv("HISAAB_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	var eng ledger.Engine
	var pool *pgxpool.Pool

	if dbURL != "" {
		ctxInit, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
		p, err := pgxpool.New(ctxInit, dbURL)
		cancelInit()
		if err != nil {
			slog.Warn("Failed to connect to PostgreSQL; falling back to memory ledger", "error", err)
			mem := ledger.NewMemoryLedger()
			_ = mem.SeedStandardAccounts()
			_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)
			_, _ = mem.RegisterAccount("sa:tolerance_adjustment", ledger.AccountAsset, "INR", false)
			_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
			_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
			eng = mem
		} else {
			slog.Info("Connected to PostgreSQL ledger", "db_configured", true)
			pool = p
			pgLedger := ledger.New(pool)
			ctxSeed, cancelSeed := context.WithTimeout(context.Background(), 5*time.Second)
			if err := pgLedger.SeedStandardAccounts(ctxSeed); err != nil {
				slog.Error("Failed to seed standard accounts in PostgreSQL", "error", err)
			}
			_ = pgLedger.RegisterAccount(ctxSeed, "sa:suspense", ledger.AccountAsset, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "sa:tolerance_adjustment", ledger.AccountAsset, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
			_ = pgLedger.RegisterAccount(ctxSeed, "tax:tds_payable", ledger.AccountLiability, "INR", false)
			cancelSeed()
			eng = pgLedger
		}
	} else {
		slog.Info("HISAAB_DB_URL not configured; running in-memory ledger engine")
		mem := ledger.NewMemoryLedger()
		_ = mem.SeedStandardAccounts()
		_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)
		_, _ = mem.RegisterAccount("sa:tolerance_adjustment", ledger.AccountAsset, "INR", false)
		_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
		_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
		eng = mem
	}

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	cal := calendar.New(nil)
	taxAdapter := &defaultTaxAdapter{cfg: nptoolkit.DefaultIndiaTaxConfig()}

	orch := settlement.NewOrchestrator(eng, matcher, cal, taxAdapter, nil)
	if pool != nil {
		orch.SetPool(pool)
	}

	// 3. HTTP Settlement Handler
	settleHandler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: verifySigs},
		signer,
		matcher,
	)
	settleHandler.SetOrchestrator(orch)

	health := &HealthState{}
	health.SetReady(true)

	mux := http.NewServeMux()

	// Operational & health probes
	mux.HandleFunc("GET /healthz", health.HandleLiveness)
	mux.HandleFunc("GET /readyz", health.HandleReadiness)

	// ONDC RSF 2.0 Beckn Endpoints
	mux.HandleFunc("POST /settle", settleHandler.HandleSettle)
	mux.HandleFunc("POST /on_settle", settleHandler.HandleOnSettle)

	// V1 Query APIs for Console and CLI
	mux.HandleFunc("GET /v1/batches", func(w http.ResponseWriter, r *http.Request) {
		batches := orch.ListBatches()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":   len(batches),
			"batches": batches,
		})
	})

	mux.HandleFunc("GET /v1/batches/{id}", func(w http.ResponseWriter, r *http.Request) {
		batchID := r.PathValue("id")
		batch, ok := orch.GetBatch(batchID)
		if !ok {
			http.Error(w, "Batch not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(batch)
	})

	mux.HandleFunc("GET /v1/trial-balance", func(w http.ResponseWriter, r *http.Request) {
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
	})

	mux.HandleFunc("GET /v1/exceptions", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Live scenario execution endpoint for Ops Console workbench
	mux.HandleFunc("POST /v1/scenarios/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		scenarioID := r.PathValue("id")
		scenarioIDLower := strings.ToLower(scenarioID)

		// Locate scenario YAML file
		scenarioPath := fmt.Sprintf("spec/scenarios/%s.yaml", scenarioIDLower)
		if _, err := os.Stat(scenarioPath); err != nil {
			scenarioPath = fmt.Sprintf("spec/scenarios/%s.yaml", strings.ToUpper(scenarioID))
			if _, err2 := os.Stat(scenarioPath); err2 != nil {
				http.Error(w, fmt.Sprintf("Scenario file %s not found", scenarioPath), http.StatusNotFound)
				return
			}
		}

		sc, err := scenariodsl.LoadScenarioFile(scenarioPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load scenario %s: %v", scenarioID, err), http.StatusBadRequest)
			return
		}

		runner := scenariodsl.NewRunner()
		result, err := runner.Run(r.Context(), sc)
		if err != nil {
			http.Error(w, fmt.Sprintf("Scenario execution error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
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
