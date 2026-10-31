# Hisaab (हिसाब) — Architectural Specification

**Document Version:** 2.0.0  
**Specification:** ONDC RSF 2.0 / NTS10 2.0.0 (Reconciliation & Settlement Framework)  
**Architecture Style:** Modular Monolith with Event-Driven Transactional Outbox  
**Primary Engine:** Go 1.23+ | **Operator Workbench:** Next.js 15 App Router | **Database:** PostgreSQL 16  

---

## 1. System Overview

Hisaab is a deterministic reconciliation, statutory tax calculation, and double-entry settlement engine engineered for multi-party decentralized commerce on the **Open Network for Digital Commerce (ONDC)**.

Unlike centralized e-commerce platforms (e.g., Amazon, Flipkart) where a single platform ledger debits consumer payment instruments, deducts platform commissions, and disburses seller payouts, ONDC unbundles the transaction lifecycle across independent legal entities:
- **Buyer App (BAP)**: Collects funds from consumers into a nodal/escrow pool.
- **Seller App (BPP)**: Disburses payouts to the merchant / Seller on Record (SoR) and retains network commissions.
- **Logistics Service Provider (LSP)**: Transports goods and claims freight fees.
- **Settlement Agency (SA) / RSP**: Reconciles order claims across counterparties, calculates statutory withholding (TDS/TCS), produces multilateral net bank instructions, and anchors an append-only audit trail.

Hisaab provides the reference implementation for both the Settlement Agency (SA) and Network Participant (NP) toolkits.

---

## 2. Component Architecture

```
+-----------------------------------------------------------------------------------+
|                                   HISAAB CORE                                     |
+-----------------------------------------------------------------------------------+
|                                                                                   |
|  [ Beckn Transport & Cryptography ]                                               |
|    internal/auth:       Ed25519 & BLAKE2b-512 Signature & Digest (ONDC standard)   |
|    internal/audit:      RFC 6962 Domain-Separated Merkle Tree & Batch Roots       |
|    cmd/api:             Beckn HTTP Handlers (/settle, /on_settle, /readyz)        |
|                                                                                   |
|  [ Domain Engines ]                                                               |
|    internal/settlement: Two-Level Netting Matcher (Component Parity & Tolerance)  |
|    internal/settlement: Bidirectional Refund Engine (Authoritative Caps)          |
|    internal/settlement: Orchestrator (Atomic Claim-to-Outbox Pipeline)            |
|    internal/nptoolkit:  Statutory Tax Engine (Section 393(1) TDS & Section 52 TCS)|
|    internal/nptoolkit:  NP Claim Builder & Logistics Separation (EC-06)           |
|    internal/recon:      Three-Way Recon Matcher (Claims vs Advice vs Bank UTR)    |
|    internal/calendar:   RBI Banking Calendar, IST Cutoff & T+N Window Resolver    |
|                                                                                   |
|  [ Ledger & Accounting Core ]                                                     |
|    internal/ledger:     PostgreSQL Append-Only Double-Entry Engine                |
|    internal/ledger:     Thread-Safe In-Memory Ledger (Unified Engine Interface)   |
|    internal/ledger:     Deadlock-Free Ascending Lock Graph (Inv-2)                |
|    internal/ledger:     Atomic Inside-Tx Idempotency Keys (Inv-3)                 |
|    internal/ledger:     Lock-Free Non-Negative Account Balance Constraints (Inv-4)|
|                                                                                   |
|  [ Worker & CLI Tooling ]                                                         |
|    cmd/worker:          Transactional Outbox Worker (SKIP LOCKED, Lease, DLQ)     |
|    cmd/cli:             Operational CLI (Scenario Runner, Merkle Verifier)        |
|    console/             Next.js 15 Unified High-Density Operator Workbench        |
+-----------------------------------------------------------------------------------+
```

---

## 3. Financial Invariants

Hisaab enforces 15 formal mathematical invariants across transactions, batches, and settlement cycles:

| Invariant | Name | Mathematical / System Formulation | Enforcement Mechanism |
| :--- | :--- | :--- | :--- |
| **Inv-1** | **Global Zero-Sum** | $\sum \text{Debits} - \sum \text{Credits} \equiv 0$ | SQL deferred trigger `assert_balanced()` + memory ledger checks. |
| **Inv-2** | **Ascending Lock Graph** | $\forall i < j : \text{AccountID}_i < \text{AccountID}_j$ | Deterministic ascending sort before acquiring row updates. Prevents deadlocks. |
| **Inv-3** | **Effectively-Once Idempotency** | $\text{Post}(K, H) \rightarrow R; \ \text{Post}(K, H) \rightarrow R$ | Unique index on `idempotency_keys(key)` committed atomically with entries. |
| **Inv-4** | **Non-Negative Constrained Balances** | $\text{Balance}(\text{account}) \ge 0 \quad \forall t$ | DB CHECK constraint `(NOT constrained OR balance_paise >= 0)` on `account_balances`. |
| **Inv-5** | **Order Value Conservation** | $\text{Gross} = \text{Commission} + \text{BFF} + \text{SoRDue} + \text{Taxes} + \text{Residual}$ | Integer paise arithmetic validation on all order settlement lines. |
| **Inv-6** | **Authoritative Refund Cap** | $\sum \text{Refunds} \le \text{SettledForwardAmount}$ | Bound against authoritative forward settled order record, counting suspense breaks. |
| **Inv-7** | **Logistics Separation** | $\text{Orders}_{\text{Retail}} \cap \text{Orders}_{\text{Logistics}} = \emptyset$ | Distinct claim message partitioning per counterparty pair. |
| **Inv-8** | **Dual Control Maker-Checker** | $\text{Proposer} \neq \text{Approver}$ | Server-side identity validation in reconciliation exception resolution. |
| **Inv-9** | **RBI Calendar Discipline** | $\text{SettleDate} \notin \{\text{Sundays}, \text{2nd/4th Saturdays}, \text{RBI Holidays}\}$ | Versioned statutory holiday repository pinned strictly to `Asia/Kolkata`. |
| **Inv-10**| **Merkle Auditability** | $\text{Root}_B = \text{MerkleTree}(\{\text{Entry}_1, \dots, \text{Entry}_N\})$ | RFC 6962 domain-separated cryptographic hashing (`0x00` leaf, `0x01` internal node). |
| **Inv-11**| **Outbox Atomicity** | $\text{Tx}(\text{LedgerEntries} \cup \text{OutboxEvent})$ | Single database transaction boundary guarantees exactly-once dispatch intent. |
| **Inv-12**| **Pre-Flight Validation** | $\forall o \in \text{Claim} : o.\text{Amount} > 0 \land \text{Unique}(o.\text{ID})$ | Structural validation rejecting duplicate orders with `DUPLICATE_ORDER`. |
| **Inv-13**| **Gross-to-Net Tie-Out** | $\sum \text{NetInstructions} \equiv \sum \text{Gross} - \sum \text{BFF}$ | Matcher invariant checker validating bank payouts against gross dues. |
| **Inv-14**| **Terminal Immutability** | $\text{State} \in \{\text{SETTLED}, \text{REJECTED}\} \implies \text{Terminal}$ | Finite state machine rejecting secondary transitions. |
| **Inv-15**| **Zero Real PII** | $\forall \text{Fixture} : \text{Fixture} \in \text{SyntheticDataPool}$ | Synthetic bank accounts, IFSC codes, and PANs across all test fixtures. |

---

## 4. ONDC RSF 2.0 Spec Ambiguity Register

When the regulatory specification leaves implementation choices unspecified, Hisaab documents the ambiguity, assigns a policy flag, and defaults to the strictest financial posture:

- **AMB-001: Match Semantics (Order ID vs Component Amounts)**
  - *Reference:* BRD §4.2 & Annexure Balance Check Example.
  - *Issue:* Whether counterparty claims match solely on `NetworkOrderID` or require strict component equality.
  - *Policy Flag:* `match_amount_tolerance_paise` (default: `0` paise / strict parity). Any non-zero tolerance residual is routed to `sa:tolerance_adjustment` to prevent monetary leakage.
- **AMB-002: Insufficient Balance Behavior**
  - *Reference:* BRD §4.2 Case 12.
  - *Issue:* When Collector balance is inadequate to fund matched volume, whether to reject the entire batch or perform partial FIFO settlement.
  - *Policy Flag:* `partial_fill_allowed` (default: `false` / all-or-nothing per BRD).
- **AMB-003: Carry-Forward Expiration Horizon**
  - *Reference:* BRD §4.2 Cases 9, 10, 11.
  - *Issue:* Upper bound on retention of unmatched counterparty claims.
  - *Policy Flag:* `carry_forward_days` (default: `2` business days).
- **AMB-004: Cut-Off Window Time & Timezone Authority**
  - *Reference:* BRD §3.1 Settlement Cycle.
  - *Issue:* Ambiguity around exact cutoff time and server clock authority.
  - *Policy Flag:* `cut_off_time_ist` (default: `23:59:59` IST Asia/Kolkata).
- **AMB-005: Tax Withholding Allocation**
  - *Reference:* BRD Annexure §5.
  - *Issue:* Unbundled tax calculation between BAP and BPP.
  - *Policy Flag:* `tax_reconciliation_mode` (default: `explicit_deduction_legs`).
- **AMB-006: Insolvent Seller Refund Clawback**
  - *Reference:* BRD §4.2 Case 16.
  - *Issue:* How to handle consumer refunds when Seller App settlement account has insufficient funds.
  - *Policy Flag:* `insolvent_seller_refund` (default: route to `sa:suspense` without overdrafting nodal pool).

---

## 5. Ledger Data Model & Double-Entry Schema

PostgreSQL 16 partitioned tables enforce relational integrity and immutability:

```sql
-- Chart of accounts with normal balance convention
CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'equity', 'income', 'expense', 'external')),
    currency TEXT NOT NULL DEFAULT 'INR',
    constrained BOOLEAN NOT NULL DEFAULT FALSE,
    normal_balance TEXT NOT NULL CHECK (normal_balance IN ('debit', 'credit')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fast row-level non-negative balance checks without table-scan SUM()
CREATE TABLE account_balances (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id),
    constrained BOOLEAN NOT NULL,
    balance_paise BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT constrained_non_negative CHECK (NOT constrained OR balance_paise >= 0)
);

-- Append-only journal transactions (partitioned by effective date)
CREATE TABLE journal_transactions (
    id BIGSERIAL,
    idempotency_key TEXT NOT NULL,
    description TEXT NOT NULL,
    cause_type TEXT NOT NULL,
    cause_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Double-entry legs with database-enforced zero-sum trigger
CREATE TABLE journal_entries (
    id BIGSERIAL,
    transaction_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id),
    direction TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount_paise BIGINT NOT NULL CHECK (amount_paise > 0 AND amount_paise <= 10000000000000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Database Trigger Enforcing Double-Entry Zero-Sum
CREATE CONSTRAINT TRIGGER journal_entries_balanced
    AFTER INSERT ON journal_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION assert_balanced();

-- Database Trigger Preventing TRUNCATE on Core Accounting Tables
CREATE TRIGGER journal_entries_no_truncate
    BEFORE TRUNCATE ON journal_entries
    FOR EACH STATEMENT EXECUTE FUNCTION no_truncate();
```

---

## 6. Mathematical Foundations & Statutory Taxes

### 6.1. Integer Micro-Unit Banker's Rounding
To eliminate half-paise rounding leaks on odd-paise amounts:
1. Taxes are computed in micro-units ($10^{-4}$ INR or basis-point integer units).
2. Deterministic Banker's Rounding (**RoundHalfEven**) resolves fractional values before debit/credit journal line generation.
3. Conservation guarantees that $\sum \text{Deductions} + \text{NetPayout} \equiv \text{GrossAmount}$.

### 6.2. Statutory Rates (October 2026 Reality)
- **GST TCS**: 0.5% (0.25% CGST + 0.25% SGST) under Section 52 of CGST Act 2017 (per CGST Notification 15/2024 dated 10 July 2024).
- **E-Commerce TDS**: 0.1% under Section 393(1) Table Sl. 8(v) of Income-tax Act 2025 (Payment Code 1035). Rate increases to 5.0% for sellers without PAN/Aadhaar.

---

## 7. Enterprise Persistence Architecture

To guarantee consistency across separated micro-processes (`cmd/api` and `cmd/worker`), Hisaab provides dual-mode transactional persistence via formal repository interfaces:

### 7.1. Ledger Store (`internal/ledger/store.go`)
- **`SQLStore` (PostgreSQL)**: Manages atomic double-entry postings (`PostEntriesTx`), account balance lookups with `SELECT ... FOR UPDATE` row locks, ascending lock ordering, idempotency validation, and transactional integrity.
- **`MemoryStore` (Thread-Safe In-Memory)**: Unified high-throughput in-memory store guarded by sync primitives, ensuring unit test suites run in under 200ms without database overhead.

### 7.2. Settlement Repository (`internal/settlement/repository.go`)
- **`PostgresRepository`**: Fully persists incoming RSF claims, claim line items, settlement batches, netting instructions (`instructions` table), and RFC 6962 signed batch roots (`batch_roots` table).
- **Restart Recovery**: Restores batch execution state, active outbox events, and uncommitted netting cycles across container restarts.

### 7.3. Reconciliation Exception Repository (`internal/recon/repository.go`)
- **`PostgresRepository`**: Persists 3-way reconciliation exceptions (`recon_exceptions` table), discrepancy classification, status lifecycle (`OPEN`, `RESOLVED`, `ESCALATED`), and dual-control audit trails (`ResolveException`).

---

## 8. Security, Rate Limiting & Field-Level Encryption

### 8.1. Envelope Field Encryption (ADR-002)
- Implemented in `internal/crypto/encryption.go`.
- Encrypts PII, bank account numbers, and IFSC codes using **AES-256-GCM** with 96-bit random nonces and authenticated data tags.
- Uses master key envelope encryption with key identifiers (`kid`) supporting non-destructive key rotation.

### 8.2. Token-Bucket Rate Limiting (ADR-003)
- Implemented as HTTP middleware on `cmd/api`.
- Enforces participant-level quotas (default 60 requests/minute per authenticated participant ID).
- Emits RFC-compliant headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.
- When quota is exhausted, returns HTTP 429 with backpressure telemetry, tripping the frontend circuit breaker to prevent cascading nodal failures.

---

## 9. Next.js 15 Operator Workbench Architecture

The operator console (`console/`) delivers an institutional, high-density fintech command center:
- **Design System & Visual Tokens**: Deep obsidian background (`#090A0F`), hairline borders (`rgba(255,255,255,0.07)`), bone-white accents (`#FAF9F6`), maroon alert badges (`#7A1C24`), and tabular figures.
- **Unified Typed API Client (`console/lib/apiClient.ts`)**: Enforces typed RPCs, automatic role header propagation (`X-Hisaab-Role`), circuit-breaker backpressure handling, and error normalization.
- **Global Resilience**: Root layout wrapped in `<ToastProvider>` for real-time operational feedback and `<ErrorBoundary>` with component tree recovery.
- **Spec & Ambiguity Inspector (`console/components/SpecInspector.tsx`)**: Live matrix mapping regulatory edge cases to pinned upstream ONDC commit SHAs and policy flags.
- **Worker Queue Monitor (`console/components/WorkerMonitor.tsx`)**: Real-time outbox telemetry monitoring queue depth, lease health, and dispatch latency.

