# Hisaab (हिसाब) — ONDC RSF 2.0 Money-Correctness Engine & Settlement Simulation Laboratory

[![Hisaab CI](https://github.com/notUbaid/hisaab/actions/workflows/ci.yml/badge.svg)](https://github.com/notUbaid/hisaab/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.23.2-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Next.js](https://img.shields.io/badge/Next.js-15.0_App_Router-black?logo=next.js&logoColor=white)](https://nextjs.org)
[![Precision](https://img.shields.io/badge/Precision-Integer_Paise_(int64)-blue)](#zero-float-policy)
[![Calendar](https://img.shields.io/badge/Banking_Calendar-Asia%2FKolkata_(RBI_Gazetted)-orange)](internal/calendar/)

> **ONDC Workbench tests message syntax and protocol flow sequences.**  
> **Hisaab tests the money: guaranteeing every single rupee across decentralized Buyer Apps, Seller Apps, and Logistics Providers is mathematically conserved without rounding evaporation, deadlock, overdrafts, or un-reconciled breaks.**

---

## 1. Why Hisaab? The Syntax vs Money Gap

In centralized e-commerce (e.g. Amazon, Flipkart), a single platform database debits payment instruments, deducts platform commissions, and settles sellers.

In the **Open Network for Digital Commerce (ONDC)**, settlement is decentralized. Financial clearing is governed by Beckn protocol exchanges (`/settle` and `/on_settle`) coordinated through independent **Settlement Agencies (SAs)** operating RBI-regulated Nodal Accounts under Section 4.2 of the Payment and Settlement Systems Act, 2007 (RSF 2.0 / NTS10 2.0.0, BRD dated 2 August 2024).

Multi-party reconciliation across unbundled buyer apps, seller apps, and logistics providers is a recognized industry pain point. While commercial clearing networks like NPCI's NOCS platform and Protean's RSP exist, open tooling that verifies money correctness under concurrency, crashes, and network failure has been scarce.

```
+--------------------------------------------------------------------------------------------------+
| FAILURE MODE                    TRADITIONAL ENGINE BEHAVIOR       HISAAB MATHEMATICAL GUARANTEE   |
+--------------------------------------------------------------------------------------------------+
| 1. Rounding Evaporation         Floats leak 1-3 paise per order.  Integer paise int64 everywhere; |
|                                 Evaporates into suspense pools.   Zero float policy (Inv-5).     |
|                                                                                                  |
| 2. Reversal Overdrafts          Refund debits insolvent seller,   Pre-validates seller balance;   |
|    (EC-16)                      overdrafting nodal escrow pool.   routes insolvent to suspense.   |
|                                                                                                  |
| 3. High-Volume Deadlocks        O(N*M) nested row locking loops   Deterministic lock ordering;    |
|                                 cause DB deadlocks & timeouts.    Ascending lock graph (Inv-2).   |
|                                                                                                  |
| 4. Outbox Dropouts              SA executes bank payouts but      Outbox pattern + idempotent     |
|                                 crashes; replays duplicate cash.  insert-first guard (Inv-3).    |
+--------------------------------------------------------------------------------------------------+
```

---

## 2. System Architecture

```
                             ONDC NETWORK (BECKN PROTOCOL)
                  +--------------------------------------------------+
                  | Buyer App (BAP)               Seller App (BPP)   |
                  +--------------------------------------------------+
                            |                                |
                 POST /settle (Ed25519)            POST /settle (Ed25519)
                            |                                |
                            v                                v
+-----------------------------------------------------------------------------------+
|                           HISAAB SETTLEMENT AGENCY SIMULATOR                      |
|                                                                                   |
|  [ Beckn HTTP Handlers ] ---> Request Digest & Ed25519 Signature Verification     |
|                                                                                   |
|  [ Two-Level Netting ] -----> Level 1: Order Sort-Merge Match                     |
|                               Level 2: NP-Pair Net Instruction (Gross - BFF)      |
|                                                                                   |
|  [ Statutory Tax Engine ] --> Section 393(1) TDS (0.1%) & Section 52 TCS (0.5%)   |
|                                                                                   |
|  [ Refund / Reversal ] -----> Pre-flight Receiver Balance Gate -> Suspense Router |
|                                                                                   |
|  [ Double-Entry Ledger ] ---> Append-Only PostgreSQL / In-Memory (Zero Float)     |
|                               Ascending Account Lock Ordering (Deadlock-Free)     |
|                                                                                   |
|  [ Verification Engine ] ---> Formal Invariant Audits + Adversarial Test Suite    |
+-----------------------------------------------------------------------------------+
           |                                                      |
           v                                                      v
  [ Transactional Outbox ]                              [ Next.js 15 Ops Console ]
  SKIP LOCKED Worker -> POST /on_settle                 UI Prototype & Simulation Studio
```

---

## 3. Financial Invariants & Verification Status

Hisaab enforces formal mathematical invariants across transactions, batches, and settlement cycles:

| Invariant | Description | Verification Method | Status |
| :--- | :--- | :--- | :--- |
| **Inv-1: Global Zero-Sum** | $\sum \text{Debits} - \sum \text{Credits} \equiv 0$ | Rapid property tests + SQL constraint trigger | **Verified** |
| **Inv-2: Ascending Lock Graph** | Accounts locked in ascending ID order | Concurrency test suite | **Verified** |
| **Inv-3: Insert-First Idempotency** | Duplicate keys return cached response or reject | Adversarial suite + Postgres unique index | **Verified** |
| **Inv-4: Constrained Non-Negative** | Constrained account balances never drop $< 0$ | 50-goroutine overdraft test + DB CHECK | **Verified** |
| **Inv-5: Order Conservation** | Total collected = Payout + Fees + Tax + Residual | Adversarial test A3 + Matcher tie-out | **Verified** |
| **Inv-6: Authoritative Refund Cap** | $\sum \text{Refunds} \le \text{SettledForwardAmount}$ | Adversarial tests A4, A5 (authoritative store) | **Verified** |
| **Inv-7: Logistics Separation** | Core retail and logistics claims strictly partitioned | Unit tests (EC-06) | **Verified** |
| **Inv-8: Maker-Checker Dual Control** | Proposer $\neq$ Approver for exceptions | Recon test suite (UI prototype in console) | **Verified (Core)** |
| **Inv-9: RBI Calendar Discipline** | Settlement skips Sundays, 2nd/4th Sats, RBI holidays | Calendar test suite | **Verified** |
| **Inv-10: Batch Hash Chains** | RFC 6962 Merkle tree & sequential batch root chaining | `internal/audit/merkle.go` & CLI `verify-merkle` | **Verified** |
| **Inv-11: Outbox Atomicity** | Outbox event staged atomically with ledger posting | `internal/settlement/orchestrator.go` | **Verified** |
| **Inv-12: Pre-Flight Validation** | Claims with duplicate orders or invalid paise rejected | Adversarial test A1 (`DUPLICATE_ORDER`) | **Verified** |
| **Inv-13: Gross-to-Net Tie-Out** | $\sum \text{NetInstructions} \equiv \sum \text{Gross} - \sum \text{BFF}$ | Matcher invariant checker | **Verified** |
| **Inv-14: Terminal Immutability** | Settled/Rejected state cannot be mutated | State machine transition guards | **Verified** |
| **Inv-15: Zero Real Financial PII** | All identifiers are synthetic test fixtures | `scripts/check_real_data.py` scan | **Verified** |

---

## 4. Current Verification & Audit Reality

Following a comprehensive architecture audit, Hisaab maintains an explicit, verifiable matrix of claims:

| Component | Implementation | Verification Command | Reality & Known Limitations |
| :--- | :--- | :--- | :--- |
| **In-Memory Ledger** | `internal/ledger/memory.go` | `go test -v ./internal/ledger/...` | Production-ready, thread-safe, verified under 50-goroutine concurrent overdrafts. |
| **PostgreSQL Ledger** | `internal/ledger/ledger.go` | `go test -v ./internal/ledger/...` | Schema migration updated; row-level balance checks and atomic idempotency in development. |
| **Adversarial Suite** | `tests/adversarial/` | `go test -v ./tests/adversarial/...` | 11 adversarial tests (A1–A11) verifying duplicate order rejection, tolerance leaks, refund caps, and arithmetic overflow. |
| **Scenario DSL** | `internal/scenariodsl/` | `go test -v ./internal/scenariodsl/...` | Declarative YAML scenario runner validating ONDC BRD edge cases. |
| **Statutory Taxes** | `internal/nptoolkit/tax.go` | `go test -v ./internal/nptoolkit/...` | Section 52 GST TCS (0.5% per Notif 15/2024) and Section 393(1) TDS (0.1% standard, 5% no-PAN). |
| **Ops Console** | `console/` (Next.js 15) | `npm test` (in `console/`) | **UI Prototype**: Rich interactive simulation UI for exploring settlement concepts and YAML scenarios. |

---

## 5. Quickstart Guide

### Prerequisites
- **Go 1.23+**
- **Node.js 20+**
- **Python 3.11+** (for spec checking utilities)

### Run Tests
```bash
# 1. Run core unit tests
go test ./internal/...

# 2. Run adversarial test suite
go test -v ./tests/adversarial/...

# 3. Run property tests
go test -v ./tests/property/...
```

### Build Binaries
```bash
go build -o bin/hisaab-cli.exe ./cmd/cli/main.go
go build -o bin/hisaab-api.exe ./cmd/api/main.go
go build -o bin/hisaab-worker.exe ./cmd/worker/main.go
```

---

## 6. Hisaab CLI Subcommands

```bash
# 1. Execute declarative YAML Scenario and audit financial invariants
bin/hisaab-cli run-scenario spec/scenarios/ec-03.yaml
bin/hisaab-cli run-scenario spec/scenarios/ec-16.yaml

# 2. Print Double-Entry Trial Balance and verify Global Zero-Sum (Inv-1)
bin/hisaab-cli trial-balance

# 3. Inspect RBI banking calendar holidays
bin/hisaab-cli seed-calendar --year 2026

# 4. Generate formal invariant report
bin/hisaab-cli generate-report --output docs/CORRECTNESS-REPORT.md
```

---

## 7. License & Compliance
- **Specification Reference**: ONDC RSF 2.0 / NTS10 2.0.0 (BRD 2 Aug 2024).
- **Regulatory Jurisdiction**: Reserve Bank of India (Payment and Settlement Systems Act, 2007).
- **Tax Law**: Section 393(1) Income Tax Act (TDS) & Section 52 CGST Act (GST TCS).
- **Synthetic Financial Identifiers**: All test fixtures use synthetic bank account numbers, synthetic IFSC codes, and synthetic PANs. Zero real financial PII.
