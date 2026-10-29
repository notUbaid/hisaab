package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/settlement"
)

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

	// Initialize key registry & request signer for SA Simulator
	keyRegistry := auth.NewMemoryKeyRegistry()
	_, saPriv, err := auth.GenerateKeyPair()
	if err != nil {
		slog.Error("Failed to generate SA keypair", "error", err)
		os.Exit(1)
	}
	signer := auth.NewRequestSigner("sa.ondc.org", "sa-key-1", saPriv, keyRegistry)

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleHandler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: verifySigs},
		signer,
		matcher,
	)

	health := &HealthState{}
	health.SetReady(true)

	mux := http.NewServeMux()

	// Operational & health probes
	mux.HandleFunc("GET /healthz", health.HandleLiveness)
	mux.HandleFunc("GET /readyz", health.HandleReadiness)

	// ONDC RSF 2.0 Beckn Endpoints
	mux.HandleFunc("POST /settle", settleHandler.HandleSettle)
	mux.HandleFunc("POST /on_settle", settleHandler.HandleOnSettle)

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

func init() {
	// Ensure formatted strings are available
	_ = fmt.Sprintf("")
}
