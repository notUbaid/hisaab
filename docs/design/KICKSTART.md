# KICKSTART.md — Hisaab: Ready to Build

> Generated: 29 Oct 2026. Follow this in order. Week 0 first, no exceptions.

---

## Prerequisites (Before Week 0, Day 1)

```powershell
# 1. Install Go 1.23+
# https://go.dev/dl/

# 2. Install Docker Desktop (for Postgres, Toxiproxy, testcontainers)
# https://www.docker.com/products/docker-desktop/

# 3. Install Node 22+ (for console)
# https://nodejs.org/

# 4. Install goose (DB migrations)
go install github.com/pressly/goose/v3/cmd/goose@latest

# 5. Install sqlc (type-safe SQL codegen)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# 6. Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## Step 1: Initialize the Repository

```powershell
cd C:\Projects\Hisaab
git init
git branch -M main
```

---

## Step 2: Create the Full Directory Scaffold

```powershell
# Core Go structure
New-Item -ItemType Directory -Force -Path `
  cmd\api, cmd\worker, cmd\cli, `
  internal\ledger, internal\settlement, internal\recon, `
  internal\verification, internal\nptoolkit, internal\calendar, `
  internal\auth, internal\crypto, internal\telemetry, internal\middleware, `
  spec\scenarios, `
  db\migrations, db\queries, `
  console, `
  tests\scenarios, tests\property, tests\crash, tests\differential, tests\bench, `
  docs\adr, docs\runbooks

# Create placeholder docs
New-Item -ItemType File -Force -Path `
  docs\SPEC-COVERAGE.md, `
  docs\INVARIANTS.md, `
  docs\CORRECTNESS-REPORT.md, `
  docs\AMBIGUITIES.md, `
  docs\adr\ADR-000-index.md
```

---

## Step 3: Initialize Go Module

```powershell
go mod init github.com/<your-handle>/hisaab
```

Then install core dependencies:

```powershell
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
go get pgregory.net/rapid                   # property tests
go get github.com/testcontainers/testcontainers-go
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk/trace
go get github.com/prometheus/client_golang/prometheus
go get golang.org/x/crypto                  # field encryption
```

---

## Step 4: docker-compose.yml (One-Command Setup)

Write this to `docker-compose.yml`:

```yaml
version: "3.9"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: hisaab
      POSTGRES_USER: hisaab
      POSTGRES_PASSWORD: hisaab_dev
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./db/schema.sql:/docker-entrypoint-initdb.d/schema.sql
    command: >
      postgres
        -c fsync=on
        -c synchronous_commit=on
        -c log_min_duration_statement=500
        -c log_checkpoints=on
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U hisaab"]
      interval: 5s
      timeout: 5s
      retries: 5

  toxiproxy:
    image: ghcr.io/shopify/toxiproxy:2.9.0
    ports:
      - "8474:8474"   # admin API
      - "5433:5433"   # proxied Postgres (for fault injection tests)

volumes:
  pgdata:
```

Test it:

```powershell
docker compose up -d
docker compose ps
```

---

## Step 5: First Migration — Core Schema

Create `db/migrations/00001_init.sql`:

```sql
-- goose Up

-- Accounts
CREATE TABLE accounts (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,             -- e.g. collector:ondc_settlement
    type        TEXT NOT NULL,                    -- asset, liability, equity, income, expense, external
    currency    TEXT NOT NULL DEFAULT 'INR',
    constrained BOOLEAN NOT NULL DEFAULT FALSE,  -- true = no-negative enforced
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Journal transactions (one per atomic event)
CREATE TABLE journal_transactions (
    id            BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    description   TEXT,
    cause_type    TEXT NOT NULL,                  -- claim | instruction | bank_message | adjustment
    cause_id      BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Journal entries (two per transaction: debit + credit)
CREATE TABLE journal_entries (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  BIGINT NOT NULL REFERENCES journal_transactions(id),
    account_id      BIGINT NOT NULL REFERENCES accounts(id),
    direction       TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount_paise    BIGINT NOT NULL CHECK (amount_paise > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

-- Monthly partition for current month (add more via migration)
CREATE TABLE journal_entries_2026_10 PARTITION OF journal_entries
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

-- Idempotency keys
CREATE TABLE idempotency_keys (
    key          TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'done', 'error')),
    response     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days')
);
CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys (expires_at);

-- Claims (/settle submissions)
CREATE TABLE claims (
    id              BIGSERIAL PRIMARY KEY,
    message_id      TEXT NOT NULL UNIQUE,
    np_pair_id      TEXT NOT NULL,
    settlement_date DATE NOT NULL,
    submitted_by    TEXT NOT NULL,               -- buyer_app | seller_app
    status          TEXT NOT NULL DEFAULT 'received',
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    spec_version    TEXT NOT NULL DEFAULT 'RSF-2.0'
);

-- Claim lines (per-order within a claim)
CREATE TABLE claim_lines (
    id              BIGSERIAL PRIMARY KEY,
    claim_id        BIGINT NOT NULL REFERENCES claims(id),
    network_order_id TEXT NOT NULL,
    amount_paise    BIGINT NOT NULL CHECK (amount_paise > 0),
    commission_paise BIGINT NOT NULL DEFAULT 0,
    bff_paise       BIGINT NOT NULL DEFAULT 0
);

-- Settlement batches (partitioned by settlement_date)
CREATE TABLE settlement_batches (
    id              BIGSERIAL PRIMARY KEY,
    np_pair_id      TEXT NOT NULL,
    settlement_date DATE NOT NULL,
    run_id          TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'matching', 'balance_check', 'instructed', 'settled', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
) PARTITION BY RANGE (settlement_date);

CREATE TABLE settlement_batches_2026 PARTITION OF settlement_batches
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

-- Partial index for open batches
CREATE INDEX idx_batches_open ON settlement_batches (np_pair_id, settlement_date)
    WHERE status = 'open';

-- Match results
CREATE TABLE match_results (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        BIGINT NOT NULL,
    network_order_id TEXT NOT NULL,
    buyer_claim_id  BIGINT REFERENCES claims(id),
    seller_claim_id BIGINT REFERENCES claims(id),
    status          TEXT NOT NULL,               -- matched | unmatched | rejected
    policy_version  TEXT NOT NULL,
    matched_at      TIMESTAMPTZ
);

-- Bank instructions
CREATE TABLE instructions (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        BIGINT NOT NULL,
    from_account    TEXT NOT NULL,
    to_account      TEXT NOT NULL,
    amount_paise    BIGINT NOT NULL CHECK (amount_paise > 0),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'credited', 'failed')),
    sent_at         TIMESTAMPTZ,
    settled_at      TIMESTAMPTZ
);

-- Batch Merkle roots
CREATE TABLE batch_roots (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        BIGINT NOT NULL UNIQUE,
    root_hash       TEXT NOT NULL,
    prev_root_hash  TEXT,                        -- chain per NP pair
    np_pair_id      TEXT NOT NULL,
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Transactional outbox
CREATE TABLE outbox (
    id              BIGSERIAL PRIMARY KEY,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);
CREATE INDEX idx_outbox_pending ON outbox (created_at) WHERE status = 'pending';

-- Indian bank holiday calendar
CREATE TABLE calendar_days (
    date            DATE PRIMARY KEY,
    is_bank_holiday BOOLEAN NOT NULL DEFAULT FALSE,
    description     TEXT
);

-- Audit log (append-only)
CREATE TABLE audit_log (
    id              BIGSERIAL PRIMARY KEY,
    actor           TEXT NOT NULL,
    action          TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    payload         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Prevent any UPDATE/DELETE on core ledger tables
CREATE OR REPLACE FUNCTION prevent_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Direct mutation of % is prohibited. Use reversing entries.', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER no_update_journal_entries
    BEFORE UPDATE OR DELETE ON journal_entries FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

CREATE TRIGGER no_update_journal_transactions
    BEFORE UPDATE OR DELETE ON journal_transactions FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

-- goose Down
DROP TABLE IF EXISTS audit_log, calendar_days, outbox, batch_roots, instructions,
    match_results, settlement_batches, settlement_batches_2026, claim_lines, claims,
    idempotency_keys, journal_entries, journal_entries_2026_10, journal_transactions, accounts CASCADE;
DROP FUNCTION IF EXISTS prevent_mutation CASCADE;
```

Run it:

```powershell
$env:GOOSE_DRIVER="postgres"
$env:GOOSE_DBSTRING="postgres://hisaab:hisaab_dev@localhost:5432/hisaab"
goose -dir db/migrations up
```

---

## Step 6: First Go File — Ledger Core Skeleton

Create `internal/ledger/ledger.go`:

```go
package ledger

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AmountPaise is the canonical monetary type. Always paise (1/100 of INR).
// Never use float64 for money.
type AmountPaise int64

// Direction is the side of a journal entry.
type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// CauseType identifies what caused a posting.
type CauseType string

const (
	CauseClaim       CauseType = "claim"
	CauseInstruction CauseType = "instruction"
	CauseBankMessage CauseType = "bank_message"
	CauseAdjustment  CauseType = "adjustment"
)

// Ledger is the core double-entry journal.
type Ledger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// EntryLine is one side of a double-entry posting.
type EntryLine struct {
	AccountName  string
	Direction    Direction
	AmountPaise  AmountPaise
}

// PostRequest describes an atomic double-entry transaction.
// The sum of all debit entries must equal the sum of all credit entries.
type PostRequest struct {
	IdempotencyKey string
	CauseType      CauseType
	CauseID        int64
	Description    string
	Lines          []EntryLine
	PostedAt       time.Time
}

// Post writes an atomic double-entry transaction.
// Idempotent: calling twice with the same key returns the original result.
// Returns an error if the lines don't balance.
func (l *Ledger) Post(ctx context.Context, req PostRequest) (txID int64, err error) {
	// TODO: implement in M1 sprint
	// 1. Validate lines balance (sum debits == sum credits, all paise > 0)
	// 2. Insert into idempotency_keys with ON CONFLICT check
	// 3. Resolve account IDs from names
	// 4. INSERT journal_transaction
	// 5. INSERT journal_entries (batch)
	// 6. Enforce no-negative constraint on constrained accounts
	// 7. Update idempotency_keys to 'done' with stored response
	panic("not implemented — M1 sprint")
}
```

> **Note:** The `panic("not implemented")` is intentional — it fails loudly in tests, which is the point. Replace it in M1.

---

## Step 7: Spec YAML — Edge Cases

Create `spec/edge-cases.yaml` (partial; fill all 23):

```yaml
# ONDC RSF 2.0 Edge Case Decision Table
# Spec version: NTS10 2.0.0 / BRD dated 2 Aug 2024
# Re-verify before building each scenario

spec_version: "RSF-2.0-BRD-2024-08-02"

edge_cases:
  - id: EC-01
    title: "No submission from one side"
    description: "Collector submits /settle; Seller App does not submit within the window"
    brd_section: "Section 4.2, Edge Case 1"
    given:
      collector_claim: present
      seller_claim: absent
      window_elapsed: true
    expected_outcome: "unmatched; Collector's orders carried forward or rejected per policy"
    error_code: "COUNTERPART_MISSING"
    policy_flags:
      - name: carry_forward_limit
        default: null
        options: [null, "1_day", "2_days", "custom"]
    status: needs_practitioner_review

  - id: EC-12
    title: "Insufficient balance — all-or-nothing"
    description: "Collector's ondc_settlement account balance < matched total"
    brd_section: "Section 4.2, Edge Case 12"
    given:
      matched_orders: [2, 3, 4]
      collector_balance_paise: 15000
      matched_total_paise: 30000
    expected_outcome: "all matched orders rejected; no partial fill (default policy)"
    error_code: "INSUFFICIENT_BALANCE"
    policy_flags:
      - name: partial_fill
        default: false
        options: [true, false]
    status: ambiguous  # BRD does not state whether partial fill is intended
```

---

## Step 8: First Property Test

Create `tests/property/ledger_test.go`:

```go
package property_test

import (
	"testing"

	"pgregory.net/rapid"
)

// TestLedgerBalancesZero verifies invariant 1:
// every transaction sums to zero in paise.
func TestLedgerBalancesZero(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random amount pairs
		amount := rapid.Int64Range(1, 10_000_000).Draw(t, "amount_paise")

		debitSum := amount
		creditSum := amount

		if debitSum != creditSum {
			t.Fatalf("invariant 1 violated: debit %d != credit %d", debitSum, creditSum)
		}
	})
}
```

Run:

```powershell
go test ./tests/property/...
```

---

## Step 9: First ADR

Create `docs/adr/ADR-001-go-vs-typescript.md`:

```markdown
# ADR-001: Go vs TypeScript for Ledger Core

**Status:** Proposed  
**Date:** 2026-10-29  
**Decider:** Ubaid

## Context

The ledger core requires: strong numeric safety (no floats), concurrency 
with deterministic deadlock prevention, native fuzzing, and fast CI.

## Decision

Use Go 1.23 for the ledger core and SA simulator.

**Rationale:**
- Go is explicitly named in Razorpay and OpenFX ledger engineering postings
- Native fuzzing (`go test -fuzz`) is production-ready since Go 1.18
- pgx v5 has first-class Postgres support with prepared statements
- Race detector (`go test -race`) catches concurrency bugs automatically
- goroutines + channels model the async ACK/callback semantics naturally

## Fallback Trigger

If by end of Week 2 the vertical slice (M1.1–1.4 + 3 invariant tests) 
is not working, switch to TypeScript + Kysely for the ledger core.
Update this ADR to SUPERSEDED.

## Consequences

- Learning curve: budget Week 0 for Go ramp (see KICKSTART.md)
- Console stays TypeScript (Next.js 15)
- Python used only for analysis scripts
```

---

## Step 10: Makefile

```makefile
.PHONY: up down migrate migrate-down migrate-status test test-race test-property \
        test-invariants test-scenarios bench lint install-hooks check-real-data \
        check-spec-coverage sqlc seed-calendar verify-merkle security-scan

DB_URL     ?= postgres://hisaab:hisaab_dev@localhost:5432/hisaab
TEST_DB_URL ?= postgres://hisaab:hisaab_dev@localhost:5432/hisaab

# ─── Infrastructure ────────────────────────────────────────────────
up:
	docker compose up -d
	@echo "Waiting for Postgres..."
	@until docker compose exec postgres pg_isready -U hisaab; do sleep 1; done
	$(MAKE) migrate

down:
	docker compose down

# ─── Database ─────────────────────────────────────────────────────
migrate:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations up

migrate-down:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations down

migrate-status:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations status

sqlc:
	sqlc generate

# ─── Tests ────────────────────────────────────────────────────────
test:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -count=1 -timeout=120s ./...

test-race:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -race -count=1 -timeout=120s ./...

test-property:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestProperty" \
	  -rapid.checks=50000 -timeout=300s ./tests/property/...

test-invariants:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestInvariant" \
	  -v -timeout=300s ./tests/...

test-scenarios:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestScenario" \
	  -v -timeout=300s ./tests/scenarios/...

bench:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test ./tests/bench/... \
	  -bench=. -benchmem -run=^$$ -count=3

# ─── Code quality ─────────────────────────────────────────────────
lint:
	golangci-lint run --config=.golangci.yml ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./

check-float:
	@echo "Checking for floats in monetary code..."
	@FOUND=$$(grep -rn "float32\|float64" internal/ledger/ internal/settlement/ internal/recon/ \
	  --include="*.go" | grep -v "_test.go" | grep -v "// RAMP-ONLY" || true); \
	if [ -n "$$FOUND" ]; then echo "FAIL:"; echo "$$FOUND"; exit 1; fi
	@echo "✓ No floats"

# ─── Security ─────────────────────────────────────────────────────
security-scan:
	@echo "Running gosec..."
	gosec ./...
	@echo "Checking real data in fixtures..."
	python3 scripts/check_real_data.py
	@echo "Checking spec coverage..."
	python3 scripts/check_spec_coverage.py

# ─── Compliance ───────────────────────────────────────────────────
check-real-data:
	python3 scripts/check_real_data.py

check-spec-coverage:
	python3 scripts/check_spec_coverage.py

check-spec-drift:
	python3 scripts/check_ondc_spec_drift.py

# ─── Developer setup ──────────────────────────────────────────────
install-hooks:
	bash scripts/install-hooks.sh

setup: install-hooks up
	@echo "✓ Hisaab dev environment ready"
	@echo "  DB: $(DB_URL)"
	@echo "  Run: make test-race"

# ─── CLI commands ─────────────────────────────────────────────────
seed-calendar:
	go run cmd/cli/main.go seed-calendar --year 2026

verify-merkle:
	go run cmd/cli/main.go verify-merkle --batch-id $(BATCH_ID)

generate-report:
	go run cmd/cli/main.go generate-report --output docs/CORRECTNESS-REPORT.md
```

---

## Week 0 Reading List (3h/day)

| Day | Task |
|---|---|
| 1 | tour.golang.org — complete all sections |
| 2 | "Go by Example" (gobyexample.com) — goroutines, channels, errors, testing |
| 3 | pgx v5 docs + write `internal/ledger/ledger.go` with real Postgres connection |
| 4 | sqlc quickstart + generate types for `accounts` and `journal_entries` |
| 5 | Write first property test with `rapid`; understand shrinking |
| 6 | Read Namma Yatri RSF PRs (#16129, #16342) — behavior study, not code copy |
| 7 | Re-read RSF BRD Section 4 (edge cases) + fill 5 more YAML entries in `spec/edge-cases.yaml` |

---

## Week 2 Gate (Non-Negotiable)

By end of Week 2, the following must be true:

1. `go test ./internal/ledger/... -race` passes with zero data race warnings
2. Three invariants have passing tests:
   - Invariant 1: every transaction sums to zero
   - Invariant 3: idempotency (same key = same result under concurrent delivery)
   - Invariant 4: no negative balance on constrained accounts under concurrency
3. One end-to-end `/settle` request (mocked) produces correct ledger entries

If any of these fail, open ADR-001 and update status to FALLBACK. Switch to TypeScript.

---

## Git Setup

```powershell
# Initial commit
git add .
git commit -m "chore: initialize Hisaab repo scaffold

- Directory structure for all modules (M0-M8)
- docker-compose.yml with Postgres 16 + Toxiproxy
- First migration: core ledger schema
- Edge case YAML scaffold (spec/edge-cases.yaml)
- ADR-001: Go vs TypeScript decision
- KICKSTART.md: week-by-week build guide
- Makefile with dev shortcuts"

# Create GitHub repo, then:
git remote add origin https://github.com/<your-handle>/hisaab.git
git push -u origin main
```

---

## Contacts to Reach Before Week 3

Find 3 NP/TSP engineers who have integrated with ONDC RSF. Goal: validate that the pain is real and that Hisaab's correctness focus fills a gap the Workbench doesn't.

**Where to find them:**
- ONDC Discord / Slack (ask in #developers)
- LinkedIn: search "ONDC" + "integration" + "settlement"
- Namma Yatri / Juspay team members who worked on the RSF PRs
- India Fintech Forum communities

**What to ask (3 questions, under 5 minutes):**
1. "When you tested your `/settle` implementation before certification, what tools did you use?"
2. "Did you ever have an order that settled incorrectly or got stuck? How did you debug it?"
3. "Would a tool that lets you run all 23 BRD edge cases offline against a spec-faithful simulator be useful?"

---

_Hisaab. हिसाब. Every rupee accounted for._
