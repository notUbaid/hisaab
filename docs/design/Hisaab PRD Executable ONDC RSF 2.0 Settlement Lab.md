# Hisaab: An Executable Settlement Lab for ONDC RSF 2.0

PRD v0.2 · 29 Oct 2026 · Owner: Ubaid · Status: draft for review

## 1. Decision summary

Build **Hisaab**, an open-source money-correctness engine for ONDC's Reconciliation and Settlement Framework (RSF 2.0): a Settlement Agency (SA) simulator, Network Participant (NP) tooling, and a double-entry Postgres ledger underneath, all attacked by a failure-injection harness. ONDC's own Workbench already checks message shape and flow order, so Hisaab does not compete there. It answers a different question: after a valid sequence of messages, with crashes, retries and failing banks, is every rupee still where it should be? The **Correctness Report** that answers it is the headline deliverable.

What changed in v0.2 after the red-team pass (section 2):

- **Positioning:** Hisaab sits on top of ONDC's Workbench and schemas instead of rebuilding them, and owns money state only.
- **Stack:** Go core on Postgres 16, Next.js console. You allowed any stack; Go fits payments backends, concurrency and test tooling best for a 14-week build.
- **Design:** a two-level ledger (order legs plus net batch postings) to avoid hot-account contention, per-batch Merkle roots instead of a global hash chain, async ACK and callback semantics, request signing, an Indian bank calendar, and deduction legs.
- **Testing:** exhaustive crash-point enumeration and differential testing replace random kill -9. The claims are about our code, not Postgres.
- **Plan:** 14 weeks with a week-2 validation gate, a week-6 public v0.1, and written kill criteria.

Confirmed: India-first. 'Production-ready' here means the checklist in section 13, with evidence in the repo for every row. It does not mean the sandbox ever touches real money.

## 2. Red team: where this idea breaks

The idea survives only in a narrower form: Hisaab is strong as a money-state correctness engine and weak as a protocol tool or a product. I found 17 flaws; nine are High or Critical and would have sunk v0.1 as written. All are addressed in this version.

| # | Flaw | Severity | Fix (section) |
| --- | --- | --- | --- |
| 1 | ONDC already runs a Protocol Workbench with schema validation and flow testing, with NTS10 2.0.0 and RSF 1.0 specs on actively updated branches. Rebuilding that duplicates it and goes stale. | Critical | Reuse ONDC's validator in CI; Hisaab owns money state only (4, 8) |
| 2 | Hisaab models an SA whose real behavior sits behind NPCI-NBBL's NOCS. The BRD is a business document, so 'BRD-faithful' is not 'SA-faithful'. | High | Policy flags, differential tests, honest labeling; never claim certification value (4, 12) |
| 3 | No proof of demand: the NP engineer audience is small and nobody has been asked yet. | High | Three practitioner calls before week 3, written kill criteria (14, 15) |
| 4 | Tests encode my reading of the spec, so a misread passes every test. | High | Spec-independent invariants as the oracle, differential tests, practitioner review (9, 12) |
| 5 | Every order debits the same collector settlement account, so per-order postings serialize on one row. This kills throughput and the 'highly optimized' goal. | High | Two-level ledger: order legs plus one net posting per account per batch; benchmark against a naive baseline (10, 12) |
| 6 | A simulator without request signing, registry lookups, async ACK then callback, message\_id dedupe and TTL proves little and fails against real clients. | High | M2.2 and M2.3 are P0 (8) |
| 7 | Idempotency done as check-then-insert races under concurrent duplicates; keys get reused with different payloads or expire silently. | High | Insert-first unique key, stored response, in-progress state, retention policy (8) |
| 8 | 'Production-ready' without access control, field encryption, audit and maker-checker is a slogan. Claims carry seller bank details. | High | Production readiness checklist with evidence per row (13) |
| 9 | Ten weeks part-time for 23 edge cases, a ledger, a harness, a console and a new language is not credible. | High | 14 weeks, vertical slice in week 2, public v0.1 in week 6, cut line (14) |
| 10 | The hiring signal is niche: recruiters at gateway companies may not know RSF. | Medium | Lead the README with transferable guarantees; ONDC is the case study (17) |
| 11 | Random kill -9 mostly tests Postgres, not our code, and cannot be reproduced. | Medium | Crash-point enumeration: a failpoint at every commit, send and ack boundary (12) |
| 12 | A global hash chain makes every write depend on the previous one and serializes the whole ledger. | Medium | One Merkle root per batch, chained per NP pair, verified offline (8) |
| 13 | 'Same date' cut-offs, settlement windows and bank holidays in IST. Naive UTC handling mis-ages exceptions. | Medium | Business calendar service, zone on every time field, tests around midnight and holidays (8) |
| 14 | Real payouts carry deductions. The BRD claim fields do not show tax legs such as GST TCS or TDS (verify applicability per flow). | Medium | Deduction legs as configurable postings, no hard-coded rates (8, 10) |
| 15 | Performance claims without method are marketing, and the language is not the bottleneck; the Postgres write path is. | Medium | Benchmark protocol and targets stated as hypotheses (6, 12) |
| 16 | Spec drift: the BRD I read is dated Aug 2024, and RSF 1.0 and 2.0 flows coexist in ONDC's tooling. | Medium | Versioned spec packs, weekly upstream diff job (8) |
| 17 | 'Exactly-once' is an overclaim across SA, bank and NP boundaries. Only at-least-once delivery with idempotent consumers is achievable. | Low | Say effectively-once; word the invariants accordingly (9) |

What survives: the correctness evidence and the ONDC case study. What does not: any claim that Hisaab is the SA, a certification route, or a product with a proven market. Treat it as a portfolio-grade engineering artifact that may also help a few integrators.

## 3. Research findings

### 3.1 What RSF 2.0 defines

Source: ONDC's RSF 2.0 explainer and BRD (version dated 2 Aug 2024).

- Actors: Buyer App (usually the Collector), Seller App (Receiver), the SA, the banks, and the Seller on Record (SoR).
- Both sides submit a /settle claim independently. The SA matches them order by order, checks the Collector's balance in its 'account for ONDC settlement', then instructs the banks to move money on a net basis. NPs do not net before submitting.
- Each /settle call must cover exactly one NP pair. Logistics fees go in a separate /settle.
- Refunds reuse the machinery in reverse, and the balance check moves to the Seller App's account.
- Afterwards, NPs exchange discrepancies through /receiver\_recon and /on\_receiver\_recon.
- The BRD lists 23 edge cases: no submission, partial submission, claims mixing several counterparties (rejected as invalid format), missing fields (invalid schema), a counterpart arriving a day late, insufficient balance (all matched orders rejected), and bank-side failures.

Settlement basis and window (the BRD's example is order delivered plus 2 days) decide when an order becomes claimable, so Hisaab models both as per-pair parameters. Refund handling also depends on whether money has already moved to the Seller App.

### 3.2 Who is already working on this

| What exists | What it is | Gap for us |
| --- | --- | --- |
| [ONDC Protocol Workbench](https://github.com/ONDC-Official/automation-framework) and [automation-specifications](https://github.com/ONDC-Official/automation-specifications) | Self-service schema validation and flow testing; NTS10 2.0.0 and RSF 1.0 on active branches | Covers message shape and flow order. I found no sign it models money state (match, balance check, netting, ledger). Verify before building |
| [log-validation-utility](https://github.com/ONDC-Official/log-validation-utility) | Open-source validator with an RSF endpoint for settle, on\_settle, report and recon flows | Reuse it in CI instead of re-implementing schemas |
| [RSF mock service and mock-server-utility](https://github.com/ONDC-Official/mock-server-utility) | Stubs for NP-side tests; expects a fixed 5 objects per call; recon status faked from amount thresholds | Not a spec-faithful SA; no edge-case behavior |
| [rsf-bulk-utility](https://github.com/ONDC-Official/rsf-bulk-utility) | Mongo-backed workbench for receiver\_recon and on\_receiver\_recon, written for ticketing flows; signed requests with registry key lookup | Narrow; no settlement logic or ledger |
| Namma Yatri open-source PRs ([16129](https://github.com/nammayatri/nammayatri/pull/16129), [16342](https://github.com/nammayatri/nammayatri/pull/16342)) | In-house RSF reconciliation inside a mobility platform (Haskell) | Proves the pain is real and teams solve it privately; study for behavior, do not copy |
| Protean RSP, NOCS | Commercial and industry-run | Closed |
| Gateway recon repos (Razorpay-style) | Many near-identical projects | Saturated, different problem |
| Open-source ledgers | Formance, Blnk, TigerBeetle, Midaz | Saturated; do not headline |

Reading: ONDC itself owns protocol compliance, so the gap is narrower than I first said. What looks open is a model of what happens to money after valid messages (match, balance check, netting, instruction, bank result), with failure evidence. As far as my searches show, that does not exist in the open. Searches cannot prove absence, so re-check in week 0 (section 16) and ask ONDC integrators.

### 3.3 Hiring signals

Settlement and reconciliation postings I found (Easebuzz, OpenFX, Razorpay finance ops) ask for the same things: accuracy and auditability, PostgreSQL and event-sourcing depth, multi-entity ledger reconciliation, refunds, chargebacks and reversals, suspense accounts, and NPCI/RBI familiarity. Most are senior roles, so the repo has to stand in for experience. Interview themes that keep appearing: idempotent retries, atomic ledger entries, and a reconciliation job that fills in when a callback goes missing. Hisaab is designed so each has a demonstrable answer.

### 3.4 Ideas examined and parked

- **UPI MDR engine.** Reported structure: 0.4% on specified P2M payments above ₹2,000, capped at ₹300 (reached at ₹75,000), flat ₹5 for notified categories, 0.02% for capital markets, small merchants under ₹1 lakh a month via QR exempt, e-mandates outside. The deferral did not change the structure, but the start date is unknown. Becomes plug-in M7.
- **Account Aggregator statement pipeline, UPI fraud simulator, agent payments.** Valid, but they need heavy synthetic data or sit on unstable protocols.

### 3.5 Spec ambiguities already spotted

From my reading of the BRD text (verify against the PDF before publishing). These seed the ambiguity register.

1. **Match semantics.** In the balance-check example the seller claims orders 1 to 4 and the collector orders 2 to 5. Orders 3 and 4 count as matched although BFF differs (seller 5 and 25, collector 15 and 5). Is a match on Network Order ID alone, or ID plus amounts?
2. **Arithmetic.** The example states a matched total of ₹200, but orders 2 to 4 sum to ₹300 on either side. The text also says processed for orders 2 and 3 and rejected for order 3, which looks like a typo.
3. **Insufficient balance** rejects every matched order (edge case 12). The BRD does not say whether partial fill is intended.
4. **Late counterparts** (edge cases 9 and 11): no stated limit on how long unmatched orders can be carried forward.
5. **'Same date'** has no defined cut-off time or time zone.

Each becomes a policy flag in the simulator (strict, lenient, custom) and a question you can raise with ONDC.

## 4. Positioning and guardrails

- Hisaab is not a Settlement Agency and not a certification route. In the BRD the SA role runs on the NPCI-NBBL NOCS platform, and ONDC's Workbench is the path to protocol compliance. Hisaab models money state for testing.
- It reuses ONDC's schemas and validators (log-validation-utility, automation-specifications) instead of copying them, and pins every scenario to a spec version.
- Label everything 'BRD-faithful, not production-verified'. No ONDC or NPCI logos, no implied endorsement.
- 'Production-ready' means the operating discipline in section 13: a sandbox run like a payment service, never one that touches real money.
- Synthetic data only: no real account numbers, IFSCs or merchant data.
- Where the spec is silent, behavior is a visible policy flag with a register entry, never a silent guess.

## 5. Users and jobs to be done

| Persona | Job | Pain today |
| --- | --- | --- |
| NP or TSP engineer (seller, buyer, logistics app) | Test /settle claims offline before certification and understand every rejection | Mock server is stub-level, the Workbench checks message shape rather than money outcomes, and the real rules live in a PDF |
| NP finance ops | See what settled, was rejected or is stuck; know what to resubmit and expected cash | One bad field can reject a whole claim; manual tracing |
| Integrator or QA | Run conformance scenarios in CI | No shared scenario suite |
| Reviewer or hiring manager | Judge in ten minutes whether you understand money correctness | Most portfolios show features, not guarantees |

## 6. Goals, non-goals, success metrics

**Goals:** spec-faithful behavior; ledger invariants that hold under failure; reproducible failures; outsider setup in 15 minutes with `docker compose up`.

**Non-goals:** real bank connectivity, UPI rails, KYC, fraud ML, multi-currency, a mobile app, throughput bragging.

| Metric | Target |
| --- | --- |
| BRD edge cases as executable scenarios | 23 of 23, each cited to a BRD section and pinned to a spec version |
| Ledger invariants enforced and attacked | At least 15, each with a test built to break it |
| Crash-point coverage | Every commit, send and ack boundary on the settlement path crashed once per scenario; zero violations after recovery |
| Randomized operations | 50k per PR run, 1M nightly, zero violations |
| Mutation score | Harness kills at least 90% of seeded defects (remove the idempotency check, drop the transaction wrapper, reorder locks) |
| Batch settlement (hypothesis) | A 10,000-order NP-pair batch settles in under 3 s at p95 with durability on |
| Ledger throughput (hypothesis) | 5,000 ledger transactions per second sustained in batches on 4 vCPU |
| Recon (hypothesis) | 1M-line three-way recon in under 60 s |
| Recovery (hypothesis) | An interrupted batch resumes within 10 s of restart with no manual repair |
| Ambiguity register | At least 8 entries, each with a policy flag |
| External validation | 3 practitioner reviews or one upstream issue or PR |
| Outsider setup time | Under 15 minutes |

Mutation score is there because I cannot promise you will find real bugs. Seeded defects make the claim checkable either way. Real bugs found go in the report as a bonus.

The four rows marked hypothesis are guesses to test, not claims. Publish what you measure even when it misses, with a profile and the next fix; an honest miss is still evidence.

## 7. Scope: modules

| ID | Module | Summary | Priority |
| --- | --- | --- | --- |
| M0 | Spec-as-code | Versioned spec pack: edge-case table, scenario DSL, ambiguity register; reuses ONDC schemas | P0 |
| M1 | Ledger core | Two-level append-only Postgres ledger: order legs plus net postings, idempotency, batch Merkle roots | P0 |
| M2 | SA simulator | Signed async /settle flow, matching, balance check, netting, synthetic banks | P0 |
| M3 | NP toolkit | Claim builder, pre-flight validator, balance forecast | P1 |
| M4 | Recon engine | Three-way reconciliation, exception taxonomy, maker-checker adjustments | P0 |
| M5 | Verification harness | Crash points, properties, differential tests, mutation, benchmarks, report | P0 |
| M6 | Ops console | Exception queue, timeline, playground, RBAC | P1 |
| M7 | Rule plug-ins | Effective-dated rules, UPI MDR pack | P2 |
| M8 | Production hardening | Security, observability, migrations, backups, runbooks (section 13); runs through every milestone | P0 |

## 8. Functional requirements

### M0 Spec-as-code

| ID | Requirement | Pri |
| --- | --- | --- |
| M0.1 | YAML decision table of the 23 edge cases: inputs, expected outcome, error code, BRD citation | P0 |
| M0.2 | Scenario DSL (given orders, claims, balances; then outcomes) shared by simulator, tests and playground | P0 |
| M0.3 | Ambiguity register with one policy flag per entry | P0 |
| M0.4 | Versioned spec packs, pinned to NTS10 2.0.0 first; a weekly CI job diffs the upstream ONDC spec repos and opens an issue on change | P1 |
| M0.5 | Payload validation calls ONDC's log-validation-utility RSF endpoint in CI; no home-grown schema checker | P0 |

### M1 Ledger core

| ID | Requirement | Pri |
| --- | --- | --- |
| M1.1 | Two-level model: order legs in a sub-ledger plus one net posting per account per batch, so per-order work never locks a shared account row | P0 |
| M1.2 | Append-only double-entry journal; integer paise; no floats anywhere | P0 |
| M1.3 | Idempotency: insert-first unique key with request hash, stored response and an in-progress state; replay returns the original result; same key with a different payload is an error; retention window defined | P0 |
| M1.4 | Balanced transactions enforced in the database: writes only through one posting function, plus a deferred constraint as a safety net | P0 |
| M1.5 | Immutability: UPDATE and DELETE revoked and trigger-blocked; fixes are reversing entries | P0 |
| M1.6 | No negative balance on constrained accounts under concurrency: locks in fixed account order, retry on serialization and deadlock errors | P0 |
| M1.7 | Trial balance and point-in-time balance queries; cached balances continuously reconciled to journal sums | P0 |
| M1.8 | Two-phase postings (pending, then posted or voided) for in-flight bank instructions | P1 |
| M1.9 | Tamper evidence: one Merkle root per settlement batch, chained per NP pair, with an offline verification CLI; no global hash chain | P1 |
| M1.10 | Transactional outbox for events, drained by SKIP LOCKED workers with idempotent consumers | P1 |
| M1.11 | Journal partitioned by month; retention and archival policy documented | P1 |

### M2 SA simulator

| ID | Requirement | Pri |
| --- | --- | --- |
| M2.1 | /settle and /on\_settle shaped like the official ONDC contract; payloads validated by ONDC's own validator | P0 |
| M2.2 | Beckn async semantics: synchronous ACK or NACK, then a callback; message\_id dedupe; TTL expiry; callback retry with backoff | P0 |
| M2.3 | Request signing: verify inbound signatures and sign callbacks as the ONDC protocol docs specify; public keys come from a registry lookup (stubbed registry in tests) | P0 |
| M2.4 | Order-level matcher with policy flag: ID only vs ID plus amounts | P0 |
| M2.5 | NP-pair rule and schema checks with BRD error codes (edge cases 5 to 8) | P0 |
| M2.6 | Balance check, all-or-nothing by default, partial-fill flag (edge case 12) | P0 |
| M2.7 | Gross obligations per order and net instructions per bank and NP; the two must tie out (invariant 13) | P0 |
| M2.8 | Settlement basis and window per NP pair (for example order delivered plus 2 days) decide when an order is claimable | P1 |
| M2.9 | Indian business calendar: bank holidays, IST cut-offs, value dates; every time field carries its zone | P1 |
| M2.10 | Carry-forward of unmatched orders with configurable retention (edge cases 9 to 11) | P1 |
| M2.11 | Refund flow with reversed claimant and seller-side balance check | P1 |
| M2.12 | Synthetic bank adapter with injectable delay, failure and duplicate messages (edge cases 13 to 23) | P1 |

### M3 NP toolkit and M4 Recon

| ID | Requirement | Pri |
| --- | --- | --- |
| M3.1 | Claim builder: order book in, one /settle per NP pair out, logistics fees split | P0 |
| M3.2 | Pre-flight validator that predicts SA rejections and explains why | P0 |
| M3.3 | Collector balance forecast against expected matched total | P1 |
| M3.4 | Deduction legs (for example GST TCS or TDS) as configurable postings; no hard-coded rates; tax correctness itself is out of scope | P1 |
| M3.5 | Resubmission planner for rejected or carried orders | P2 |
| M4.1 | Three-way recon: order book vs SA report vs synthetic bank statement | P0 |
| M4.2 | Exception taxonomy, 10+ classes (missing counterpart, amount mismatch, invalid schema, insufficient balance, bank failure, duplicate credit, timing break) | P0 |
| M4.3 | Every resolution is a new posting, never an edit; manual adjustments need maker-checker approval and an audit record | P0 |
| M4.4 | receiver\_recon and on\_receiver\_recon exchange | P1 |
| M4.5 | Suspense account with ageing measured in business days | P1 |

### M5 Verification, M6 Console, M7 Plug-ins

| ID | Requirement | Pri |
| --- | --- | --- |
| M5.1 | Invariant checker after every scenario and as a background job | P0 |
| M5.2 | Crash-point enumeration: a failpoint at every commit, send and ack boundary; each scenario runs once per point and recovery is checked | P0 |
| M5.3 | Property-based generator for order books, claims and partial failures, with shrinking | P0 |
| M5.4 | Network faults through Toxiproxy (delay, duplicate, drop); clock skew through an injectable clock | P0 |
| M5.5 | Differential tests against ONDC's mock service and validator; observable behavior compared with other open implementations, studied but not copied | P1 |
| M5.6 | Every failure prints a seed and a minimal repro | P0 |
| M5.7 | Generated Correctness Report in CI | P0 |
| M5.8 | Mutation testing with seeded defects | P1 |
| M5.9 | Load and soak tests with a published benchmark method and a naive-baseline comparison | P0 |
| M5.10 | Concurrency history checking in the style of Jepsen's Elle | P2 |
| M6.1 | Exception queue with ageing and filters | P0 |
| M6.2 | Settlement-day timeline per NP pair: claims, match, balance check, instruction, bank result | P0 |
| M6.3 | Order detail showing postings and audit trail | P0 |
| M6.4 | RBAC with viewer, operator and approver roles, and a full operator audit log | P0 |
| M6.5 | Scenario playground: pick an edge case, run it, watch it unfold | P1 |
| M7.1 | Effective-dated rules (valid time and record time) with a 'pending' effective-date state | P2 |
| M7.2 | UPI MDR rule pack, only once NPCI notifies the date | P2 |

## 9. Ledger invariants

These are the product. Each gets a test written to break it.

1. Every transaction sums to zero in INR.
2. The journal is append-only, and each batch's Merkle root verifies offline.
3. Effectively-once: one idempotency key produces at most one set of postings, however many times the request is delivered.
4. Constrained accounts never go negative at any commit, including under concurrency.
5. Conservation: an order's settlement moves exactly its total (commission + SoR due + BFF = order amount).
6. An order settles at most once per direction (forward, refund).
7. The SA instructs money only for orders matched under the active policy.
8. Crash safety: after any crash and recovery each settlement is wholly applied or wholly absent, and in-flight instructions are recoverable from the outbox.
9. Recon completeness: every SA line and bank line is claimed by exactly one outcome.
10. Determinism: the same scenario and seed give an identical ledger.
11. The trial balance is zero at all times, and cached balances equal journal sums.
12. Every posting links to its cause (claim, instruction or bank message).
13. Gross-to-net tie-out: per batch, the order legs equal the net postings, and net bank instructions across all banks sum to zero.
14. Callback integrity: each /settle reaches at most one terminal outcome per message\_id; late or duplicate callbacks never change state.
15. Time: every cut-off decision is reproducible from the timestamp, zone and calendar version.

## 10. Domain and ledger model

Accounts per NP: `ondc_settlement` (the SA-controlled account) and `operative`. Plus `sor:{id}:bank`, an external buyer clearing account, and an SA `suspense` account.

Worked example using the BRD annex numbers: an order of ₹100 = commission 10 + SoR due 85 + BFF 5. Amounts in paise.

| Step | Debit | Credit | Paise |
| --- | --- | --- | --- |
| 1. Buyer pays the Collector | ext:buyer\_clearing | collector:ondc\_settlement | 10000 |
| 2. SA moves the remaining amount to the Receiver | collector:ondc\_settlement | receiver:ondc\_settlement | 9500 |
| 3. Last mile to the Seller on Record | receiver:ondc\_settlement | sor:bank | 8500 |
| 4. Receiver commission | receiver:ondc\_settlement | receiver:operative | 1000 |
| 5. BFF to the Collector | collector:ondc\_settlement | collector:operative | 500 |

Both settlement accounts return to zero. That is invariant 5 in miniature.

**Two-level posting.** Steps 1 to 5 are written per order as legs in the sub-ledger, with no shared-row locks. At batch commit, one net posting per account is applied under a single lock taken in account-id order, in the same transaction as the instruction records and outbox events. Invariant 13 checks that the legs and the net postings tie out. Deductions such as GST TCS or TDS appear as extra legs; rates are configuration, not code.

Core tables: accounts, journal\_transactions, journal\_entries, idempotency\_keys, claims, claim\_lines, match\_results, instructions, bank\_messages, recon\_items, exceptions, outbox, settlement\_batches, batch\_roots, approvals, audit\_log, calendar\_days.

## 11. Architecture and stack

Go core, Postgres 16, Next.js console. The language is not the bottleneck; the Postgres write path is, so most optimization work targets that.

| Option | Evidence and fit | Cost | Verdict |
| --- | --- | --- | --- |
| Go | Named in a [Razorpay engineering posting](https://builtin.com/job/lead-software-development-engineer/7831140) and the [OpenFX ledger role](https://jobs.m13.co/companies/openfx-2/jobs/57768166-staff-engineer-ledger-and-recon); goroutines, pgx, native fuzzing, fast CI | A new language for you; budget two weeks | Pick |
| Rust | Also listed by OpenFX; best speed and type safety | Steep curve, slow iteration, schedule risk | Skip for v1 |
| Java or Kotlin | Common in large payment stacks (verify against the postings you target) | Heavier setup, slower start | Reasonable alternative |
| TypeScript | Your fastest path | Weaker numeric safety and concurrency model for a ledger core | Console only |
| Python | Best property-testing library (Hypothesis) | Slowest runtime; hard to defend 'highly optimized' | Scripts and analysis only |

- **Where the speed comes from:** batched multi-row or COPY inserts, one round trip per batch through a posting function, short lock hold times, a monthly-partitioned journal, narrow indexes, and fillfactor tuning on hot balance rows so updates stay HOT.
- **Concurrency:** account locks in fixed id order, retry on serialization and deadlock errors (SQLSTATE 40001 and 40P01). Compare against SERIALIZABLE by experiment and record the result in an ADR. Jepsen's 2020 analysis reported serializability anomalies in Postgres 12.3, so test your target version instead of trusting the label.
- **Pooling:** a pgx pool in the app. If you add PgBouncer, use transaction mode and keep no session state or advisory locks across statements.
- **Settlement runs** are idempotent, resumable batches keyed by NP pair, settlement date and run id.
- **Hosting:** API on Cloud Run and managed Postgres with point-in-time recovery in asia-south1 for the demo. Fault tests run on local Docker Postgres because you need to kill processes.
- **Console:** Next.js and TypeScript on a read-only database role.
- **Adapter boundary:** a SpecAdapter interface isolates RSF 2.0 from the core, so another network's settlement spec could plug in later (unverified idea).
- **Tooling:** pgx, sqlc, goose, testcontainers-go, rapid for property tests, Go native fuzzing, Toxiproxy, OpenTelemetry, GitHub Actions.

## 12. Verification plan

Layers: (1) scenario tests for all 23 edge cases, (2) crash-point enumeration, (3) property tests with random order books and fault schedules, (4) differential tests against ONDC's mock service and validator, (5) mutation testing, (6) load and soak. These test our code's behavior under failure. They do not test Postgres itself: durability is assumed and configured (fsync on, synchronous\_commit on).

The oracle problem: the 15 invariants do not depend on my reading of the spec, while the edge-case table does. That is why the table is cross-checked by differential tests, spec citations and practitioner review.

Fault catalogue (each fault runs at every crash point where it applies):

1. Crash after the debit leg, before the credit leg.
2. Crash after commit, before the acknowledgement.
3. Duplicate /settle delivery.
4. Claims arriving out of order or a day apart.
5. Two settlement runs for the same pair at once.
6. Bank sends a duplicate credit message.
7. Bank fails after the SA instruction.
8. Clock skew around the cut-off.
9. Partial network partition between SA and NP.
10. Database restart mid-batch.

The Correctness Report is a table: Invariant, Attack, Result, Bug found, Fix commit. Pass means zero invariant violations and every failure reproducible from a seed.

**Benchmark method.** Publish hardware, Postgres version and settings with durability on, dataset size, warm-up, and p50, p95 and p99. Run the two-level design against a naive per-order-lock baseline in the same harness and report both; the comparison is the evidence that the design earns its complexity.

## 12. Milestones (10 weeks, part-time)

| Week | Deliverable |
| --- | --- |
| 1 | Spec encoded (M0), schema, first ADR |
| 2 to 3 | Ledger core, invariants, first property tests |
| 4 to 5 | SA simulator forward flow, edge cases 1 to 12 |
| 6 | Refunds, carry-forward, bank adapter, edge cases 13 to 23 |
| 7 | NP toolkit and recon |
| 8 | Fault injection, mutation testing, first Correctness Report |
| 9 | Console and playground |
| 10 | Docs, demo video, outreach |

Exit rule for every milestone: suite green and the report updated. Cut line if you fall behind: drop 3.3, 3.4, 5.7, M7 and 6.4. Never drop M0, M1 or M5.

## 13. Risks

| Risk | Mitigation |
| --- | --- |
| Misreading the spec | Cite a BRD section on every scenario, keep the ambiguity register, ask practitioners, open upstream issues |
| Real SA behavior differs from the BRD | Label as BRD-faithful, expose policy flags |
| Criticism that it is just a simulator | Position as a conformance and test tool, and lead with the Correctness Report |
| Someone already built it | Week 0 prior-art search; plan B is the NP-side recon plus the harness only |
| Scope creep | Cut line in section 12 |
| Legal or data issues | Synthetic data only, no logos, explicit disclaimer |

## 14. Verify before building

- Re-read the latest RSF BRD and the API contract in ONDC's official repos. I read the Aug 2024 BRD.
- Search GitHub again for prior art (ONDC-Official org, Namma Yatri finance code, topic searches).
- UPI MDR start date: still not notified as of the sources I found.
- RBI Payment Aggregator Directions 2025: sources conflict on whether T+1 is a ceiling or timelines are set in merchant agreements. Read the RBI text if you ever add a PA flow.

## 15. Packaging for hiring

- README first screen: one architecture diagram, three headline numbers (edge cases covered, invariants attacked, mutation score), one GIF of the playground.
- Docs: SPEC-COVERAGE.md, INVARIANTS.md, CORRECTNESS-REPORT.md, AMBIGUITIES.md, and numbered ADRs.
- Two write-ups: what the BRD leaves ambiguous, and what crash-testing a ledger taught you.
- Resume bullets only after the numbers exist. Never write a claim you cannot point to in the repo.
- Outreach: share the ambiguity findings with ONDC integrators and the Namma Yatri finance code owners.

## 16. Open questions

1. India-first confirmed?
2. Python core or all TypeScript?
3. Hours per week you can commit?
4. Is the name Hisaab fine?

## Appendix: sources

- ONDC RSF 2.0 explainer and BRD: https://ondc-static-website-media.s3.ap-south-1.amazonaws.com/res/daea2fs3n/image/upload/ondc-website/files/esf\_2\_0\_explainer\_and\_brd\_02\_08\_24.pdf
- ONDC mock server: https://github.com/ONDC-Official/mock-server-utility
- ONDC RSF workbench: https://github.com/ONDC-Official/rsf-bulk-utility
- Namma Yatri RSF PRs: https://github.com/nammayatri/nammayatri/pull/16129 and https://github.com/nammayatri/nammayatri/pull/16342
- Protean on RSF 2.0: https://www.proteantech.in/articles/protean-RSF-02-04-2025/
- UPI MDR deferral: https://www.business-standard.com/finance/news/upi-mdr-deferred-until-after-diwali-new-date-to-be-notified-npci-126101300556\_1.html
- UPI MDR structure: https://bfsi.eletsonline.com/npci-introduces-0-4-mdr-on-upi-transactions-above-rs2000/
- RBI PA Master Direction: https://rbi.org.in/SCRIPTs/BS\_ViewMasDirections.aspx?id=12896
- Jepsen on Postgres 12.3: https://jepsen.io/analyses/postgresql-12.3
- Elle isolation checker: https://arxiv.org/pdf/2003.10554
- Hiring: OpenFX Staff Engineer Ledger and Recon (job-boards.greenhouse.io/openfx), Easebuzz S&R backend role, Razorpay finance ops posting
