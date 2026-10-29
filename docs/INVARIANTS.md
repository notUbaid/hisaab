# Hisaab: Ledger Invariants Specification

These 15 invariants are the core foundation of Hisaab. Each invariant is formal, testable, and attacked by dedicated failure-injection test suites.

---

### Invariant 1: Zero-Sum Conservation
**Rule:** Every journal transaction sums to exactly zero across all entries in INR paise.
$$\sum \text{Debits} - \sum \text{Credits} = 0$$
- **Enforcement:** Enforced in database stored posting function, plus deferred check constraint on `journal_entries`.
- **Target Attack:** Split transactions, partial inserts, off-by-one roundings.

### Invariant 2: Append-Only Immutability
**Rule:** The journal contains no `UPDATE` or `DELETE` operations. Corrections are made strictly via reversing journal entries.
- **Enforcement:** PostgreSQL row-level triggers reject mutations on `journal_entries` and `journal_transactions`.
- **Target Attack:** Malicious/accidental SQL updates, retro-dated adjustments.

### Invariant 3: Effectively-Once Idempotency
**Rule:** One idempotency key produces at most one set of postings, regardless of duplicate deliveries.
- **Enforcement:** Insert-first unique index on `idempotency_keys` with SHA-256 request payload hash.
- **Target Attack:** Concurrent duplicate `/settle` and `/on_settle` deliveries.

### Invariant 4: Non-Negative Constrained Balances
**Rule:** Constrained accounts (e.g. `ondc_settlement`, `escrow`) never have a negative balance at transaction commit.
$$\text{Balance}(\text{account}) \ge 0 \quad \forall \ t$$
- **Enforcement:** Account row locks acquired in ascending account-id order; pre-posting balance assertion.
- **Target Attack:** Concurrent debit race condition draining an account beyond available balance.

### Invariant 5: Order Value Conservation
**Rule:** An order's total settled amount equals the exact sum of its commission, Seller on Record due, Buyer Finder Fee (BFF), and statutory taxes.
$$\text{OrderTotal} = \text{Commission} + \text{SoRDue} + \text{BFF} + \text{Taxes}$$
- **Enforcement:** Arithmetic assertion on order sub-ledger legs prior to batch net posting.
- **Target Attack:** Component discrepancy in submitted claim lines.

### Invariant 6: Single Forward Settlement
**Rule:** A network order settles at most once in the forward direction and at most once in reverse (refund).
- **Enforcement:** Unique constraint on `(network_order_id, direction)` across successful match records.
- **Target Attack:** Re-submitting settled orders in subsequent day batches.

### Invariant 7: Policy-Faithful Instruction
**Rule:** The SA simulator instructs money transfers exclusively for orders matched according to the active settlement policy.
- **Enforcement:** Foreign key linking instructions to verified `match_results`.
- **Target Attack:** Instruction generation on partial or unverified matches.

### Invariant 8: Crash Recovery Atomicity
**Rule:** Following any mid-batch crash, restart, or network severance, every settlement batch is either completely applied or cleanly rolled back.
- **Enforcement:** ACID PostgreSQL transaction boundaries and transactional outbox.
- **Target Attack:** SIGKILL / process crash between order sub-ledger creation and net batch instruction.

### Invariant 9: Recon Completeness (No Orphan Rupee)
**Rule:** Every line in the SA settlement report and bank statement is reconciled to exactly one order outcome (Settled, Exception, or Suspense).
- **Enforcement:** Three-way reconciliation engine assertion.
- **Target Attack:** Missing bank statement lines, unrecorded bank charges.

### Invariant 10: Deterministic Execution
**Rule:** Given identical initial state, seed, and input claims, the resulting ledger journal is bitwise identical.
- **Enforcement:** Monotonic sorting of orders, accounts, and events prior to processing.
- **Target Attack:** Hash-order iteration nondeterminism in Go maps.

### Invariant 11: Real-Time Trial Balance Equilibrium
**Rule:** The aggregate balance across all asset, liability, equity, revenue, and expense accounts in the chart of accounts is zero at all times.
$$\sum_{\text{all accounts}} \text{Balance} = 0$$
- **Enforcement:** Automated trial balance background verifier job.
- **Target Attack:** Orphan postings or unlinked journal transactions.

### Invariant 12: Cryptographic Audit Traceability
**Rule:** Every posting references an immutable cause (claim ID, instruction ID, or bank message ID).
- **Enforcement:** `NOT NULL` foreign keys for `cause_type` and `cause_id`.
- **Target Attack:** Unattributed adjustments injected into the journal.

### Invariant 13: Gross-to-Net Tie-Out
**Rule:** For every batch, the sum of gross order sub-ledger obligations equals the net bank instruction amounts across all participants.
- **Enforcement:** Dual-level verification step prior to batch closure.
- **Target Attack:** Dropped rounding paise during multi-party netting.

### Invariant 14: Terminal Callback Integrity
**Rule:** Each `/settle` message reaches at most one terminal callback state per `message_id`. Late, out-of-order, or duplicate callbacks cannot alter terminal state.
- **Enforcement:** Finite State Machine transition guards in settlement engine.
- **Target Attack:** Out-of-order bank callbacks arriving after timeout handling.

### Invariant 15: Temporal Determinism (Indian Calendar)
**Rule:** Every cut-off and settlement window decision is deterministically reproducible from timestamp, Asia/Kolkata timezone, and the active RBI holiday calendar version.
- **Enforcement:** Injectable `Clock` and versioned `calendar_days` table.
- **Target Attack:** Midnight boundary leap second or timezone offset drift.
