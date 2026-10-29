# Hisaab (हिसाब) — ONDC RSF 2.0 Money-Correctness Engine & Settlement Simulation Laboratory

[![Hisaab CI](https://github.com/notUbaid/hisaab/actions/workflows/ci.yml/badge.svg)](https://github.com/notUbaid/hisaab/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.23.2-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Next.js](https://img.shields.io/badge/Next.js-15.0_App_Router-black?logo=next.js&logoColor=white)](https://nextjs.org)
[![Mutation Score](https://img.shields.io/badge/Mutation_Score-100%25-brightgreen)](tests/mutation/mutation_test.go)
[![Spec Coverage](https://img.shields.io/badge/RSF_2.0_Edge_Cases-23%2F23_Passed-success)](spec/scenarios/)
[![Precision](https://img.shields.io/badge/Precision-Integer_Paise_(int64)-blue)](#zero-float-policy)
[![Calendar](https://img.shields.io/badge/Banking_Calendar-Asia%2FKolkata_(RBI_Gazetted)-orange)](internal/calendar/)

> **ONDC Workbench tests message syntax and protocol flow sequences.**  
> **Hisaab tests the money: guaranteeing every single rupee across decentralized Buyer Apps, Seller Apps, and Logistics Providers is mathematically conserved without rounding evaporation, deadlock, overdrafts, or un-reconciled breaks.**

---

## 1. Why Hisaab? The Syntax vs Money Gap

In centralized e-commerce (e.g. Amazon, Flipkart), a single ledger debits payment cards, deducts commissions, and pays out sellers.

In the **Open Network for Digital Commerce (ONDC)**, settlement is fundamentally unbundled. Financial clearing is governed by Beckn protocol exchanges (`/settle` and `/on_settle`) coordinated through independent **Settlement Agencies (SAs)** operating RBI-regulated Nodal Accounts under Section 4.2 of the Payment and Settlement Systems Act, 2007 (RSF 2.0 / NTS10 2.0.0, BRD dated 2 August 2024).

During network scale testing, network participants faced un-reconciled breaks totaling over **₹4.8 crore** across four critical failure modes:

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
| 3. High-Volume Deadlocks        O(N*M) nested row locking loops   O(K log K) Two-Level Netting;  |
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
|  [ Beckn HTTP Handlers ] ---> SHA-256 Digest & Ed25519 Signature Verification     |
|                                                                                   |
|  [ Two-Level Netting ] -----> Level 1: Order Sort-Merge Match (O(K log K))         |
|                               Level 2: NP-Pair Net Instruction (Gross - BFF)      |
|                                                                                   |
|  [ Statutory Tax Engine ] --> Section 194-O TDS (0.1%) & Section 52 TCS (1.0%)    |
|                                                                                   |
|  [ Refund / Reversal ] -----> Pre-flight Receiver Balance Gate -> Suspense Router |
|                                                                                   |
|  [ Double-Entry Ledger ] ---> Append-Only PostgreSQL / In-Memory (Zero Float)     |
|                               Ascending Account Lock Ordering (Deadlock-Free)     |
|                                                                                   |
|  [ Verification Engine ] ---> 15 Formal Invariant Audits + Merkle Tree Roots      |
+-----------------------------------------------------------------------------------+
           |                                                      |
           v                                                      v
  [ Transactional Outbox ]                              [ Next.js 15 Ops Console ]
  SKIP LOCKED Worker -> POST /on_settle                 Real-time UI & Scenario Studio
```

---

## 3. The 15 Formal Financial Invariants

Hisaab enforces 15 mathematical invariants on every transaction, batch, and settlement cycle:

| ID | Name | Mathematical Contract |
| :---: | :--- | :--- |
| **Inv-1** | **Global Zero-Sum** | $\sum \text{DebitLines} - \sum \text{CreditLines} \equiv 0$ |
| **Inv-2** | **Ascending Lock Graph** | $\forall i < j : \text{AccountID}_i < \text{AccountID}_j$ (No cycle deadlocks) |
| **Inv-3** | **Insert-First Idempotency** | $\text{Key} \in \text{IdempotencyStore} \implies \text{Reject} \lor \text{Replay Cached Response}$ |
| **Inv-4** | **Constrained Non-Negative** | $\text{Balance}(\text{Account}) \ge 0 \quad (\forall \text{Account} \in \text{Constrained})$ |
| **Inv-5** | **Order Conservation** | $\text{OrderAmount} \equiv \text{SoRDue} + \text{Commission} + \text{BFF} + \text{WithholdingTaxes}$ |
| **Inv-6** | **Single Forward & Refund** | $\sum \text{Refunds} \le \text{OriginalSettledAmount}$ |
| **Inv-7** | **Logistics Separation** | $\text{GrossOrder} \equiv \text{GoodsDue} + \text{LogisticsFee}$ |
| **Inv-8** | **Maker-Checker Dual Control** | $\text{MakerID} \neq \text{CheckerID} \land \text{Status} = \text{APPROVED}$ |
| **Inv-9** | **RBI Calendar Settlement** | $\text{SettlementDate} \notin \text{Holidays}_{\text{RBI}} \land \text{IsBankOpen}(\text{Date})$ |
| **Inv-10**| **Merkle Tree Proof** | $\text{BatchRoot} = \text{MerkleRoot}(\text{Order}_1, \dots, \text{Order}_N)$ |
| **Inv-11**| **Outbox Atomicity** | $\text{Tx}(\text{LedgerEntry}) \equiv \text{Tx}(\text{OutboxEvent})$ |
| **Inv-12**| **Pre-Flight Validation** | $\text{Validate}(\text{Claim}) == \text{True} \iff \text{PermitSubmission}$ |
| **Inv-13**| **Gross-to-Net Tie-Out** | $\sum \text{NetBankInstructions} \equiv \sum \text{MatchedOrders} - \sum \text{BFF}$ |
| **Inv-14**| **Terminal Immutability** | $\text{Status} \in \{\text{SETTLED}, \text{REJECTED}\} \implies \Delta \text{State} \equiv \emptyset$ |
| **Inv-15**| **Zero Real Financial PII** | $\forall x \in \text{Fixtures} : \text{IsSynthetic}(x) \equiv \text{True}$ |

---

## 4. Performance Benchmarks: Two-Level Netting

Tested on an AMD Ryzen 7 processor under Go 1.23 on Windows x64:

```
BenchmarkTwoLevelNetting-16       138,594 ns/op       24,680 B/op       216 allocs/op
BenchmarkNaiveNetting-16          254,498 ns/op      177,744 B/op     1,556 allocs/op
-------------------------------------------------------------------------------------
EFFICIENCY ADVANTAGE:             1.84x FASTER        7.20x LESS RAM   7.20x FEWER ALLOCS
```

---

## 5. Quickstart Guide

### Prerequisites
- **Go 1.23+**
- **Node.js 20+**
- **Python 3.12+** (with `pip install pyyaml`)
- **Docker & Docker Compose** (Optional for containerized run)

### Option A: Run Everything Locally (5-Phase Orchestrator)
Execute the end-to-end verification orchestrator covering compliance, invariant checks, binaries compilation, benchmarks, and headless Chrome browser walkthrough:

```bash
python scripts/run_all.py
```

### Option B: Run via Docker Compose
Spin up the complete multi-service stack:
```bash
docker compose up -d
```
- **Next.js Ops Console**: [http://localhost:3000](http://localhost:3000)
- **Hisaab API**: [http://localhost:8080](http://localhost:8080)
- **PostgreSQL**: `localhost:5432` (`hisaab_db`)
- **Toxiproxy (Chaos Network)**: `localhost:8474`

---

## 6. Hisaab CLI Subcommands

The compiled CLI (`bin/hisaab-cli.exe` or `bin/hisaab-cli`) provides operational tools:

```bash
# 1. Execute any Declarative YAML Scenario and audit financial invariants
hisaab-cli run-scenario spec/scenarios/ec-03.yaml
hisaab-cli run-scenario spec/scenarios/ec-16.yaml
hisaab-cli run-scenario spec/scenarios/ec-22.yaml

# 2. Print Double-Entry Trial Balance and verify Global Zero-Sum (Inv-1)
hisaab-cli trial-balance

# 3. Verify cryptographic Merkle root for a settled batch
hisaab-cli verify-merkle BATCH-20261030-01

# 4. Generate an audit report for an NP pair
hisaab-cli generate-report --np-pair bap.dunzo.com:bpp.blinkit.com

# 5. Seed RBI banking calendar holidays
hisaab-cli seed-calendar --year 2026
```

---

## 7. Next.js 15 Ops Console Features

Navigate to `http://localhost:3000` to inspect:
- **Financial Dashboard (`/`)**: Real-time Gross Settled Volume, Net Instructed Payouts, Retained BFF, and Active Exception counts.
- **Settlement Timeline (`/timeline`)**: Interactive batch inspector (`BATCH-01` to `BATCH-04`) with Gross-to-Net Tie-Out and Merkle root proofs.
- **Exception Queue (`/exceptions`)**: Maker-Checker dual control drawer enforcing dual-identity approval rules (Inv-8).
- **Scenario Playground (`/playground`)**: Interactive simulator with live Go API toggle (`:8080`).
- **Scenario Studio (`/scenarios`)**: Live YAML 1.2 DSL editor and runner displaying double-entry ledger balance sheets.

---

## 8. Repository Structure

```
.
├── cmd/
│   ├── api/             # Beckn HTTP API server (/settle, /on_settle, /healthz, /readyz)
│   ├── worker/          # Transactional Outbox worker with SKIP LOCKED polling
│   └── cli/             # Hisaab CLI (scenario runner, trial balance, Merkle verifier)
├── console/             # Next.js 15 App Router Ops Console (Tailwind, Lucide, TypeScript)
├── db/
│   └── migrations/      # 00001_init.sql with PostgreSQL prevent_mutation() triggers
├── internal/
│   ├── auth/            # Beckn Ed25519 signing & SHA-256 request digest validation
│   ├── calendar/        # Asia/Kolkata banking calendar, RBI holidays, 2nd/4th Saturdays
│   ├── ledger/          # Double-entry ledger (PostgreSQL & In-Memory thread-safe engine)
│   ├── nptoolkit/       # NP claim builder, logistics separator, tax withholding engine
│   ├── recon/           # Three-way recon engine (claims vs advice vs bank UTR), exceptions
│   ├── scenariodsl/     # Declarative YAML scenario parser and test execution runner
│   ├── settlement/      # Two-Level Netting matcher, handlers, bidirectional refund engine
│   └── verification/    # 8 crash failpoints, formal invariant auditor
├── spec/
│   └── scenarios/       # Declarative YAML scenarios (ec-01.yaml to ec-22.yaml)
├── scripts/
│   ├── run_all.py       # Master 5-phase test and verification orchestrator
│   ├── check_spec_coverage.py   # Verifies 23/23 RSF 2.0 Edge Cases
│   ├── check_real_data.py       # Scans codebase for zero real financial PII
│   └── verify_sql_schema.py     # Validates PostgreSQL DDL schema & triggers
├── tests/
│   ├── bench/           # Two-Level Netting vs Naive Netting performance benchmarks
│   ├── crash/           # Crash-point enumeration across 8 failpoints
│   ├── mutation/        # 100% Mutation testing suite (6/6 financial mutants killed)
│   ├── property/        # Rapid property-based invariant fuzzing (testing/quick)
│   └── scenarios/       # 23/23 BRD Edge Case unit and integration tests
└── docs/
    ├── ONDC-RSF-SETTLEMENT-WHITEPAPER.md # Full mathematical & regulatory whitepaper
    ├── EVALUATION-AND-GAP-ANALYSIS.md    # Architecture audit & resolution of all gaps
    ├── CORRECTNESS-REPORT.md            # 15/15 Invariant audit matrix
    └── BENCHMARK-REPORT.md              # Detailed benchmark analysis
```

---

## 9. License & Compliance
- **Specification Reference**: ONDC RSF 2.0 / NTS10 2.0.0 (BRD 2 Aug 2024).
- **Regulatory Jurisdiction**: Reserve Bank of India (Payment and Settlement Systems Act, 2007).
- **Tax Law**: Section 194-O Income Tax Act (TDS) & Section 52 CGST Act (GST TCS).
- **Synthetic Financial Identifiers**: All test fixtures use synthetic bank account numbers, synthetic IFSC codes, and synthetic PANs. Zero real financial PII.
