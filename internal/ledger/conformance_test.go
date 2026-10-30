package ledger_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/ledger/ledgertest"
)

func findMigrationFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "db", "migrations", "00001_init.sql")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("00001_init.sql not found in parent directories")
}

func loadMigrationUpSQL() (string, error) {
	path, err := findMigrationFile()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if parts := strings.Split(content, "-- goose Down"); len(parts) > 0 {
		return parts[0], nil
	}
	return content, nil
}

func TestLoadMigrationUpSQL(t *testing.T) {
	sql, err := loadMigrationUpSQL()
	if err != nil {
		t.Fatalf("loadMigrationUpSQL failed: %v", err)
	}
	if !strings.Contains(sql, "CREATE TABLE accounts") {
		t.Fatalf("migrationUp missing CREATE TABLE accounts")
	}
	if strings.Contains(sql, "DROP TABLE") {
		t.Fatalf("migrationUp must not contain DROP TABLE from goose Down")
	}
}

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
	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer adminPool.Close()

	migrationUp, err := loadMigrationUpSQL()
	if err != nil {
		t.Fatalf("Failed to load migration SQL: %v", err)
	}

	ledgertest.Run(t, func(t *testing.T) ledger.Engine {
		// Generate unique isolated schema for each subtest to guarantee zero cross-subtest balance contamination
		schemaName := fmt.Sprintf("test_subtest_%d_%d", time.Now().UnixNano(), rand.Intn(100000))
		_, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
		if err != nil {
			t.Fatalf("Failed to create isolated schema %s: %v", schemaName, err)
		}

		// Register cleanup for dropping schema (runs after subPool is closed)
		t.Cleanup(func() {
			_, _ = adminPool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName))
		})

		// Configure pool connection to direct all unqualified queries to this isolated schema
		cfg, err := pgxpool.ParseConfig(dbURL)
		if err != nil {
			t.Fatalf("Failed to parse db config: %v", err)
		}
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = make(map[string]string)
		}
		cfg.ConnConfig.RuntimeParams["search_path"] = schemaName

		subPool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatalf("Failed to create subtest pool for schema %s: %v", schemaName, err)
		}
		// subPool.Close will run before dropping the schema because Cleanups run in LIFO order
		t.Cleanup(subPool.Close)

		// Apply the schema DDL inside this isolated schema
		if _, err := subPool.Exec(ctx, migrationUp); err != nil {
			t.Fatalf("Failed to apply migration to schema %s: %v", schemaName, err)
		}

		pgLedger := ledger.New(subPool)
		if err := pgLedger.SeedStandardAccounts(ctx); err != nil {
			t.Fatalf("Failed to seed standard accounts in schema %s: %v", schemaName, err)
		}

		return pgLedger
	})
}
