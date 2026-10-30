# Hisaab

Deterministic reconciliation, tax calculation, and double-entry settlement engine for ONDC RSF 2.0 (Reconciliation & Settlement Framework).

Hisaab models the complete lifecycle between Buyer Apps (BAP), Seller Apps (BPP), and Reconciliation & Settlement Providers (RSP): computing Section 393(1) TDS, GST TCS, and finder-fee splits in integer paise, reconciling counterparty payloads, executing T+N banking calendar payouts, and anchoring every journal entry in an append-only Merkle tree.

## Architecture

- `internal/ledger` — Double-entry accounting engine enforcing zero-sum balanced postings and idempotency keys.
- `internal/nptoolkit` — ONDC Network Participant toolkit for RSF 2.0 payload validation, GST/TCS/TDS computation, and schema construction.
- `internal/recon` — Three-way reconciliation matcher comparing BAP, BPP, and collector settlement books.
- `internal/settlement` — Payout orchestrator, partial/full refund clawback netting, and wire dispatch.
- `internal/calendar` — RBI / Indian banking holiday calendar for T+N settlement window resolution.
- `internal/auth` — Beckn Ed25519 / Blake2b HTTP signature generation and verification.
- `internal/audit` — Cryptographic Merkle tree over committed ledger entries.
- `internal/scenariodsl` — Deterministic YAML runner for ONDC RSF 2.0 edge-case scenarios (`spec/scenarios/`).
- `console/` — Next.js operator workbench for inspecting ledger postings, reconciliation diffs, and signed payloads.

## Quickstart

### Prerequisites
- Go 1.22+
- PostgreSQL 16+ (or Docker Compose)
- Node.js 20+ (for the operator workbench)

### Run Stack via Docker
```bash
docker compose up --build
```

### Run Tests & Conformance Suite
```bash
make test
go test -v -race ./internal/... ./tests/...
```

### Execute RSF Edge-Case Scenarios via CLI
```bash
go run ./cmd/cli run-scenario spec/scenarios/ec-01.yaml
```

## Core Ledger Invariants

- **Zero-Sum Conservation:** Every transaction batch $\sum \text{Debits} - \sum \text{Credits} = 0$ in integer paise before commit.
- **Idempotent Posting:** Duplicate Beckn message IDs or settlement reference IDs never double-post or mutate settled entries.
- **Immutable Corrections:** Settled discrepancies and post-settlement refunds are resolved strictly via compensating reversal entries, never `UPDATE` or `DELETE` statements.
- **Cryptographic Auditability:** Every committed journal entry updates the Merkle root in `internal/audit`.
