# Hisaab (हिसाब) — Developer Quickstart Guide

This guide walks you through setting up, testing, running, and deploying Hisaab locally or in containerized environments.

---

## 1. System Requirements

- **Go**: Version 1.23.2 or later
- **Node.js**: Version 20+ (with `npm`)
- **Python**: Version 3.12+ (with `pyyaml` installed via `pip install pyyaml`)
- **Docker & Docker Compose**: Optional, for multi-container orchestration with PostgreSQL and Toxiproxy.

---

## 2. End-to-End Orchestrated Verification

The fastest way to verify that your local environment is correctly configured and all invariant suites pass is running the master orchestrator:

```bash
python scripts/run_all.py
```

This runs 5 automated phases in sequence:
1. **Compliance & Spec Coverage**: Verifies 23/23 RSF 2.0 Edge Cases, Zero Real Financial PII, and PostgreSQL DDL triggers.
2. **Core Ledger & Invariant Verification**: Runs all 13 Go test suites (including M1 ledger, M2 matching & refunds, M3 tax withholding, M4 recon, M5 failpoints, and M5.8 mutation testing with 100% kill score).
3. **Binaries & Benchmarks**: Compiles `hisaab-api.exe`, `hisaab-worker.exe`, and `hisaab-cli.exe`, and executes the Two-Level Netting benchmarks.
4. **Next.js 15 Ops Console**: Runs TypeScript type checking and builds the production SSR application.
5. **Physical Browser Walkthrough**: Boots the backend and frontend servers, executes a headless Playwright Chrome session, and captures UI screenshot artifacts.

---

## 3. Running Services Locally

### Step 1: Start the Go Settlement API Server
```bash
go run ./cmd/api
# Or run pre-compiled binary:
./bin/hisaab-api
```
Server listens on `http://127.0.0.1:8080`.
- Health probe: `GET http://127.0.0.1:8080/healthz`
- Readiness probe: `GET http://127.0.0.1:8080/readyz`
- Beckn Settle handler: `POST http://127.0.0.1:8080/settle`

### Step 2: Start the Transactional Outbox Worker
```bash
go run ./cmd/worker
# Or run pre-compiled binary:
./bin/hisaab-worker
```
Polls `outbox` table using `FOR UPDATE SKIP LOCKED` and delivers signed `/on_settle` callbacks to network participants.

### Step 3: Start the Next.js 15 Ops Console
```bash
cd console
npm install
npm run dev
```
Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## 4. Running via Docker Compose

To launch the full production topology (PostgreSQL 16, Hisaab API, Outbox Worker, Next.js Console, and Toxiproxy):

```bash
docker compose up -d
```

Verify service status:
```bash
docker compose ps
```

---

## 5. Working with Declarative Scenarios (Scenario DSL)

Hisaab allows you to test settlement scenarios without writing code using declarative YAML files in `spec/scenarios/`.

### Run a scenario via CLI:
```bash
./bin/hisaab-cli run-scenario spec/scenarios/ec-03.yaml
```

Output:
```
========================================================
  SCENARIO AUDIT: EC-03 (Both Buyer App and Seller App submit identical order sets)
========================================================
  [INIT] Executing Scenario EC-03: Both Buyer App and Seller App submit identical order sets
  [SPEC] Citation: Section 4.2, Case 3 (Spec: RSF-2.0-BRD-2024-08-02)
  [LEDGER] Initial balance funded: 5000000 paise for collector:ondc_settlement
  [CLAIM] Buyer Claim parsed: 4 orders
  [CLAIM] Seller Claim parsed: 4 orders
  [MATCH] Matched: 4 orders, Unmatched: 0 orders, Net Instructions: 1
  [LEDGER_POST] Transferred 396000 paise from collector:ondc_settlement to receiver:ondc_settlement
  [INVARIANT_13] Gross SoR sum (384000 paise) ties out to Net Bank Instruction (396000 paise)
  [FINAL_BALANCE] collector:ondc_settlement final balance: 4604000 paise
  [SUCCESS] Scenario EC-03 completed successfully with 100% invariant conformance

Invariants Verified:
  [OK] Invariant 5: Order Conservation (Paise Integrity)
  [OK] Invariant 13: Gross-to-Net Tie-Out
  [OK] Invariant 1: Global Zero-Sum Debits == Credits
  [OK] Invariant 4: Constrained Account Non-Negative Balance

Outcome: PASSED (100% Invariant Compliant)
```

---

## 6. Running Individual Tests

```bash
# Run unit tests for double-entry ledger:
go test -v ./internal/ledger/...

# Run Indian banking calendar tests:
go test -v ./internal/calendar/...

# Run Two-Level Netting and Refund tests:
go test -v ./internal/settlement/...

# Run statutory tax withholding tests:
go test -v ./internal/nptoolkit/...

# Run formal mutation testing harness:
go test -v ./tests/mutation/...

# Run performance benchmarks:
go test -v -bench=. -benchmem ./tests/bench/...
```
