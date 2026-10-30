# Acceptance Gates — Hisaab Audit & Roadmap Remediation

## Phase 0: Stop the Bleeding (Claims Honesty & Adversarial Baseline)
- [x] Gate 0.1: Companion adversarial test suite `tests/adversarial/adversarial_test.go` exists and faithfully reproduces tests A1–A11.
- [x] Gate 0.2: Remove or relabel false claims C1–C9 in README.md, whitepaper, and codebase (Merkle proof, mutation score, spec coverage, ₹4.8 crore citation, benchmark numbers, console label, Postgres status).
- [x] Gate 0.3: Credibility cleanup: remove dummy init in `cmd/api/main.go`, relocate PRDs to `docs/design/`, clean session artifacts, fix module badge path, mark ADR-002/ADR-003 as Proposed.

## Phase 1: Tier 1 Money-Correctness Bugs & Domain Rates
- [x] Gate 1.1 (B1): `ValidateClaimPayload` and `Matcher.MatchAndNet` reject/dedup duplicate `NetworkOrderID`, ensuring total instructed <= total collected from buyer. Reconcile flags duplicate credit lines.
- [x] Gate 1.2 (B2): Non-zero tolerance preserves component parity (Commission & BFF) and accounts for residual paise in `sa:tolerance_adjustment`, preserving Invariant 5 and Invariant 13.
- [x] Gate 1.3 (B3): `RefundProcessor` validates against authoritative settled amounts, counts suspense refunds toward cumulative caps, registers `sa:suspense` in `SeedStandardAccounts`, and protects state with mutex.
- [x] Gate 1.4 (B5): `ValidateLines` prevents integer overflow on 64-bit addition and caps line amounts. `ComputeRequestHash` uses unambiguous delimiters to prevent collision. Internal hash generation in `Post`.
- [x] Gate 1.5 (B6): HTTP handlers enforce `http.MaxBytesReader` and context timeouts.
- [x] Gate 1.6 (D1/D2): Tax engine rates updated to GST TCS 0.5% (CGST Notif 15/2024) and Section 393(1) TDS 0.1% / 5% no-PAN.
- [x] Gate 1.7: All 11 adversarial tests in `tests/adversarial/adversarial_test.go` pass (100% green).

## Phase 2: PostgreSQL Ledger & Infrastructure Realization
- [x] Gate 2.1 (S1): Fix partitioned table unique constraint in `00001_init.sql` (`settlement_batches` composite unique + `batch_runs` table). Update `db/schema.sql`.
- [x] Gate 2.2 (S2): Prevent 2027 partition cliff by adding `DEFAULT` partitions for `journal_entries` and `settlement_batches`.
- [x] Gate 2.3 (S3/S4): Database-side double-entry check trigger (`assert_balanced()`) and `BEFORE TRUNCATE` statement triggers.
- [x] Gate 2.4 (S5): Chart of accounts `normal_balance` semantics and proper trial balance calculation.
- [x] Gate 2.5 (B4): Atomic `Ledger.Post` in `internal/ledger/ledger.go` with lock-free non-negative `account_balances` CHECK constraint in ascending `account_id` order, atomic inside-tx idempotency, and `DBTX` support.
- [x] Gate 2.6 (U2): Define unified `Engine` interface implemented by both in-memory and PostgreSQL ledgers.

## Phase 3: The Spine — End-to-End Orchestrator & Integrations
- [x] Gate 3.1 (U1): Build `Orchestrator` in `internal/settlement/orchestrator.go` uniting claim ingestion, counterpart detection, matching, tax withholding, double-entry ledger posting, net instructions, and outbox staging.
- [x] Gate 3.2 (S7): Outbox worker hardening with two-phase lease (`status = 'processing'`, `locked_until`), outside-transaction dispatch, and exponential backoff retry with DLQ routing.
- [x] Gate 3.3 (D3): ONDC standard Ed25519 + BLAKE2b-512 request signing with `(created) (expires) digest`, in-memory signature replay cache, TTL bounds checking, and future timestamp guards.
- [x] Gate 3.4 (D4): Wire format decimal-to-paise string adapter without floats, rejecting >2 decimals or negatives.
- [x] Gate 3.5 (D5): Multi-year statutory RBI banking calendar (2024–2027) with official notification URLs, thread-safe access, fail-closed `ErrYearNotConfigured`, and cut-off boundary handling (T-1s, T, T+1s).
- [x] Gate 3.6 (S6 & U9): Backend storage readiness probe on `/readyz` and read endpoints (`GET /v1/batches`, `/v1/batches/{id}`, `/v1/trial-balance`, `/v1/exceptions`).

## Phase 4: Proof, Real Verification & Test Vectors
- [x] Gate 4.1 (U3): Real crash testing across 8 failpoints in `tests/crash/crash_test.go` verifying transaction atomicity, 0 partial entries on crash, and idempotent replay recovery.
- [x] Gate 4.2 (C4): 23/23 BRD edge case scenarios in `tests/scenarios/scenarios_test.go` rewritten with real, non-trivial behavioral assertions.
- [x] Gate 4.3 (U6): RFC 6962 domain-separated tamper-evident Merkle tree with inclusion proofs and sequential batch root chaining in `internal/audit/merkle.go` and CLI `verify-merkle`.
- [x] Gate 4.4 (U2): Conformance test suite `ledgertest.Run(t, factory)` testing idempotent replay, payload mismatch, concurrent overdraft, unbalanced rejection, zero/negative rejection, and 2027-dated posting.
- [x] Gate 4.5 (U8): Effective-dated statutory rules engine in `internal/nptoolkit/rules.go` with golden tests at 10 Jul 2024, 1 Oct 2024, and 1 Apr 2026.
- [x] Gate 4.6 (U7): Declarative scenario YAML vectors in `spec/scenarios/*.yaml` passing with exact invariant conformance via CLI scenario runner.
- [x] Gate 4.7: Master test runner `scripts/run_tests.py` running all 16 test packages with 100% PASS (0 failures). Production binaries built: `bin/hisaab-api.exe`, `bin/hisaab-worker.exe`, `bin/hisaab-cli.exe`. Console build passing with 0 errors.
