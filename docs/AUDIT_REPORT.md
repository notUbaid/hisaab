<USER_REQUEST>
# Hisaab â€” Audit & Roadmap

Reviewed: `hisaab-main.zip` (156 files) on 2026-10-30.
Companion file: `adversarial_test.go` (drop into `tests/adversarial/`; every red test is a to-do item).

---

## 0. Verdict

**The domain choice is excellent, the skeleton is real, and the project currently claims far more than it proves.**

- ONDC multi-party settlement (RSF) is a real, recognised pain point. NPCI's NOCS platform and Protean's RSP exist as commercial answers, and open tooling is thin. This was the right target.
- The in-memory ledger is genuinely well-behaved under concurrency (my 50-goroutine overdraft test passes).
- But: the Postgres ledger is never called, it has a real overdraft race, the migration does not apply on real PostgreSQL, the "crash tests / mutation score / Merkle proofs / 23-of-23 coverage" are mostly not what they say they are, and the matcher can pay an order twice.

**Why this matters for your goal:** a fintech interviewer will find the false claims in 5â€“10 minutes (`cat tests/crash/crash_test.go`, run `hisaab-cli verify-merkle --batch-id 999`). A project that claims 100% and is wrong looks worse than one that claims 60% and is right. **The good news: fixing this *is* the project.** "I audited my own settlement engine, found N bugs, and here are the tests that now prevent them" is stronger proof of fintech skill than any badge.

**Strategy, in order:**
1. Make every claim true, or delete it. (1â€“2 days)
2. Fix the money bugs. (week 1)
3. Make Postgres real and prove it. (week 2)
4. Connect the pieces into an actual end-to-end system. (week 3)
5. Real verification + publish the write-up and test vectors. (week 4)

Do **not** add features before steps 1â€“3.

---

## 1. How I tested (and the limits)

- Read the code, docs, scripts, schema, CI and console source.
- Ran every existing Go test I could compile (12 packages; not `cmd/worker` or the `rapid`-based `tests/property`): **all pass**. They pass because of what they test; see Â§3 for what that is.
- Wrote 11 adversarial tests against the real code (`adversarial_test.go`): **9 fail, 1 passes, 1 is informational**. Ran with `-race`.
- Installed PostgreSQL 16.15 and applied your migration. Reproduced the overdraft race by replaying the exact SQL of `Ledger.Post` in two concurrent sessions, then verified each fix I recommend.
- Verified tax rates and ONDC signing against current public sources.

**Limits (so you can trust the report):**
- I used Go 1.24 (your `go.mod` says 1.23). My sandbox couldn't download `pgx`, `rapid`, so I could not compile `cmd/worker` or `tests/property` and ran the Postgres race via `psql`, not through Go. The logic is identical; the SQL is copied from `ledger.go`.
- I did not run your benchmark, and I did not have the ONDC RSF 2.0 spec text. Anything marked **(verify)** needs checking against the spec or a CA.
- I can't prove a negative. "No Merkle code exists" means I searched the whole repo and found none.

---

## 2. What is genuinely good (keep it)

- **Integer paise everywhere** (only float use is one CLI display line). Correct instinct.
- **In-memory ledger**: serialized, idempotent, overdraft-safe (my concurrency test passes).
- **Insert-first idempotency + request hash** as a *design intent*. Right idea, wrong execution in Postgres (Â§4, B4).
- **Spec-driven scenario YAML + a DSL runner.** This is your most valuable idea. See U7.
- **ADRs and an ambiguity register.** Hiring managers love "here's what the spec doesn't say."
- **Immutability triggers** on the ledger (DELETE/UPDATE blocked; I verified). TRUNCATE gap in S4.
- **Right problem framing:** "ONDC Workbench tests syntax; nobody tests the money." That sentence is true and is your pitch.

---

## 3. Tier 0 â€” Claims that are false or unsupported (fix first)

> These are the dangerous ones. None of them is a hard engineering fix; each is "make it true or delete it."

### C1. `verify-merkle` is fake
- `cmd/cli/main.go:86-104` hashes the string `batch:<id>:verified` and prints `[OK] Offline Merkle proof intact. Cryptographic audit verified.` for **any** id.
- **No Merkle tree exists anywhere in the repo.** `batch_roots` is never written.
- The UI timeline shows hardcoded roots. Batch 2's root `sha256:e3b0c442â€¦b855` is **the SHA-256 of the empty string** (I checked).
- README's example `verify-merkle BATCH-20261030-01` doesn't even parse (the flag is `--batch-id`, an int).
- **Fix now:** delete Inv-10, the CLI command and the UI roots. **Fix properly:** see U6.

### C2. The "crash tests" test fakes, not your system
`tests/crash/crash_test.go`:
- `SimulatedSettlementRunner` is a test-local slice + map. It is not your ledger.
- `failpoints.Trigger()` has **zero call sites** in production code (I grepped).
- FP1 panics *before* the append and asserts nothing was appended.
- FP4 sets `outboxStatus := "pending"` then asserts it equals `"pending"`.
- FP6 panics then asserts that it panicked. FP7 calls the checker with string literals.
- Result: "crash-point enumeration across 8 failpoints" currently verifies **nothing** about crash recovery. Real version: U3.

### C3. "Mutation score 100%" is not mutation testing
- `tests/mutation` passes six hand-crafted bad inputs to checker functions and confirms they return an error. No source code is mutated.
- Real mutation testing (e.g. `gremlins` or `go-mutesting`) rewrites your code (`<` â†’ `<=`, deletes a check) and asks whether tests fail. Expect well under 100% on the first run, which is fine; publish the real number and kill the survivors (U5).

### C4. "23/23 edge cases passed" counts function names
- `scripts/check_spec_coverage.py` only checks that a function named `TestScenario_EC_NN` exists.
- **13 of 23 tests contain no assertion at all.** EC-08 asserts `10000 != 12000` (two local constants). EC-12 asserts `15000 < 30000`. EC-01 and EC-20 only assert a date is non-zero.
- Real logic with meaningful assertions: EC-03, EC-16, EC-17 (and narrowly EC-11, EC-19).
- The 8 YAML scenarios through the DSL runner are the real ones, but the runner builds inputs so Invariant 5 holds *by construction* (`sor := amount - comm - bff`), only asserts `Then` fields when they're `> 0` (so you can't assert "0 matched"), and prints `PASSED (100% Invariant Compliant)` unconditionally on no-error.

### C5. The â‚¹4.8 crore figure has no source
- README and whitepaper: "network participants reported â‚¹4.8 crore of un-reconciled breaks in Q2 2024 pilots." No citation in the repo; I found no public source.
- An ONDC-literate interviewer will ask first. **Delete it.** Replace with something citable: ONDC multi-party reconciliation is a recognised problem, and NPCI's NOCS / Protean's RSP exist as commercial answers.

### C6. The benchmark is a strawman
- "Naive" = 100 goroutines locking a Go mutex. "TwoLevel" = calling the matcher once. That compares mutex contention to a function call, not databases.
- README's benchmark names (`BenchmarkNaiveNetting`) **don't exist** in the code (`BenchmarkNaivePerOrderLocking` does).
- All 100 benchmark orders share the ID `"ORD-BENCH"`, so the matcher's map collapses them and the benchmark silently triggers bug B1 without checking the result.
- "O(K log K) two-level netting": the matcher is a hash map (O(K)) plus a sort, for **one** NP pair, **forward flow only**.
- **Fix:** remove the numbers. Replace with a real Postgres contention benchmark after Â§S/B4, or delete.

### C7. The console is a static mock
- Dashboard, timeline and exceptions pages contain **zero `fetch` calls**: all data is hardcoded arrays.
- Maker-checker (Inv-8) is UI-only. `lib/rbac.ts` is imported nowhere.
- The "Live API" playground POSTs a synthetic `/settle` which the handler merely ACKs; the `X-Hisaab-Scenario` header is ignored.
- **Fix:** wire read pages to the API (U9) or label the console "UI prototype."

### C8. The Postgres ledger is dead code
- `ledger.New(pool)` is never called. `cmd/api` has no database; `/readyz` is hard-coded `true`. CI has no Postgres service.
- Everything verified runs against `MemoryLedger`. The README says "Append-Only PostgreSQL / In-Memory."
- The `ledger` package imports `pgx`, so "zero-dependency in-memory engine" isn't true either: every package that touches the ledger drags in the driver.

### C9. Signals that undermine credibility
- `cmd/api/main.go` has `func init() { _ = fmt.Sprintf("") }` to keep an import alive.
- Root contains `.session-checkpoint.md` ("Anti-Laziness Active"), `KICKSTART.md`, two PRDs, `.playwright_artifacts/`, `.secrets.baseline`.
- Module path `github.com/ubaid/hisaab` vs badge `notUbaid/hisaab`.
- CI never runs `golangci-lint` (you have a config), `go vet`, or `-race`; no Postgres.
- ADR-002 (field encryption) and ADR-003 (rate limiting) are documented but **I found no implementation** in the Go code.
- **Fix:** move design docs to `docs/design/`, delete agent session files, fix the module path, run lint/vet/race in CI, and either implement the ADRs or mark them "proposed."


---

## 4. Tier 1 â€” Money-correctness bugs (each reproduced by running code)

Run `go test -race -v ./tests/adversarial/...` to see them. IDs match the test names.

### B1. A duplicate order ID inside a claim gets paid twice  *(test A1)*
- Buyer claim `[O1: â‚¹1000]`, seller claim `[O1, O1]` â†’ `matched=2`, `TotalMatched=200000`, instruction `199000` paise, while the buyer collected `100000`.
- Cause: `buyerOrderMap[id] = o` silently overwrites; the seller loop never dedups.
- **Fix:** `ValidateClaimPayload` rejects duplicate `NetworkOrderID` within a claim (error code `DUPLICATE_ORDER`). Add a property/invariant: *total instructed â‰¤ total collected from buyer*.
- **Same pattern in `recon.Reconcile`** (`saMap`, `bankCreditMap` keyed by order/reference id): a duplicate SA line or duplicate bank credit overwrites silently. I read this in the code but did not run a test. Duplicate-credit detection is a core recon feature and you list it as a failpoint, so test it.

### B2. Any non-zero tolerance switches off commission/BFF checking  *(tests A2, A3)*
- `matcher.go`: the component-parity check sits inside `if m.policy.AmountTolerancePaise == 0`. Commission â‚¹10 (buyer) vs â‚¹90 (seller) is silently **matched** when tolerance is 5 paise.
- Within-tolerance amount differences use the seller's amount; the buyer's extra paise vanish with no ledger leg. Conservation breaks quietly.
- **Fix:** compare each component strictly (or with its own explicit tolerance). Post any tolerated residual to a named account (`sa:tolerance_adjustment`) so Inv-5/13 include it.

### B3. Refund engine (Invariant 6) is bypassable  *(tests A4, A5, A6, A10)*
- **(a)** The cap uses `claim.OriginalAmount`, **supplied by the claim itself**. A claim can say a â‚¹1000 order was â‚¹10 crore and refund â‚¹9 crore. The bound must come from authoritative state (the settled forward amount in the ledger/order store).
- **(b)** The insolvent-seller (suspense) path doesn't update `settledRefunds`, so the same order can be refunded unlimited times.
- **(c)** Its journal lines debit `sa:suspense`, which `SeedStandardAccounts` doesn't create â†’ posting fails `ErrAccountNotFound`. Only the scenario runner registers it, which is why scenarios pass.
- **(d)** `settledRefunds` is an unsynchronised map: `go test -race` reports a data race.
- **(e)** `sellerAvailableBalance` is passed in by the caller (check-then-act). Read it inside the same DB transaction as the posting.
- **Fix:** `RefundProcessor` takes a store interface (original settled amount, cumulative refunds, balance) and runs inside one transaction. Count suspense refunds toward the cap.

### B4. The Postgres `Ledger.Post` is not safe  *(reproduced on PG 16.15)*
1. **No lock before the balance check.** The comment says "lock constrained accounts in ascending ID order" but the code only runs `SELECT SUM(...)`. Replaying its exact statements in two sessions: balance 100, two withdrawals of 80, **both succeed, balance = âˆ’60** on a constrained account. This violates Inv-4, and it is the classic fintech interview question.
2. **Idempotency row is inserted outside the transaction** (`l.pool.QueryRow`, auto-commit). A failed/crashed post leaves the key `in_progress` forever. A concurrent duplicate sees `in_progress`, **falls through and posts**; the `UNIQUE` on `journal_transactions.idempotency_key` then turns it into an error instead of a replay. A different payload under an `in_progress` key isn't rejected.
3. `expires_at` is never enforced or cleaned; status `'error'` is never set.
4. Balance is `SUM` over every entry on every post: O(n) per posting.
5. `DBTX` is declared but unused, so `Post` can't join a caller's transaction. **Inv-11 (ledger + outbox atomic) is impossible with this API.**
6. The two implementations already **diverge**: `MemoryLedger` honours `req.PostedAt` (caller-controlled); Postgres uses `NOW()`. This is the strongest argument for one conformance suite (U2).
- **Fix:** reference design with verified SQL in Appendix B.

### B5. Validation gaps in the ledger core  *(tests A7, A8)*
- `ValidateLines` sums `int64` with no overflow check. 4Ã—2â¶Â² paise of debits vs 8Ã—2â¶Â² of credits (unbalanced by 2â¶â´) **passes**. Add a per-line cap (e.g. â‰¤ 10Â¹Â³ paise), a max line count, and checked addition.
- `ComputeRequestHash` concatenates fields with no delimiters: `(cause_id=1, desc="2x")` and `(cause_id=12, desc="x")` hash identically, so the idempotency guard could replay the wrong result. Length-prefix every field (or hash canonical JSON) and include currency.
- `PostRequest.RequestHash` lets the **caller** supply the hash, which defeats payload-mismatch detection. Compute it internally; remove the field.

### B6. HTTP handler
- `io.ReadAll(r.Body)` with no `http.MaxBytesReader`; unbounded in-memory `claims` map; no per-request context timeout.
- `/settle` stores and ACKs but **never triggers matching, posting or an outbox event**. `/on_settle` ACKs anything without parsing it, so Inv-14 is never enforced on the wire.
- Beckn `context.ttl`/`timestamp` are not enforced.

---

## 5. Tier 2 â€” Database & infrastructure

### S1. The migration does not apply  *(reproduced)*
- `settlement_batches` is `PARTITION BY RANGE (settlement_date)` but declares `run_id TEXT NOT NULL UNIQUE`. PostgreSQL: *"unique constraint on partitioned table must include all partitioning columns."*
- This fails for **both** `db/migrations/00001_init.sql` **and** `db/schema.sql`, which is the file `docker-compose.yml` mounts. `docker compose up` cannot create the schema.
- CI didn't notice because `verify_sql_schema.py` is regex-over-text; it never executes SQL.
- **Fix:** `UNIQUE (run_id, settlement_date)`; enforce global `run_id` uniqueness via a small non-partitioned `batch_runs` table. Delete `schema.sql` (one source of truth: migrations). **CI: add a `postgres:16` service, apply migrations with goose, then run tests.**

### S2. Time bomb on 1 January 2027
- `journal_entries` has partitions for 2026-10â€¦12 only. **From 2027-01-01 every posting fails** (`no partition of relation "journal_entries" found for row`; reproduced). `settlement_batches` stops at 2027.
- **Fix (verified):** add `CREATE TABLE journal_entries_default PARTITION OF journal_entries DEFAULT` immediately, then `pg_partman` or a scheduled job that pre-creates months. Add a test that posts a row dated two years ahead.
- The calendar has the same cliff (D5).

### S3. The database does not enforce double-entry  *(reproduced, fix verified)*
- A raw `INSERT` of debit 1000 / credit 1 is accepted. The ledger's central invariant lives only in application code.
- **Fix (verified on the partitioned table):** see Appendix B for the deferred constraint trigger. Better still: make a SQL function the only writer and `REVOKE INSERT` from the app role.

### S4. Immutability has a bypass  *(reproduced, fix verified)*
- `DELETE` is blocked; **`TRUNCATE journal_entries` succeeded**. Add a `BEFORE TRUNCATE` statement trigger (Appendix B), and run the app as a non-owner role with `UPDATE/DELETE/TRUNCATE` revoked.

### S5. Chart-of-accounts semantics are inconsistent
- Balance is `credit âˆ’ debit` for **every** account regardless of `Type`. `collector:ondc_settlement` is typed *Asset* but behaves credit-normal; the CLI trial balance labels "Debit/Credit" from a signed net.
- **Fix:** add `normal_balance` per account type; define balance sign from it; produce a proper trial balance (Î£ signed balances = 0).

### S6. Schema gaps
- No FKs from `journal_entries` to transactions/accounts; no `CHECK` on `accounts.type`.
- Idempotency keys are global; scope by `(caller_id, endpoint, key)`.
- `cause_id BIGINT` but claims are keyed by text `message_id`.
- No **value date** vs **recorded date**. Settlement/cut-off logic needs both. Add `effective_date DATE`.

### S7. Outbox worker
- A failure sets `status='failed'` **permanently**. The README promises retry/backoff; there's no `attempts`, `next_attempt_at` or dead-letter.
- Network I/O happens **inside** the row-locking transaction.
- Handlers only `slog`: "Dispatched net bank instruction" is a log line; the `/on_settle` callback is signed but never sent, and the signed headers are discarded.
- **Nothing ever inserts into `outbox`.**
- **Fix:** claim rows in a short tx (`status='processing'`, `locked_until`), do I/O outside the tx, then mark done/failed with attempts, exponential backoff and a DLQ. Consumers must be idempotent by event id (delivery is at-least-once).

---

## 6. Tier 3 â€” Domain accuracy (vs. October 2026 reality)

### D1. GST TCS is wrong by 2Ã—
- Default is `GSTTCSBps: 100` (1%). Since **10 July 2024** (CGST Notification 15/2024) the rate is **0.5%** (0.25% CGST + 0.25% SGST, or 0.5% IGST).
- Base: net value of taxable supplies **excluding GST, less returns**. You use `orderAmount âˆ’ commission âˆ’ BFF`; the platform's commission is its own fee and doesn't reduce the supplier's supply value.
- Model per-order **tax regime**: Section 52 TCS vs Section 9(5) (operator is deemed supplier, e.g. ride-hailing, restaurant, accommodation; no TCS). ONDC mobility and food categories hit this.

### D2. TDS: the section number moved and the no-PAN rate is wrong
- Section 194-O no longer exists. Since **1 April 2026** (Income-tax Act 2025) e-commerce TDS is **Section 393(1), Table Sl. 8(v)**, payment code **1035**.
- Rate: **0.1%** (correct since 1 Oct 2024). **No PAN/Aadhaar â†’ 5%.** Your comment says 100 bps.
- Individual/HUF sellers have a **â‚¹5 lakh/year** threshold (with PAN/Aadhaar). The engine models no thresholds; this needs per-seller cumulative state.
- `PanExempt` conflates "no PAN" with a Section 197 lower-deduction certificate (two different mechanisms), and `true` skips TDS entirely.
- **(verify with a CA)** whether GST shown separately is excluded from the TDS base, and the rounding rule under the new Act. You floor paise; the statute has its own rounding. Choose and document a policy.
- **Best fix:** effective-dated rules as data (U8). The 1 Oct 2024 and 1 Apr 2026 changes become your test cases.

### D3. Request signing is not ONDC-compatible
- ONDC uses **Ed25519 + BLAKE2b-512**: the signing string contains `digest: BLAKE-512=<base64>`, recomputed from the body, and every ONDC example I found carries it only via the `Authorization` header's `headers="(created) (expires) digest"`, with no separate `Digest` header. You use **SHA-256 with a separate `Digest` header**, so you can neither verify nor produce real ONDC-signed requests.
- ONDC publishes an official **`ondc-crypto-sdk-go`** (it validates `created <= now <= expires`). Use it instead of hand-rolling.
- Your verifier never checks `created`, never caps `expires - created`, has no replay cache, and the signed string doesn't bind method/path.
- `cmd/api` generates a random SA key and an **empty** registry, so `HISAAB_VERIFY_SIGNATURES=true` rejects every request.
- **Fix:** registry-lookup interface (static file for tests, ONDC registry in staging), test vectors from the ONDC SDK.

### D4. Wire format
- Handlers accept bespoke JSON (`np_pair_id`, `order_amount_paise`). Real Beckn/RSF payloads carry amounts as decimal strings in the protocol's message model **(verify against the RSF 2.0 spec)**.
- Keep paise internally, but add an adapter layer: exact decimal-string â†’ paise (no floats; reject more than 2 decimals), validated against ONDC's published schemas. ONDC provides a mock server and an `rsf-bulk-utility`; test against them. Your `spec/ondc-pins.yaml` + drift script should actually fetch and validate.

### D5. Calendar
- Holidays are hand-typed for **2026 only**, with no cited source. From 2027-01-01 only Sundays and 2nd/4th Saturdays are non-business days: **every gazetted holiday silently becomes a business day.** The 2026 list looks incomplete, and bank holidays are state-specific.
- `SettlementCutOffDetermined` has **no cut-off time**: 23:59:59 on a business day stays the same day (EC-20 only asserts "not zero").
- `AddHoliday` mutates a map with no lock.
- **Fix:** holidays as a versioned data file with a source URL per entry; **fail closed** (return an error) for years not loaded; configurable cut-off with boundary tests (Tâˆ’1s, T, T+1s).

### D6. Netting is narrower than claimed
- README says two-level / multilateral netting. Implemented: one NP pair, forward flow only, one instruction.
- Real settlement also nets refunds and adjustments, and the SA nets across participants. Implement `net = Î£forward âˆ’ Î£refunds âˆ’ BFF` per pair, then multilateral netting across pairs, or reword the README.

---

## 7. Making it unique and useful

**The pitch after fixes:** *"An open conformance suite and reference engine for ONDC settlement, where the ledger is verified by crash, property and mutation testing."*

### U1. Build the orchestrator (turns a library into a system)
One transaction: validate â†’ store claim â†’ when both sides present (or at cut-off) â†’ match â†’ tax â†’ post to ledger â†’ insert instructions + outbox rows â†’ commit. `/settle` triggers it; the CLI and console read from the DB. Today these are well-tested-ish pure functions with no spine.

### U2. Ledger interface + one conformance suite, run on both implementations
```go
type Ledger interface {
    Post(ctx context.Context, req PostRequest) (*PostResult, error)
    Balance(ctx context.Context, account string) (AmountPaise, error)
    Entries(ctx context.Context) ([]JournalEntry, error)
}
```
Split into `ledger` (types + interface), `ledger/memory`, `ledger/pg`. One `ledgertest.Run(t, factory)` suite: idempotent replay, payload-mismatch, concurrent overdraft, crash-after-commit retry, overflow, hash collisions, 2027-dated posting. CI runs it against both, with `-race` and a `postgres:16` service.

### U3. Real crash testing (what you currently claim)
Wire `Trigger()` into `pg.Post` and the orchestrator at your 8 boundaries. Each test: arm a failpoint â†’ run â†’ recover/kill â†’ **open a fresh connection** â†’ assert from SQL: global zero-sum; no negative constrained balance; at most one posting per idempotency key; **outbox row exists iff ledger rows exist**. Then retry and assert exactly-once effect.

### U4. Stateful property tests (`rapid` state machine)
Operations: `Fund`, `Settle(random claims incl. duplicates/mismatches)`, `Refund`, `ReplayEarlierRequest`, `CrashAndRecover`. After each step check a model: conservation (money in = money out + held), `instructed â‰¤ collected`, no negative constrained account, replays return identical results. Shrinking hands you minimal counterexamples for the write-up.

### U5. Real mutation testing
Run `gremlins` or `go-mutesting` on `ledger`, `settlement`, `nptoolkit`, `recon`. Publish the genuine score and a "survivors I killed" list.

### U6. Merkle / hash chain, done properly
Per batch: leaf = SHA-256(0x00 â€– canonical entry bytes); node = SHA-256(0x01 â€– left â€– right) (domain separation); store root + `prev_root_hash` in `batch_roots`. `verify-merkle` **recomputes from the database**, exits non-zero on mismatch, and can print an inclusion proof. Test: tamper a row as superuser â†’ verification fails. Claim only "tamper-evident", nothing grander.

### U7. Publish ONDC settlement test vectors  *(the genuinely unique artifact)*
Your YAML scenario DSL is the seed. Make it language-agnostic: `rsf-vectors/*.yaml` with inputs (claims, balances, calendar, tax config) and expected outputs (matched/unmatched, instructions, ledger postings), each tied to a spec clause. Add an HTTP runner that replays vectors against **any** `/settle` implementation. Grow to ~40 vectors, including every adversarial case found here (duplicate orders, over-refund, tolerance, midnight cut-off, 2027 dates). Turn each entry in `AMBIGUITIES` into a public question to the ONDC community. Other implementers (e.g. an open-source mobility platform building its own RSF reconciliation) can then test against you. Nobody else offers this.

### U8. Effective-dated rules engine for statutory rates and fees
Rules as data: `rate`, `legal_ref`, `valid_from`, `valid_to`, `source_url`; `Compute(asOf, â€¦)`. Golden tests at 10 Jul 2024 (TCS), 1 Oct 2024 (TDS rate), 1 Apr 2026 (Section 393). The same engine can later carry the UPI MDR rules (announced, now deferred until after Diwali, date TBD).

### U9. Make the console real
Read APIs: `GET /v1/batches`, `/v1/batches/{id}`, `/v1/trial-balance`, `/v1/exceptions`. Server-side maker-checker (maker â‰  checker, written to `audit_log`) and server-side role checks. Delete every hardcoded array. Your Playwright setup can then run against a seeded DB.

### U10. Operations
`/readyz` actually pings the DB; Prometheus counters (postings, idempotent replays, rejects, invariant failures); structured logs with batch/message ids; graceful worker shutdown; a short runbook ("Inv-13 failed: what now").

### U11. Honest README
Replace badges with a table: **Claim | How verified | Command | Status**. Add "Known limitations" and "Not implemented." Reviewers trust this far more than green badges.

---

## 8. The order of work

**Phase 0 (1â€“2 days): stop the bleeding.** Remove or relabel C1â€“C9. Add `adversarial_test.go` on a branch (red on purpose).
*Done when:* nothing in the README is untrue.

**Phase 1 (week 1): money bugs.** B1, B2, B3, B5, B6; D1/D2 rates (config now, effective-dating later). Add `-race`, `go vet`, `golangci-lint`, `govulncheck` to CI.
*Done when:* all of `adversarial_test.go` is green under `-race`.

**Phase 2 (week 2): Postgres becomes real.** S1â€“S4, fix `Post` (Appendix B), ledger interface (U2), `postgres:16` in CI.
*Done when:* the conformance suite passes on memory **and** Postgres, and the âˆ’60 repro is a regression test.

**Phase 3 (week 3): the spine.** Orchestrator (U1), outbox fixes (S7), ONDC SDK signing (D3), calendar data (D5), wire adapter (D4). End-to-end test: POST `/settle` Ã—2 â†’ DB state â†’ `/on_settle` captured by a test server.
*Done when:* one command runs the full flow and asserts the ledger.

**Phase 4 (week 4): proof.** U3â€“U6, vectors (U7), write-up.
*Done when:* the write-up's every number can be regenerated by a `make` target.

**Cut-line if time is short:** Phases 0â€“2 + U3 + the write-up. That alone is a credible "correctness-first ledger and settlement" project.

---

## 9. Turning this into the job-winning proof

1. **Publish the audit.** *"I audited my own settlement engine: N bugs, 3 false claims, and the tests that now stop them."* Include the âˆ’60 repro SQL, the before/after, and the real mutation score. This outweighs any badge.
2. **Be able to explain every line of `Post()`:** why `FOR UPDATE` in ascending id order prevents deadlock; why the idempotency record must commit atomically with the postings; what happens on a crash between commit and ACK; at-least-once vs exactly-once; why integer paise; how you'd shard; how you'd audit.
3. **Own the AI question.** The repo shows heavy AI-agent scaffolding; interviewers *will* ask what you wrote versus generated. Do Phases 1â€“2 by hand, understanding each fix, and say so in the README: "built with AI assistance; correctness claims verified by these tests." The verified-bugs story is evidence of judgment, which is what they're hiring for.
4. **Practise the three questions:** "Two requests with the same idempotency key arrive at once; walk me through it." "A refund arrives for an order whose seller is insolvent." "The process dies after the bank call and before the callback."

---

## Appendix A â€” Reproduce my findings

```bash
# Money bugs + race
cp adversarial_test.go tests/adversarial/ && go test -race -v ./tests/adversarial/...

# Migration failure (any Postgres 13+)
psql -d scratch -v ON_ERROR_STOP=1 -f db/migrations/00001_init.sql
#  ERROR: unique constraint on partitioned table must include all partitioning columns
#  DETAIL: UNIQUE constraint on table "settlement_batches" lacks column "settlement_date"

# Overdraft race (the exact check-then-insert logic of Ledger.Post, no lock): run in TWO psql sessions at once
DO $$ DECLARE bal bigint; tx bigint; BEGIN
  SELECT COALESCE(SUM(CASE WHEN direction='credit' THEN amount_paise ELSE -amount_paise END),0)
    INTO bal FROM journal_entries WHERE account_id = <constrained account id>;
  PERFORM pg_sleep(1.5);
  IF bal - 80 < 0 THEN RAISE EXCEPTION 'ErrNegativeBalance'; END IF;
  INSERT INTO journal_transactions(idempotency_key,cause_type,cause_id)
    VALUES ('w-' || txid_current(),'instruction',1) RETURNING id INTO tx;
  INSERT INTO journal_entries(transaction_id,account_id,direction,amount_paise)
    VALUES (tx,<constrained id>,'debit',80),(tx,<other id>,'credit',80);
END $$;
-- Result with balance 100: both succeed, balance = -60.
```

---

## Appendix B â€” Reference fixes (all verified on PostgreSQL 16.15)

### B.1 Lock-free overdraft protection: the database enforces it
```sql
CREATE TABLE account_balances (
  account_id    BIGINT PRIMARY KEY REFERENCES accounts(id),
  constrained   BOOLEAN NOT NULL,
  balance_paise BIGINT  NOT NULL DEFAULT 0,
  CONSTRAINT constrained_non_negative CHECK (NOT constrained OR balance_paise >= 0)
);
-- in Post(), inside the transaction, for each touched account in ASCENDING account_id order:
UPDATE account_balances SET balance_paise = balance_paise + $delta WHERE account_id = $1;
-- SQLSTATE 23514 (check_violation) => map to ErrNegativeBalance
```
Tested: balance 100, two concurrent âˆ’80 updates â†’ exactly one succeeded, the other got `violates check constraint "constrained_non_negative"`; final balances 20/80. The `UPDATE` takes the row lock, so no `SUM` and no separate lock step. Keep `journal_entries` as the source of truth and run a periodic audit: `balance_paise == SUM(entries)`.

### B.2 `Post()` shape
```go
tx := begin(ctx)                        // READ COMMITTED
defer rollback(tx)

// 1. Idempotency INSIDE the tx. A concurrent duplicate blocks on the unique index
//    until the first tx commits (then conflicts) or rolls back (then it proceeds).
tag := tx.Exec(`INSERT INTO idempotency_keys(key, request_hash) VALUES ($1,$2)
                ON CONFLICT (key) DO NOTHING`, key, hash)
if tag.RowsAffected() == 0 {
    // row exists => it was committed together with its postings
    var h string; var resp []byte
    tx.QueryRow(`SELECT request_hash, response FROM idempotency_keys WHERE key=$1`, key).Scan(&h, &resp)
    if h != hash { return ErrIdempotencyKeyReused }
    return decode(resp)                 // exact replay
}
// 2. resolve accounts (names -> ids); 3. insert journal_transactions + entries;
// 4. apply balance deltas in ascending account_id order (B.1);
// 5. UPDATE idempotency_keys SET response=$resp; INSERT outbox rows;  <- Inv-11
commit
// retry the whole function on SQLSTATE 40001 / 40P01 (bounded, with jitter)
```
Because the key row, postings and response commit atomically, the `in_progress` state disappears entirely; there's nothing to get stuck.

### B.3 Database-side double-entry check + TRUNCATE guard
```sql
CREATE OR REPLACE FUNCTION assert_balanced() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d bigint; c bigint;
BEGIN
  SELECT COALESCE(SUM(amount_paise) FILTER (WHERE direction='debit'),0),
         COALESCE(SUM(amount_paise) FILTER (WHERE direction='credit'),0)
    INTO d, c FROM journal_entries WHERE transaction_id = NEW.transaction_id;
  IF d <> c THEN
    RAISE EXCEPTION 'unbalanced journal transaction %: debit % credit %', NEW.transaction_id, d, c;
  END IF;
  RETURN NULL;
END $$;

CREATE CONSTRAINT TRIGGER journal_entries_balanced AFTER INSERT ON journal_entries
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_balanced();

CREATE OR REPLACE FUNCTION no_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'TRUNCATE of % prohibited', TG_TABLE_NAME; END $$;
CREATE TRIGGER journal_entries_no_truncate BEFORE TRUNCATE ON journal_entries
  FOR EACH STATEMENT EXECUTE FUNCTION no_truncate();
```
Tested on the partitioned table: the unbalanced transaction was rejected at commit, the balanced one accepted, `TRUNCATE` rejected. Also: run the app as a non-owner role with `UPDATE, DELETE, TRUNCATE` revoked (a trigger is not a permission boundary).

### B.4 Partition time bomb
```sql
CREATE TABLE journal_entries_default PARTITION OF journal_entries DEFAULT;
-- then pg_partman, or a scheduled job that pre-creates the next N months
```
Tested: a posting dated 2027-01-15 is accepted. Use the same pattern for `settlement_batches`.

### B.5 The `settlement_batches` fix
```sql
CREATE TABLE settlement_batches (
    id BIGSERIAL, np_pair_id TEXT NOT NULL, settlement_date DATE NOT NULL,
    run_id TEXT NOT NULL, /* ...other columns unchanged... */
    PRIMARY KEY (id, settlement_date),
    UNIQUE (run_id, settlement_date)
) PARTITION BY RANGE (settlement_date);
```
Applied and verified (all 18 tables created).
</USER_REQUEST>
<ADDITIONAL_METADATA>
The current local time is: 2026-10-30T14:59:29+05:30.
</ADDITIONAL_METADATA>
<USER_SETTINGS_CHANGE>
The user changed setting `Model Selection` from None to Gemini 3.8 Flash (High). No need to comment on this change if the user doesn't ask about it. If reporting what model you are, please use a human readable name instead of the exact string.
</USER_SETTINGS_CHANGE>