package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/auth"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := DefaultWorkerConfig()

	if intervalStr := os.Getenv("POLL_INTERVAL_MS"); intervalStr != "" {
		if val, err := strconv.Atoi(intervalStr); err == nil && val > 0 {
			cfg.PollInterval = time.Duration(val) * time.Millisecond
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize cryptographic signer
	keyRegistry := auth.NewMemoryKeyRegistry()
	_, saPriv, err := auth.GenerateKeyPair()
	if err != nil {
		slog.Error("Failed to generate SA keypair", "error", err)
		os.Exit(1)
	}
	signer := auth.NewRequestSigner(cfg.SigningDomain, cfg.SigningKeyID, saPriv, keyRegistry)

	var pool *pgxpool.Pool
	dbURL := os.Getenv("HISAAB_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL != "" {
		p, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			slog.Error("Failed to initialize database pool", "error", err)
			os.Exit(1)
		}
		defer p.Close()
		pool = p
		slog.Info("Connected to PostgreSQL outbox database")
	} else {
		slog.Info("Running outbox worker in standalone in-memory mode")
	}

	processor := NewOutboxProcessor(cfg, pool, signer)

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Termination signal received, cancelling worker context")
		cancel()
	}()

	if err := processor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Outbox worker terminated with error", "error", err)
		os.Exit(1)
	}
}
