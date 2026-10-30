package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
	"github.com/ubaid/hisaab/internal/settlement"
)

type defaultTaxAdapter struct {
	cfg nptoolkit.TaxConfig
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

	// 2. Initialize Core Engine Spine
	memLedger := ledger.NewMemoryLedger()
	if err := memLedger.SeedStandardAccounts(); err != nil {
		slog.Error("Failed to seed standard accounts", "error", err)
		os.Exit(1)
	}
	_, _ = memLedger.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	cal := calendar.New(nil)
	taxAdapter := &defaultTaxAdapter{cfg: nptoolkit.DefaultIndiaTaxConfig()}

	orch := settlement.NewOrchestrator(memLedger, matcher, cal, taxAdapter, nil)

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
			bal, _ := memLedger.Balance(r.Context(), acc)
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

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
		} else {
			slog.Info("Server stopped gracefully")
		}
	}
}
