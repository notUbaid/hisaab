package ledger_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/ledger/ledgertest"
)

func TestConformance_MemoryLedger(t *testing.T) {
	ledgertest.Run(t, func(t *testing.T) ledger.Engine {
		mem := ledger.NewMemoryLedger()
		if err := mem.SeedStandardAccounts(); err != nil {
			t.Fatalf("Failed to seed standard accounts: %v", err)
		}
		return mem
	})
}

func TestConformance_PostgreSQLLedger(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping live PostgreSQL conformance run (run with postgres:16 in CI/staging)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	ledgertest.Run(t, func(t *testing.T) ledger.Engine {
		pgLedger := ledger.New(pool)
		if err := pgLedger.SeedStandardAccounts(ctx); err != nil {
			t.Fatalf("Failed to seed standard accounts in postgres: %v", err)
		}
		return pgLedger
	})
}
