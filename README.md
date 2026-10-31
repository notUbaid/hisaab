# Hisaab (हिसाब)

Deterministic reconciliation, tax calculation, and double-entry settlement engine for ONDC RSF 2.0 (Reconciliation & Settlement Framework).

Hisaab models the complete lifecycle between Buyer Apps (BAP), Seller Apps (BPP), and Reconciliation & Settlement Providers (RSP): computing Section 393(1) TDS, GST TCS, and finder-fee splits in integer paise, reconciling counterparty payloads, executing T+N banking calendar payouts, and anchoring every journal entry in an append-only Merkle tree.

## Architecture

- `internal/ledger` — Double-entry accounting engine with dual-mode persistence (`SQLStore` for PostgreSQL and `MemoryStore`), ascending lock ordering, non-negative balance checks, and atomic idempotency keys.
- `internal/settlement` — Payout orchestrator, refund clawbacks, wire generation, and enterprise repository persistence (`PostgresRepository` & `MemoryRepository`).
- `internal/recon` — Three-way reconciliation matcher (BAP, BPP, and Nodal Escrow) with PostgreSQL exception queue persistence.
- `internal/nptoolkit` — ONDC Network Participant toolkit for RSF 2.0 payload validation, GST/TCS/TDS computation, and schema construction.
- `internal/crypto` — Field-level envelope encryption (AES-256-GCM + PBKDF2/KMS) for PII and bank details (ADR-002).
- `internal/calendar` — Statutory RBI / Indian banking holiday calendar for T+N settlement window resolution.
- `internal/auth` — Beckn Ed25519 / Blake2b HTTP signature generation and verification.
- `internal/audit` — RFC 6962 domain-separated cryptographic Merkle tree over committed ledger entries.
- `internal/scenariodsl` — Deterministic YAML runner for all 22 ONDC RSF 2.0 edge cases (`spec/scenarios/`).
- `console/` — Next.js 15 App Router operator workbench featuring typed API clients, real-time rate limit backpressure handling, spec ambiguity matrices, and background worker telemetry.

## Quickstart

### Prerequisites
- Go 1.22+
- PostgreSQL 16+ (or Docker Compose)
- Node.js 22+ (for the operator workbench)

### Run Stack via Docker
```bash
docker compose up --build
```

### Run Tests & Conformance Suite
```bash
# Go test suite (ledger, settlement, recon, tax, crypto, scenarios)
go test -count=1 ./...

# PostgreSQL DDL & trigger immutability validation
python scripts/verify_sql_schema.py

# Frontend type-check & test suite
npm run type-check --prefix console
npm test --prefix console
```

### Execute RSF Edge-Case Scenarios via CLI
```bash
go run ./cmd/cli run-scenario spec/scenarios/ec-01.yaml
```

## Core Ledger Invariants

- **Invariant 1: Global Zero-Sum Conservation:** Every transaction batch $\sum \text{Debits} - \sum \text{Credits} = 0$ in integer paise before commit.
- **Invariant 2: Ascending Lock Graph:** Ordered row locking prevents deadlocks during high-concurrency netting runs.
- **Invariant 3: Idempotent Posting:** Duplicate Beckn message IDs or settlement references never double-post or mutate settled entries.
- **Invariant 4: Non-Negative Constrained Balances:** Constrained settlement pools cannot drop below zero.
- **Invariant 5: Order Value Conservation:** All fee components and payouts sum exactly to gross order amounts.
- **Invariant 6: Authoritative Refund Cap:** Cumulative refunds are bounded by forward settled amounts.
- **Invariant 10: Merkle Auditability:** Every committed journal entry updates an RFC 6962 cryptographic tree root.
