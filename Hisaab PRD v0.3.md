# Hisaab: Executable Settlement Lab for ONDC RSF 2.0

**PRD v0.3 · 29 Oct 2026 · Owner: Ubaid · Status: Ready to Build**

> [!IMPORTANT]
> **India-first. Go + Postgres 16 core. Next.js console. Open-source.**
> All open questions from v0.2 are resolved in this version. This document is the source of truth for what to build, in what order, and why.

---

## What Changed in v0.3

| Area | v0.2 | v0.3 |
|---|---|---|
| Open questions (Section 16) | 4 unanswered | All resolved inline |
| Stack choice | "Go fits best" | Locked: Go + Postgres 16 + Next.js. Rationale in §11 |
| India-first | "Confirmed" | Detailed: IST, RBI calendar, NPCI references, INR paise-only |
| Milestone plan | 10-week label, 14-week body | Corrected: 14-week plan, 5-day/week part-time (~3h/day) |
| New flaws found | 17 from red-team | 5 additional flaws added (§2), total 22 |
| Kickstart scaffold | Missing | Full repo scaffold in §17 |
| Section 12 duplication | Two "Section 12"s | Fixed: milestones moved to §13, verification stays §12 |

---

## 1. Decision Summary

Build **Hisaab** — a money-correctness engine for ONDC's Reconciliation and Settlement Framework (RSF 2.0).

ONDC's Workbench already validates message shape and flow order. Hisaab answers a different question: **after a valid sequence of messages, with crashes, retries, and failing banks, is every rupee still where it should be?**

The **Correctness Report** that answers it is the headline deliverable. Everything else — the SA simulator, the NP toolkit, the double-entry ledger, the failure harness — exists to produce that report.

**What Hisaab is:**
- An SA simulator (not a real SA — never touches real money)
- A double-entry Postgres ledger with invariants that survive failure
- A failure-injection harness that attacks those invariants reproducibly
- An NP toolkit for building and validating `/settle` claims offline

**What Hisaab is not:**
- A certification route (ONDC's Workbench owns that)
- A product (no proven market yet; treat as engineering artifact)
- A replacement for NOCS (NPCI-NBBL's real SA platform)

---

## 2. Red Team: Where This Idea Breaks

v0.2 found 17 flaws. This pass adds 5 more. All 22 are addressed.

| # | Flaw | Severity | Fix (§) |
|---|---|---|---|
| 1 | ONDC Workbench already does schema + flow validation. Rebuilding duplicates and goes stale. | Critical | Reuse ONDC's log-validation-utility in CI; Hisaab owns money state only (§4, §8) |
| 2 | Hisaab models an SA whose real behavior is behind NPCI-NBBL NOCS. BRD-faithful ≠ SA-faithful. | High | Policy flags, differential tests, honest labeling; never claim certification value (§4, §12) |
| 3 | No demand proof: NP engineer audience is small, nobody asked yet. | High | 3 practitioner calls before week 3, written kill criteria (§14, §15) |
| 4 | Tests encode my reading of the spec. A misread passes every test. | High | Spec-independent invariants as oracle; differential tests; practitioner review (§9, §12) |
| 5 | Per-order postings serialize on one collector settlement account row — kills throughput. | High | Two-level ledger: order legs + one net posting per account per batch; benchmark (§10, §12) |
| 6 | No request signing, registry lookup, async ACK/callback, message_id dedupe, or TTL = proves little against real clients. | High | M2.2 and M2.3 are P0 (§8) |
| 7 | Idempotency as check-then-insert races under concurrent duplicates. Keys reused with different payloads or expire silently. | High | Insert-first unique key, stored response, in-progress state, retention policy (§8) |
| 8 | "Production-ready" without access control, field encryption, audit, and maker-checker is a slogan. | High | Production readiness checklist with evidence per row (§13) |
| 9 | 14 weeks part-time for 23 edge cases, a ledger, harness, console, and a new language is tight. | High | Vertical slice in week 2; public v0.1 in week 6; explicit cut line (§13) |
| 10 | Hiring signal is niche: recruiters at gateway companies may not know RSF. | Medium | Lead README with transferable guarantees; ONDC is the case study (§16) |
| 11 | Random kill -9 mostly tests Postgres, not our code, and cannot be reproduced. | Medium | Crash-point enumeration: failpoint at every commit, send, and ACK boundary (§12) |
| 12 | A global hash chain serializes the whole ledger. | Medium | One Merkle root per batch, chained per NP pair, verified offline (§8) |
| 13 | "Same date" cut-offs and bank holidays in IST. Naive UTC handling mis-ages exceptions. | Medium | Business calendar service, zone on every time field, tests around midnight and holidays (§8) |
| 14 | Real payouts carry deductions (GST TCS, TDS). BRD claim fields do not show tax legs. | Medium | Deduction legs as configurable postings; no hard-coded rates (§8, §10) |
| 15 | Performance claims without method are marketing. | Medium | Benchmark protocol with targets as hypotheses (§6, §12) |
| 16 | Spec drift: BRD is Aug 2024. RSF 1.0 and 2.0 coexist in ONDC tooling. | Medium | Versioned spec packs; weekly upstream diff CI job (§8) |
| 17 | "Exactly-once" is an overclaim across SA, bank, and NP boundaries. | Low | Say "effectively-once"; invariants worded accordingly (§9) |
| **18** | **No database migration strategy defined. Schema changes mid-project on an append-only ledger are extremely dangerous.** | **High** | goose for versioned migrations; migration tests in CI; explicit zero-downtime migration runbook in §13 |
| **19** | **Next.js console reads from the same Postgres primary — long analytical queries can block settlement writes.** | **High** | Console uses a dedicated read replica (or read-only pool pointed at replica); no writes from the console ever (§11) |
| **20** | **Go is new to you. "Two weeks to learn" is optimistic for a concurrent, strongly-typed systems language being used for a ledger.** | **High** | Week 0 is a Go ramp sprint (see §13). Vertical slice in week 2 is the gate. If week-2 gate fails, fall back to TypeScript + Kysely for the ledger core (documented fallback in §13). |
| **21** | **The `settlement_batches` table has no partition key. At scale, scanning it for "open" batches becomes a full table scan.** | **Medium** | Partition `settlement_batches` by `settlement_date` (monthly); add partial index on `status = 'open'` (§10) |
| **22** | **No explicit rollback story for the 14-week plan if ONDC changes RSF mid-build.** | **Medium** | SpecAdapter interface isolates RSF 2.0 from core (§11). If RSF changes, only M0 + M2 surface area changes; M1, M4, M5 survive intact. |

**What survives:** the correctness evidence and the ONDC case study.
**What does not:** any claim that Hisaab is the SA, a certification route, or a product with a proven market.

---

## 3. Research Findings

### 3.1 What RSF 2.0 Defines

Source: ONDC RSF 2.0 explainer and BRD (2 Aug 2024 version). Re-verify against the latest before writing any scenario.

- **Actors:** Buyer App (Collector), Seller App (Receiver), Settlement Agency (SA), Banks, Seller on Record (SoR)
- Both sides submit `/settle` independently. SA matches order-by-order, checks Collector's balance, then instructs net bank transfers. NPs do not net before submitting.
- Each `/settle` covers exactly one NP pair. Logistics fees go in a separate `/settle`.
- Refunds use the same machinery in reverse; balance check moves to Seller App's account.
- NPs exchange discrepancies via `/receiver_recon` and `/on_receiver_recon`.
- BRD lists 23 edge cases: no submission, partial submission, invalid NP pairs, missing fields, late counterparts, insufficient balance, bank-side failures.
- Settlement basis and window (e.g., order delivered + 2 days) are per-pair parameters.

### 3.2 Competitive Landscape

| What exists | What it is | Gap |
|---|---|---|
| ONDC Protocol Workbench + automation-specifications | Schema validation + flow testing; NTS10 2.0.0 + RSF 1.0 active | Covers message shape. No money state (match, balance check, netting, ledger). **Verify before building.** |
| log-validation-utility | Open-source validator with RSF endpoint | Reuse in CI; never re-implement |
| RSF mock service + mock-server-utility | NP-side stubs; expects fixed 5 objects/call; recon status faked from amount thresholds | Not spec-faithful; no edge-case behavior |
| rsf-bulk-utility | Mongo-backed workbench for receiver_recon; narrow, no settlement logic | Narrow; study only |
| Namma Yatri RSF PRs (#16129, #16342) | In-house RSF recon inside a mobility platform (Haskell) | Proves pain is real; study for behavior, do not copy |
| Protean RSP, NOCS | Commercial + industry-run | Closed |
| Open-source ledgers (Formance, Blnk, TigerBeetle, Midaz) | Generic ledger primitives | Saturated; do not headline; consider TigerBeetle for inspiration on correctness testing |

### 3.3 Hiring Signal

Settlement and recon roles (Easebuzz, OpenFX, Razorpay finance ops) consistently ask for:
- Accuracy and auditability at scale
- PostgreSQL + event-sourcing depth
- Multi-entity ledger reconciliation, refunds, chargebacks, reversals
- Suspense accounts, NPCI/RBI familiarity

Interview themes: idempotent retries, atomic ledger entries, reconciliation jobs that fill in for missing callbacks. Hisaab has a demonstrable answer for each.

### 3.4 India-First: Regulatory and Infrastructure Context

This section locks the India-first constraints:

| Constraint | Implementation |
|---|---|
| All amounts in **Indian Rupees**, stored as **paise (integer)** | `BIGINT` in Postgres; zero floats anywhere |
| **IST (UTC+5:30)** for all cut-off decisions | Every time field stored as `TIMESTAMPTZ`; business logic uses IST; zone recorded with decision |
| **RBI bank holidays** affect settlement windows | `calendar_days` table seeded with RBI holiday list; business day arithmetic via a service |
| **NPCI conventions** for UPI payment references | Order ID format, message ID format, and trace IDs follow NPCI patterns |
| **GST TCS / TDS** deduction legs | Configurable posting rules; no hard-coded rates; correctness of tax calculation is out of scope |
| **IFSC codes** for bank routing | All bank references use IFSC; masked in logs; never in synthetic data |
| **RBI PA Directions 2025** | If a PA flow is ever added, read the RBI text. T+1 ceiling vs. merchant-agreement timelines are disputed. |

### 3.5 Spec Ambiguities Already Spotted

These seed the ambiguity register (M0.3). Each gets a policy flag.

1. **Match semantics.** Seller claims orders 1–4; Collector claims 2–5. Orders 3–4 shown as matched despite differing BFF amounts. Is a match on Network Order ID alone, or ID + amounts?
2. **Arithmetic error.** BRD states a matched total of ₹200 but orders 2–4 sum to ₹300. Also says "processed for orders 2 and 3 and rejected for order 3" — apparent typo.
3. **Insufficient balance** rejects every matched order (edge case 12). No stated intent on partial fill.
4. **Late counterparts** (edge cases 9, 11): no stated limit on carry-forward duration.
5. **"Same date"** cut-off has no defined time or time zone.
6. **Deduction applicability.** BRD claim fields don't show GST TCS/TDS legs. Verify with ONDC whether deductions apply per-order or per-batch.

### 3.6 Ideas Parked

- **UPI MDR engine:** Structure defined (0.4% on P2M > ₹2,000, capped ₹300, flat ₹5 notified categories, etc.). Start date still unnotified. Becomes plug-in M7 once notified.
- **Account Aggregator pipeline, UPI fraud simulator, agent payments:** Valid but need heavy synthetic data or sit on unstable protocols. Out of scope v1.

---

## 4. Positioning and Guardrails

- Hisaab is **not** a Settlement Agency and **not** a certification route.
- It reuses ONDC schemas and validators instead of copying them.
- Label everything: **"BRD-faithful, not production-verified."**
- No ONDC or NPCI logos. No implied endorsement.
- "Production-ready" = the operating discipline in §13. The sandbox never touches real money.
- Synthetic data only: no real account numbers, IFSCs, or merchant data.
- Where the spec is silent, behavior is a visible policy flag with a register entry — never a silent guess.

---

## 5. Users and Jobs-to-Be-Done

| Persona | Job | Pain Today |
|---|---|---|
| NP / TSP engineer (seller, buyer, logistics app) | Test `/settle` claims offline before certification; understand every rejection | Mock server is stub-level; Workbench checks message shape, not money outcomes; rules live in a PDF |
| NP finance ops | See what settled, was rejected, or is stuck; know what to resubmit and expected cash | One bad field rejects the whole claim; manual tracing |
| Integrator / QA | Run conformance scenarios in CI | No shared scenario suite |
| Reviewer / hiring manager | Judge in 10 minutes whether you understand money correctness | Most portfolios show features, not guarantees |

---

## 6. Goals, Non-Goals, Success Metrics

**Goals:** spec-faithful behavior; ledger invariants that hold under failure; reproducible failures; outsider setup in ≤15 minutes with `docker compose up`.

**Non-goals:** real bank connectivity, UPI rails, KYC, fraud ML, multi-currency, mobile app, throughput bragging.

| Metric | Target |
|---|---|
| BRD edge cases as executable scenarios | 23 of 23, each cited to a BRD section and pinned to a spec version |
| Ledger invariants enforced and attacked | ≥15, each with a test built to break it |
| Crash-point coverage | Every commit, send, and ACK boundary on the settlement path crashed once per scenario; zero violations after recovery |
| Randomized operations | 50k per PR run, 1M nightly, zero violations |
| Mutation score | Harness kills ≥90% of seeded defects (remove idempotency check, drop transaction wrapper, reorder locks) |
| Batch settlement *(hypothesis)* | 10,000-order NP-pair batch settles in <3s at p95 with durability on |
| Ledger throughput *(hypothesis)* | 5,000 ledger transactions/second sustained in batches on 4 vCPU |
| Recon *(hypothesis)* | 1M-line three-way recon in <60s |
| Recovery *(hypothesis)* | Interrupted batch resumes within 10s of restart, no manual repair |
| Ambiguity register | ≥8 entries, each with a policy flag |
| External validation | 3 practitioner reviews or one upstream issue/PR |
| Outsider setup time | ≤15 minutes |

> [!NOTE]
> The four *hypothesis* rows are guesses to test, not claims. Publish what you measure even when it misses — with a profile and the next fix. An honest miss is still evidence.

---

## 7. Scope: Modules

| ID | Module | Summary | Priority |
|---|---|---|---|
| M0 | Spec-as-code | Versioned spec pack: edge-case table, scenario DSL, ambiguity register; reuses ONDC schemas | P0 |
| M1 | Ledger core | Two-level append-only Postgres ledger: order legs + net postings, idempotency, batch Merkle roots | P0 |
| M2 | SA simulator | Signed async `/settle` flow, matching, balance check, netting, synthetic banks | P0 |
| M3 | NP toolkit | Claim builder, pre-flight validator, balance forecast | P1 |
| M4 | Recon engine | Three-way reconciliation, exception taxonomy, maker-checker adjustments | P0 |
| M5 | Verification harness | Crash points, properties, differential tests, mutation, benchmarks, report | P0 |
| M6 | Ops console | Exception queue, timeline, playground, RBAC | P1 |
| M7 | Rule plug-ins | Effective-dated rules, UPI MDR pack | P2 |
| M8 | Production hardening | Security, observability, migrations, backups, runbooks (§13); runs through every milestone | P0 |

---

## 8. Functional Requirements

### M0 — Spec-as-code

| ID | Requirement | Pri |
|---|---|---|
| M0.1 | YAML decision table of the 23 edge cases: inputs, expected outcome, error code, BRD citation | P0 |
| M0.2 | Scenario DSL (`given` orders/claims/balances; `then` outcomes) shared by simulator, tests, and playground | P0 |
| M0.3 | Ambiguity register with one policy flag per entry | P0 |
| M0.4 | Versioned spec packs pinned to NTS10 2.0.0; weekly CI job diffs upstream ONDC spec repos and opens an issue on change | P1 |
| M0.5 | Payload validation calls ONDC's log-validation-utility RSF endpoint in CI; no home-grown schema checker | P0 |

### M1 — Ledger Core

| ID | Requirement | Pri |
|---|---|---|
| M1.1 | Two-level model: order legs in sub-ledger + one net posting per account per batch; per-order work never locks a shared account row | P0 |
| M1.2 | Append-only double-entry journal; integer paise; **no floats anywhere** | P0 |
| M1.3 | Idempotency: insert-first unique key with request hash, stored response, in-progress state; replay returns original result; same key with different payload = error; retention window defined | P0 |
| M1.4 | Balanced transactions enforced in DB: writes only via one posting function + deferred constraint as safety net | P0 |
| M1.5 | Immutability: UPDATE and DELETE revoked + trigger-blocked; fixes are reversing entries | P0 |
| M1.6 | No negative balance on constrained accounts under concurrency: locks in fixed account order, retry on serialization (40001) and deadlock (40P01) | P0 |
| M1.7 | Trial balance and point-in-time balance queries; cached balances continuously reconciled to journal sums | P0 |
| M1.8 | Two-phase postings (pending → posted or voided) for in-flight bank instructions | P1 |
| M1.9 | Tamper evidence: one Merkle root per settlement batch, chained per NP pair, offline verification CLI; no global hash chain | P1 |
| M1.10 | Transactional outbox for events, drained by SKIP LOCKED workers with idempotent consumers | P1 |
| M1.11 | Journal partitioned by month; retention and archival policy documented | P1 |

### M2 — SA Simulator

| ID | Requirement | Pri |
|---|---|---|
| M2.1 | `/settle` and `/on_settle` shaped like the official ONDC contract; payloads validated by ONDC's validator | P0 |
| M2.2 | Beckn async semantics: synchronous ACK or NACK; then callback; message_id dedupe; TTL expiry; callback retry with backoff | P0 |
| M2.3 | Request signing: verify inbound signatures and sign callbacks per ONDC protocol docs; public keys from registry lookup (stubbed registry in tests) | P0 |
| M2.4 | Order-level matcher with policy flag: ID only vs. ID + amounts | P0 |
| M2.5 | NP-pair rule and schema checks with BRD error codes (edge cases 5–8) | P0 |
| M2.6 | Balance check, all-or-nothing by default, partial-fill flag (edge case 12) | P0 |
| M2.7 | Gross obligations per order + net instructions per bank/NP; the two must tie out (invariant 13) | P0 |
| M2.8 | Settlement basis and window per NP pair (e.g., order delivered + 2 days) decide when an order is claimable | P1 |
| M2.9 | Indian business calendar: RBI bank holidays, IST cut-offs, value dates; every time field carries its zone | P1 |
| M2.10 | Carry-forward of unmatched orders with configurable retention (edge cases 9–11) | P1 |
| M2.11 | Refund flow with reversed claimant and seller-side balance check | P1 |
| M2.12 | Synthetic bank adapter with injectable delay, failure, and duplicate messages (edge cases 13–23) | P1 |

### M3 — NP Toolkit and M4 — Recon

| ID | Requirement | Pri |
|---|---|---|
| M3.1 | Claim builder: order book in → one `/settle` per NP pair out; logistics fees split | P0 |
| M3.2 | Pre-flight validator predicts SA rejections and explains why | P0 |
| M3.3 | Collector balance forecast against expected matched total | P1 |
| M3.4 | Deduction legs (GST TCS, TDS) as configurable postings; no hard-coded rates; tax correctness out of scope | P1 |
| M3.5 | Resubmission planner for rejected or carried orders | P2 |
| M4.1 | Three-way recon: order book vs. SA report vs. synthetic bank statement | P0 |
| M4.2 | Exception taxonomy, 10+ classes (missing counterpart, amount mismatch, invalid schema, insufficient balance, bank failure, duplicate credit, timing break) | P0 |
| M4.3 | Every resolution is a new posting, never an edit; manual adjustments need maker-checker approval + audit record | P0 |
| M4.4 | `/receiver_recon` and `/on_receiver_recon` exchange | P1 |
| M4.5 | Suspense account with ageing measured in business days (Indian calendar) | P1 |

### M5 — Verification, M6 — Console, M7 — Plug-ins

| ID | Requirement | Pri |
|---|---|---|
| M5.1 | Invariant checker after every scenario and as background job | P0 |
| M5.2 | Crash-point enumeration: failpoint at every commit, send, and ACK boundary; each scenario runs once per point; recovery checked | P0 |
| M5.3 | Property-based generator for order books, claims, and partial failures; with shrinking | P0 |
| M5.4 | Network faults via Toxiproxy (delay, duplicate, drop); clock skew via injectable clock | P0 |
| M5.5 | Differential tests against ONDC mock service + validator; observable behavior compared with other open implementations | P1 |
| M5.6 | Every failure prints a seed and a minimal repro | P0 |
| M5.7 | Generated Correctness Report in CI | P0 |
| M5.8 | Mutation testing with seeded defects | P1 |
| M5.9 | Load and soak tests with published benchmark method + naive-baseline comparison | P0 |
| M5.10 | Concurrency history checking in the style of Jepsen's Elle | P2 |
| M6.1 | Exception queue with ageing and filters | P0 |
| M6.2 | Settlement-day timeline per NP pair: claims → match → balance check → instruction → bank result | P0 |
| M6.3 | Order detail showing postings and audit trail | P0 |
| M6.4 | RBAC: viewer, operator, approver roles; full operator audit log | P0 |
| M6.5 | Scenario playground: pick an edge case, run it, watch it unfold | P1 |
| M7.1 | Effective-dated rules (valid time + record time) with "pending" effective-date state | P2 |
| M7.2 | UPI MDR rule pack — only once NPCI notifies the date | P2 |

---

## 9. Ledger Invariants

**These are the product.** Each gets a test written to break it.

1. Every transaction sums to zero in INR paise.
2. The journal is append-only; each batch's Merkle root verifies offline.
3. **Effectively-once:** one idempotency key produces at most one set of postings, however many times the request is delivered.
4. Constrained accounts never go negative at any commit, including under concurrency.
5. **Conservation:** an order's settlement moves exactly its total (commission + SoR due + BFF = order amount).
6. An order settles at most once per direction (forward, refund).
7. The SA instructs money only for orders matched under the active policy.
8. **Crash safety:** after any crash and recovery, each settlement is wholly applied or wholly absent; in-flight instructions are recoverable from the outbox.
9. **Recon completeness:** every SA line and bank line is claimed by exactly one outcome.
10. **Determinism:** the same scenario and seed produce an identical ledger.
11. The trial balance is zero at all times; cached balances equal journal sums.
12. Every posting links to its cause (claim, instruction, or bank message).
13. **Gross-to-net tie-out:** per batch, order legs equal net postings; net bank instructions across all banks sum to zero.
14. **Callback integrity:** each `/settle` reaches at most one terminal outcome per `message_id`; late or duplicate callbacks never change state.
15. **Time:** every cut-off decision is reproducible from the timestamp, zone, and calendar version.

---

## 10. Domain and Ledger Model

**Accounts per NP:**
- `collector:ondc_settlement` — SA-controlled account
- `collector:operative` — NP's own operational account
- `receiver:ondc_settlement` — SA-controlled account
- `receiver:operative`
- `sor:{id}:bank` — external Seller on Record bank account
- `ext:buyer_clearing` — external buyer clearing (synthetic)
- `sa:suspense` — SA suspense account

**Worked example** (BRD Annex numbers): ₹100 order = commission ₹10 + SoR due ₹85 + BFF ₹5. Amounts in paise.

| Step | Debit | Credit | Paise |
|---|---|---|---|
| 1. Buyer pays Collector | `ext:buyer_clearing` | `collector:ondc_settlement` | 10,000 |
| 2. SA moves net to Receiver | `collector:ondc_settlement` | `receiver:ondc_settlement` | 9,500 |
| 3. Last mile to SoR | `receiver:ondc_settlement` | `sor:bank` | 8,500 |
| 4. Receiver commission | `receiver:ondc_settlement` | `receiver:operative` | 1,000 |
| 5. BFF to Collector | `collector:ondc_settlement` | `collector:operative` | 500 |

Both settlement accounts return to zero. That is invariant 5 in miniature.

**Two-level posting:** Steps 1–5 written per order as legs in the sub-ledger, with no shared-row locks. At batch commit: one net posting per account applied under a single lock taken in account-id order, in the same transaction as the instruction records and outbox events. Invariant 13 checks that legs and net postings tie out.

**Core tables:**

```
accounts                 — chart of accounts, account type, constrained (no-negative) flag
journal_transactions     — one row per atomic posting event
journal_entries          — two rows per transaction (debit + credit)
idempotency_keys         — request hash, status, stored response, expires_at
claims                   — /settle submissions per NP pair
claim_lines              — per-order lines within a claim
match_results            — outcome of each order-level match
instructions             — bank payment instructions
bank_messages            — synthetic bank responses (injectable)
recon_items              — three-way recon lines
exceptions               — unresolved recon exceptions
outbox                   — transactional outbox events (SKIP LOCKED drain)
settlement_batches       — keyed by (np_pair, settlement_date, run_id); partitioned by settlement_date (monthly)
batch_roots              — Merkle roots per batch, chained per NP pair
approvals                — maker-checker approval records
audit_log                — append-only log of all operator actions
calendar_days            — RBI/NPCI Indian bank holiday calendar
```

---

## 11. Architecture and Stack

> [!IMPORTANT]
> **India-first stack decisions:** Go core + Postgres 16 + Next.js console.
> Go is chosen for correctness tooling (native fuzzing, testcontainers, pgx, race detector) and concurrency model.
> TypeScript could be the fallback for the ledger core if the Go ramp in week 0 fails the week-2 gate.

### Stack Rationale

| Layer | Choice | Why | Fallback |
|---|---|---|---|
| Ledger + SA core | **Go 1.23** | Named in Razorpay + OpenFX postings; goroutines; pgx; native fuzzing; race detector; fast CI; strong numeric safety | TypeScript + Kysely (slower but familiar) |
| Database | **Postgres 16** | ACID guarantees, row-level locking, SKIP LOCKED, partitioning, PgAudit, PITR | — |
| Console | **Next.js 15 + TypeScript** | Fastest UI path; read-only DB role | — |
| Migrations | **goose** | SQL-based, version-controlled, zero-magic | — |
| ORM / query | **sqlc** | Type-safe SQL codegen; no hidden queries | — |
| Connection pool | **pgx v5 pool** | Prepared statements; first-class Postgres types | PgBouncer in transaction mode if needed (no session state, no advisory locks) |
| Property tests | **rapid** (Go) + **Hypothesis** (Python, analysis only) | Go-native shrinking | — |
| Fault injection | **Toxiproxy** | Deterministic network faults | — |
| Containerization | **Docker Compose** | One-command setup for contributors | — |
| Hosting (demo) | **Cloud Run + Cloud SQL Postgres** in `asia-south1` | India region; managed PITR | — |
| Observability | **OpenTelemetry** → **Cloud Trace / Grafana Cloud** | Structured, vendor-neutral | — |
| CI | **GitHub Actions** | Free for public repos | — |

### Where the Speed Comes From

1. **Batched inserts:** `COPY` or multi-row `INSERT` for order legs; one round trip per batch
2. **Posting function:** one stored procedure per batch, minimal lock hold time
3. **Monthly partitioning:** journal and settlement_batches; narrow indexes; fillfactor tuning on balance rows (HOT updates)
4. **Account lock order:** always take locks in account-id order to prevent deadlocks
5. **Retry on 40001/40P01:** exponential backoff with jitter
6. **Read replica:** console reads only from replica; no analytical queries on primary

### Concurrency Strategy

- Default: per-account advisory locks in fixed id order
- Experiment: compare against `SERIALIZABLE` isolation; document the result in an ADR
- Reference: Jepsen 2020 analysis found serializability anomalies in Postgres 12.3. Test your target version; don't trust the label.

### Key Interfaces

```go
// SpecAdapter — isolates RSF 2.0 from ledger core
// Another network's settlement spec can plug in here
type SpecAdapter interface {
    Validate(payload []byte) error
    ParseClaim(payload []byte) (*Claim, error)
    FormatOnSettle(result *SettleResult) ([]byte, error)
}

// BankAdapter — injectable synthetic bank
type BankAdapter interface {
    SendInstruction(ctx context.Context, inst *Instruction) error
    SimulateFailure(mode FailureMode) BankAdapter
}

// Clock — injectable for deterministic time tests
type Clock interface {
    Now() time.Time
    BusinessDay(t time.Time, cal *Calendar) time.Time
}
```

---

## 12. Verification Plan

**Layers:**
1. Scenario tests for all 23 edge cases
2. Crash-point enumeration
3. Property tests with random order books and fault schedules
4. Differential tests against ONDC mock service and validator
5. Mutation testing
6. Load and soak

> [!NOTE]
> These test **our code's behavior under failure**. They do not test Postgres itself. Durability is assumed and configured (`fsync = on`, `synchronous_commit = on`).

**The oracle problem:** The 15 invariants do not depend on our reading of the spec; the edge-case table does. The table is cross-checked by differential tests, spec citations, and practitioner review.

### Fault Catalogue

Each fault runs at every crash point where it applies:

1. Crash after debit leg, before credit leg
2. Crash after commit, before ACK
3. Duplicate `/settle` delivery
4. Claims arriving out of order or a day apart
5. Two settlement runs for the same pair concurrently
6. Bank sends duplicate credit message
7. Bank fails after SA instruction
8. Clock skew around the cut-off
9. Partial network partition between SA and NP
10. Database restart mid-batch

### Correctness Report Format

| Invariant | Attack | Result | Bug Found | Fix Commit |
|---|---|---|---|---|
| (invariant text) | (fault catalogue #) | PASS / FAIL | — or commit | — or link |

Pass = zero invariant violations + every failure reproducible from a seed.

### Benchmark Method

Publish: hardware spec, Postgres version + settings (durability on), dataset size, warm-up period, p50/p95/p99. Run the two-level design against a naive per-order-lock baseline in the same harness and report both. The comparison is the evidence that the design earns its complexity.

---

## 13. Production Readiness Checklist

> [!CAUTION]
> Every row must have evidence in the repo before claiming "production-ready." Evidence = a link to code, config, or a passing test.

| Area | Requirement | Evidence location |
|---|---|---|
| **Auth** | RBAC (viewer, operator, approver); no anonymous access to the console | `internal/auth/` |
| **Field encryption** | Sensitive fields (IFSC, bank refs) encrypted at rest | `internal/crypto/` |
| **Audit log** | Append-only; every operator action; tamper-evident | `audit_log` table + test |
| **Maker-checker** | Manual adjustments require dual approval | `approvals` table + M6.4 |
| **Migrations** | goose; migration tests in CI; zero-downtime runbook | `db/migrations/` |
| **Secrets** | No secrets in env files or repo; use Secret Manager / Vault | `.github/workflows/` |
| **TLS** | Enforced in Cloud Run; no plaintext in transit | Cloud Run config |
| **Rate limiting** | API endpoints rate-limited; `/settle` endpoint especially | `internal/middleware/` |
| **Observability** | Structured logs (JSON), traces (OTel), metrics (Prometheus) | `internal/telemetry/` |
| **Health checks** | `/healthz` and `/readyz` for liveness and readiness | `cmd/api/health.go` |
| **Backups** | PITR enabled on Cloud SQL; backup test in runbook | Cloud SQL config |
| **DR runbook** | Written, tested, linked from README | `docs/runbooks/` |
| **Synthetic data only** | CI gate: grep for real IFSC patterns in test fixtures | `.github/workflows/lint.yml` |
| **Disclaimer** | Every page and README has "BRD-faithful, not production-verified" | README, console footer |

### Go Ramp Sprint (Week 0 — Pre-start)

> [!WARNING]
> **Week 0 is a gate.** If by end of week 2 the vertical slice (ledger core with 3 invariant tests passing) is not working in Go, fall back to TypeScript + Kysely for the ledger core. Document the decision as ADR-001.

Recommended resources (3h/day for 7 days):
1. **Tour of Go** (tour.golang.org) — 2 days
2. **"Effective Go"** + **"Go by Example"** — 1 day
3. **pgx v5 docs** + write a toy ledger (accounts + journal) — 2 days
4. **"Learning Go" by Jon Bodner** (chapters 1–8) — 2 days

---

## 14. Milestones (14 Weeks, Part-Time ~3h/day)

> [!NOTE]
> **Exit rule for every milestone:** test suite green + Correctness Report updated.

| Week | Deliverable | Gate |
|---|---|---|
| **0** | Go ramp sprint; read latest RSF BRD; 3 practitioner contact attempts | Go "hello ledger" compiles and pgx connects |
| **1** | M0 complete: spec encoded as YAML, scenario DSL, ambiguity register, M0.5 CI gate | 23 edge cases in YAML, CI green |
| **2** | **Vertical slice:** basic ledger (M1.1–1.4), one `/settle` round trip, 3 invariant tests | **Gate: all 3 invariants pass under 2 crash points. If not, ADR-001 fallback.** |
| **3** | Ledger core complete (M1.1–1.7); all 15 invariants + tests | Invariant checker green |
| **4–5** | SA simulator forward flow (M2.1–2.7); edge cases 1–12 | 12 scenarios pass |
| **6** | **Public v0.1:** refunds, carry-forward, bank adapter (M2.8–2.12); edge cases 13–23 | All 23 scenarios pass; Docker Compose works |
| **7** | NP toolkit (M3.1–3.2) + Recon engine (M4.1–4.3) | Recon finds injected mismatch |
| **8** | Fault injection + mutation testing + first Correctness Report (M5.1–5.9) | Mutation score ≥90%; crash-point coverage complete |
| **9** | Console and playground (M6.1–6.5) | RBAC working; timeline renders correctly |
| **10** | Ledger hardening: Merkle roots, outbox, partitioning (M1.8–1.11) | Merkle offline verification CLI passes |
| **11** | Observability, benchmarks, production hardening (M8, M5.9) | Benchmark published with baseline |
| **12** | Practitioner review; address feedback; upstream issues/PRs to ONDC | ≥1 practitioner review complete |
| **13** | Docs, write-ups, README polish | SPEC-COVERAGE.md, INVARIANTS.md, CORRECTNESS-REPORT.md |
| **14** | Demo video, outreach, resume bullets | Repo public and linkable |

**Cut line** (if behind): drop M3.3, M3.4, M5.7 (console playground), M7, M6.4. **Never drop M0, M1, or M5.**

---

## 15. Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Misreading the spec | High | Cite BRD section on every scenario; keep ambiguity register; ask practitioners; open upstream issues |
| Real SA behavior differs from BRD | Medium | Label as BRD-faithful; expose policy flags |
| Criticism that it's just a simulator | Medium | Position as conformance and test tool; lead with Correctness Report |
| Someone already built it | Low | Week 0 prior-art search; plan B = NP-side recon + harness only |
| Scope creep | High | Cut line in §14 |
| Legal or data issues | Low | Synthetic data only; no logos; explicit disclaimer |
| RSF spec changes mid-build | Medium | SpecAdapter interface isolates core; M1/M4/M5 survive unchanged |
| Go ramp takes longer than 2 weeks | Medium | TypeScript fallback defined in §13 |
| Database migration breaks ledger invariants | Medium | Migration tests in CI; goose down migrations tested; zero-downtime runbook |

---

## 16. Verify Before Building (Week 0 Checklist)

- [ ] Re-read the latest RSF BRD and API contract in ONDC's official repos (I read the Aug 2024 BRD)
- [ ] Search GitHub again for prior art: ONDC-Official org, Namma Yatri finance code, topic searches
- [ ] Confirm UPI MDR start date: still not notified as of sources found
- [ ] RBI PA Directions 2025: read the actual RBI text if you ever add a PA flow
- [ ] Ping 3 NP/TSP engineers (WhatsApp, LinkedIn, or ONDC Slack) before week 3
- [ ] Check ONDC Workbench for any money-state features I missed
- [ ] Read Namma Yatri RSF PRs for behavioral insights (not code to copy)

---

## 17. Packaging for Hiring

- **README first screen:** one architecture diagram, three headline numbers (edge cases covered, invariants attacked, mutation score), one GIF of the playground
- **Docs:** `SPEC-COVERAGE.md`, `INVARIANTS.md`, `CORRECTNESS-REPORT.md`, `AMBIGUITIES.md`, numbered ADRs
- **Two write-ups:** what the BRD leaves ambiguous; what crash-testing a ledger taught you
- **Resume bullets** only after the numbers exist — never write a claim you cannot point to in the repo
- **Outreach:** share ambiguity findings with ONDC integrators and Namma Yatri finance code owners

**Transferable signals to lead with (not RSF-specific):**

> "Built a double-entry ledger with 15 formally stated invariants, each attacked by a custom harness. Crash-point coverage across all commit and ACK boundaries. Mutation score ≥90%. The invariants hold."

---

## 18. Repo Scaffold

The repo starts here. Full scaffold in `KICKSTART.md` (generated separately).

```
hisaab/
├── cmd/
│   ├── api/           # Go HTTP server (SA simulator + NP toolkit API)
│   ├── worker/        # Outbox drain + background jobs
│   └── cli/           # Merkle verification CLI, benchmark runner
├── internal/
│   ├── ledger/        # M1: journal, accounts, posting functions
│   ├── settlement/    # M2: SA simulator, matching, balance check, netting
│   ├── recon/         # M4: three-way recon, exception taxonomy
│   ├── verification/  # M5: invariant checker, crash points, property tests
│   ├── nptoolkit/     # M3: claim builder, pre-flight validator
│   ├── calendar/      # Indian business calendar, IST cut-offs
│   ├── auth/          # RBAC
│   ├── crypto/        # Field encryption
│   ├── telemetry/     # OTel setup
│   └── middleware/    # Rate limiting, request signing
├── spec/
│   ├── edge-cases.yaml        # M0.1: 23 edge cases
│   ├── ambiguities.yaml       # M0.3: ambiguity register
│   └── scenarios/             # M0.2: scenario DSL files
├── db/
│   ├── migrations/            # goose SQL migrations
│   ├── queries/               # sqlc .sql files
│   └── schema.sql             # canonical schema
├── console/                   # Next.js 15 + TypeScript (M6)
├── tests/
│   ├── scenarios/             # 23 BRD scenario tests
│   ├── property/              # rapid property tests
│   ├── crash/                 # crash-point enumeration
│   ├── differential/          # vs ONDC mock service
│   └── bench/                 # load + soak benchmarks
├── docs/
│   ├── adr/                   # Architecture Decision Records
│   ├── runbooks/              # DR, migration, on-call
│   ├── SPEC-COVERAGE.md
│   ├── INVARIANTS.md
│   ├── CORRECTNESS-REPORT.md
│   └── AMBIGUITIES.md
├── docker-compose.yml         # one-command setup
├── docker-compose.test.yml    # test environment with Toxiproxy
├── Makefile                   # dev shortcuts
└── README.md                  # lead with architecture diagram + 3 numbers
```

---

## Appendix: Open Questions (All Resolved)

| Question (from v0.2) | Resolution |
|---|---|
| India-first confirmed? | **Yes.** IST, paise, RBI calendar, IFSC — all first-class. See §3.4. |
| Python core or all TypeScript? | **Go core, TypeScript console.** Python for analysis scripts only. See §11. |
| Hours per week you can commit? | **~3h/day, 5 days/week = ~15h/week.** 14-week plan reflects this. |
| Is the name Hisaab fine? | **Yes.** हिसाब = "account/reckoning" in Hindi/Urdu. Fits perfectly. |

---

## Appendix: Sources

- ONDC RSF 2.0 explainer and BRD: https://ondc-static-website-media.s3.ap-south-1.amazonaws.com/res/daea2fs3n/image/upload/ondc-website/files/esf_2_0_explainer_and_brd_02_08_24.pdf
- ONDC Protocol Workbench: https://github.com/ONDC-Official/automation-framework
- ONDC automation-specifications: https://github.com/ONDC-Official/automation-specifications
- ONDC log-validation-utility: https://github.com/ONDC-Official/log-validation-utility
- ONDC mock-server-utility: https://github.com/ONDC-Official/mock-server-utility
- ONDC rsf-bulk-utility: https://github.com/ONDC-Official/rsf-bulk-utility
- Namma Yatri RSF PRs: https://github.com/nammayatri/nammayatri/pull/16129 and /16342
- Protean on RSF 2.0: https://www.proteantech.in/articles/protean-RSF-02-04-2025/
- UPI MDR deferral: https://www.business-standard.com/finance/news/upi-mdr-deferred-until-after-diwali-new-date-to-be-notified-npci-126101300556_1.html
- RBI PA Master Direction: https://rbi.org.in/SCRIPTs/BS_ViewMasDirections.aspx?id=12896
- Jepsen on Postgres 12.3: https://jepsen.io/analyses/postgresql-12.3
- Elle isolation checker: https://arxiv.org/pdf/2003.10554
- Hiring: OpenFX Staff Engineer Ledger and Recon, Easebuzz S&R backend role, Razorpay finance ops posting
