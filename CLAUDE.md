# CLAUDE.md — Hisaab Project Intelligence

> This file is read by every AI agent (Antigravity, Claude Code, Gemini) on every turn.
> It is the single source of truth for how code is written, reviewed, and shipped in this repo.
> **No exceptions. No overrides. No "just this once."**

---

## 0. What This Project Is

Hisaab is a **money-correctness engine** for ONDC RSF 2.0 — a double-entry ledger, SA simulator, and failure-injection harness. Every rupee must be accounted for. Every invariant must hold under crashes, retries, and concurrent writes.

**This is a financial system. The bar is not "works on my machine." The bar is "correct under adversarial conditions."**

---

## 1. Cardinal Rules (Absolute, No Exceptions)

### 1.1 No Floats Near Money
```go
// BANNED
var amount float64 = 99.99
var total float32

// REQUIRED — always paise (int64)
type AmountPaise int64
const orderTotal AmountPaise = 9999 // ₹99.99
```

Any `float32` or `float64` variable within 3 call frames of a monetary calculation is a **CRITICAL** defect. Use `AmountPaise int64` always.

### 1.2 No Direct SQL String Concatenation
```go
// BANNED — SQL injection
query := "SELECT * FROM claims WHERE np_pair = '" + npPair + "'"

// REQUIRED — parameterized always
row := pool.QueryRow(ctx, "SELECT * FROM claims WHERE np_pair = $1", npPair)
```

All DB queries go through `sqlc`-generated code. Raw `fmt.Sprintf` with SQL fragments is a CRITICAL defect.

### 1.3 No Secrets in Code or Config Files
```
# BANNED — anywhere in the repo
POSTGRES_PASSWORD=mysecret
API_KEY=sk-xxxx
```
Secrets live in environment variables or Secret Manager only. The CI job `detect-secrets` will **block** the PR if any secret pattern is found.

### 1.4 No Placeholders or Stub Implementations
```go
// BANNED
func (l *Ledger) Post(ctx context.Context, req PostRequest) error {
    // TODO: implement later
    return nil
}

// BANNED
panic("not implemented")  // only allowed during Week 0 ramp, tagged with // RAMP-ONLY
```

Every function written must be fully implemented. Stubs are allowed **only** in `_test.go` mock files or explicitly tagged `// RAMP-ONLY`.

### 1.5 No Mutations on Ledger Tables
The immutability triggers in Postgres prevent `UPDATE`/`DELETE` on `journal_entries` and `journal_transactions`. **Never attempt to work around these triggers.** Fixes are reversing entries only.

### 1.6 Idempotency is Not Optional
Every write operation on the settlement path must be guarded by an idempotency key. Insert-first, not check-then-insert.

```go
// BANNED — race condition
exists, _ := checkIdempotencyKey(ctx, key)
if !exists {
    createIdempotencyKey(ctx, key)
    doWork(ctx)
}

// REQUIRED — insert-first with ON CONFLICT
_, err := pool.Exec(ctx, `
    INSERT INTO idempotency_keys (key, request_hash, status)
    VALUES ($1, $2, 'in_progress')
    ON CONFLICT (key) DO NOTHING
    RETURNING key
`, key, hash)
```

---

## 2. Code Quality Rules

### 2.1 Read Before You Write
Before editing any file:
1. Read the file in full
2. Read its direct callers (`grep -r "FunctionName"`)
3. Read the relevant test file

**Never assume a type, signature, or behavior.** Assumption = bug.

### 2.2 Tests Are Not Optional
Every public function in `internal/` must have a test. New code without tests = BLOCK in review.

Test naming convention:
```go
// Unit test
func TestLedger_Post_IdempotentDuplicate(t *testing.T)

// Property test
func TestLedger_InvariantZeroSum(t *rapid.T)

// Crash test
func TestLedger_CrashAfterDebit(t *testing.T)
```

### 2.3 Error Handling
```go
// BANNED — swallowing errors
result, _ := pool.Query(ctx, query)

// BANNED — generic panic
if err != nil { panic(err) }

// REQUIRED — explicit, typed error handling
result, err := pool.Query(ctx, query, args...)
if err != nil {
    return fmt.Errorf("ledger.Post: query journal_entries: %w", err)
}
```

All errors must be wrapped with context using `fmt.Errorf("...: %w", err)`. Never discard errors with `_`.

### 2.4 Logging Rules
```go
// BANNED — logs sensitive data
slog.Info("claim received", "ifsc", claim.BankIFSC, "account", claim.AccountNumber)

// BANNED — logs at wrong level (debug in production code)
fmt.Println("DEBUG: claim =", claim)

// REQUIRED — structured, sanitized
slog.Info("claim received",
    "claim_id", claim.ID,
    "np_pair", claim.NPPairID,
    "amount_paise", claim.TotalAmountPaise,
    // sensitive fields masked
)
```

**Fields that must NEVER appear in logs:** IFSC codes, bank account numbers, UPI IDs, tax IDs, any PII.

### 2.5 Time Handling
```go
// BANNED — UTC assumption
time.Now()
time.Parse("2006-01-02", dateStr)

// REQUIRED — IST-aware
ist, _ := time.LoadLocation("Asia/Kolkata")
now := time.Now().In(ist)

// All TIMESTAMPTZ fields stored in UTC in Postgres,
// converted to IST for any cut-off decision,
// zone recorded with the decision in audit_log
```

Every time-based cut-off decision must record its zone and calendar version in `audit_log`.

### 2.6 Concurrency Rules
```go
// BANNED — taking locks in arbitrary order (deadlock risk)
mu1.Lock()
mu2.Lock()  // order not guaranteed across goroutines

// REQUIRED — locks always in account-id order
// smaller ID first, always
lockOrder := sortAccountIDs(debitAccount, creditAccount)
acquireInOrder(ctx, lockOrder)
```

Account-level Postgres locks must always be acquired in ascending account-id order. Document any exception in an ADR.

---

## 3. Security Rules (Payment-Grade)

### 3.1 Beckn Request Signing
Every inbound `/settle` request must have its Ed25519 signature verified **before** any processing. Reject unsigned requests with HTTP 401.

```go
// REQUIRED at every /settle handler entry point
if err := requestSigner.Verify(r); err != nil {
    slog.Warn("request signature invalid", "message_id", msgID, "error", err)
    writeError(w, http.StatusUnauthorized, "invalid_signature")
    return
}
```

### 3.2 RBAC on Every Console Endpoint
Every Next.js API route must check the caller's role before executing:
```typescript
// REQUIRED — first line of every API route handler
const session = await getServerSession(authOptions)
if (!session || !hasRole(session, 'operator')) {
    return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
}
```

Roles: `viewer` (read-only), `operator` (can queue actions), `approver` (can approve maker-checker). No role inheritance upward.

### 3.3 Sensitive Field Masking
IFSC codes, bank account numbers, and UPI references must be masked in API responses to `viewer` role:
- Account numbers: show last 4 digits only (`****1234`)
- IFSC: show only bank prefix (`HDFC****`)
- UPI IDs: not returned at all to `viewer`

### 3.4 Rate Limiting
All HTTP endpoints have rate limits enforced at the middleware layer:

| Endpoint | Limit |
|---|---|
| `POST /settle` | 10 req/min per NP |
| `POST /on_settle` | 10 req/min per NP |
| `GET /console/*` | 60 req/min per session |
| `POST /console/approve` | 5 req/min per approver |

Exceeding rate limits returns HTTP 429 with `Retry-After` header.

### 3.5 No Real Data in Tests
CI gate: `grep -r --include="*.go" --include="*.yaml" --include="*.json"` for patterns matching:
- IFSC format: `[A-Z]{4}0[A-Z0-9]{6}`
- Account numbers: `\b[0-9]{9,18}\b`
- UPI IDs: `@[a-z]+`

Any match in `tests/` or `spec/` fails the build.

### 3.6 Audit Log for Every State Change
Every mutation through the settlement path writes a record to `audit_log`:
```go
// REQUIRED after every successful posting
auditLog.Record(ctx, AuditEntry{
    Actor:      actorFromCtx(ctx),
    Action:     "post_journal_entries",
    EntityType: "journal_transaction",
    EntityID:   strconv.FormatInt(txID, 10),
    Payload:    sanitizedSummary(req),
})
```

### 3.7 Maker-Checker for Manual Adjustments
Any manual adjustment (reconciliation entry, exception resolution) requires two distinct approvers. The system must reject same-user approve:

```go
if approval.ProposerID == approval.ApproverID {
    return ErrSelfApproval
}
```

---

## 4. Anti-AI-Slop Rules

These rules exist because AI-generated code has specific failure modes that differ from human-generated code. Every one of these has been seen in production AI-assisted projects.

### 4.1 Banned Patterns (Auto-rejected in review)

| Pattern | Why Banned |
|---|---|
| `// TODO: add error handling` | Incomplete code ships. Error handling is not optional. |
| `interface{}` or `any` for monetary amounts | Type erasure hides float smuggling |
| `json.Number` parsed directly to `float64` | Classic float precision loss on paise amounts |
| `time.Sleep` in non-test code | Fake async; use proper backoff with context cancellation |
| `os.Exit` outside of `main()` | Bypasses deferred cleanup and partial-write rollback |
| `recover()` without re-panicking or returning error | Silences panics; hides invariant violations |
| Returning `nil` error on DB constraint violation | Makes idempotency logic incorrect |
| `SELECT *` in any sqlc query | Schema changes silently break callers |
| Hardcoded `"UTC"` timezone | IST cut-offs will be wrong by 5h30m |
| `math/rand` without seed for test data | Non-reproducible failures; use `pgregory.net/rapid` |

### 4.2 AI Overconfidence Checks
Before claiming anything is complete, AI must:
1. Run the test suite (`make test-race`) and show the output
2. Show that the invariant checker passes (`make test-invariants`)
3. Show that `golangci-lint` returns exit code 0

Saying "this should work" without running it is not accepted.

### 4.3 Spec Citations Required
Every BRD scenario implementation must have a comment:
```go
// BRD §4.2 Edge Case 12: Insufficient balance — all matched orders rejected.
// Policy flag: partial_fill (default: false)
// Spec version: RSF-2.0-BRD-2024-08-02
// Ambiguity: BRD does not state whether partial fill is intended. See AMBIGUITIES.md#EC-12
```

AI must not silently guess spec behavior. Guesses go in `AMBIGUITIES.md` with a policy flag.

### 4.4 No Hallucinated APIs
Before using any external library function, verify it exists in the installed version:
```
go doc github.com/jackc/pgx/v5.Pool.QueryRow
```
If the documentation is not reachable, do not use the function. Do not assume it exists from training data.

---

## 5. Review Gates (Every PR Must Pass All)

### Automated Gates (CI — non-negotiable)
- [ ] `go build ./...` — zero errors
- [ ] `go test -race ./...` — zero data races, zero test failures
- [ ] `golangci-lint run` — zero linter errors
- [ ] `go vet ./...` — zero issues
- [ ] `goose -dir db/migrations ... validate` — all migrations valid
- [ ] `detect-secrets scan` — zero secrets found
- [ ] `grep-real-data-check` — zero real IFSC/account patterns in test fixtures
- [ ] `sqlc generate --dry-run` — zero schema drift
- [ ] Property tests: `make test-property` (50k checks) — zero violations
- [ ] Invariant checker: all 15 invariants green after scenario runs

### Manual Gates (PR reviewer)
- [ ] Every new public function has a doc comment
- [ ] Every BRD scenario has a spec citation comment
- [ ] Every policy flag has an entry in `AMBIGUITIES.md`
- [ ] No `float64` or `float32` anywhere in `internal/ledger/` or `internal/settlement/`
- [ ] Audit log entries written for every state change
- [ ] Error messages do not leak internal details (stack traces, table names, query text)

---

## 6. Module Ownership

| Directory | Owner (logical) | What goes here |
|---|---|---|
| `internal/ledger/` | Ledger core | Journal, accounts, posting functions, idempotency |
| `internal/settlement/` | SA simulator | Matching, balance check, netting, async flow |
| `internal/recon/` | Recon engine | Three-way recon, exception taxonomy |
| `internal/verification/` | Harness | Invariant checker, crash points, property tests |
| `internal/nptoolkit/` | NP toolkit | Claim builder, pre-flight validator |
| `internal/calendar/` | India-first | IST, RBI holidays, value dates |
| `internal/auth/` | Security | RBAC, session, request signing |
| `internal/crypto/` | Security | Field encryption, key rotation |
| `internal/telemetry/` | Observability | OTel setup, structured logging |
| `internal/middleware/` | API layer | Rate limiting, request signing verification |
| `spec/` | Spec-as-code | Edge cases YAML, ambiguity register, scenario DSL |
| `db/migrations/` | Schema | goose migrations (reviewed as Tier-2 changes) |
| `console/` | UI | Next.js, read-only role, RBAC |
| `docs/adr/` | Architecture | All significant decisions |

**Cross-module dependencies flow one way:** `settlement → ledger → (nothing internal)`. No cycles.

---

## 7. ADR Requirement

Any decision in the following categories **must** produce an ADR in `docs/adr/` before code is written:

- Language or framework choice
- Database schema design
- Concurrency or locking strategy
- Cryptographic primitive selection
- Any behavior where the BRD is ambiguous (gets an AMBIGUITIES.md entry too)
- Any rollback from the primary plan (Go → TypeScript, etc.)

ADR format: use `docs/adr/ADR-000-index.md` as the template registry.

---

## 8. Sensitive Data Policy

| Data type | Storage | In transit | In logs | In tests |
|---|---|---|---|---|
| IFSC codes | Encrypted at rest (AES-256-GCM) | TLS 1.3 | MASKED | SYNTHETIC only |
| Bank account numbers | Encrypted at rest | TLS 1.3 | MASKED (last 4) | SYNTHETIC only |
| UPI IDs | Encrypted at rest | TLS 1.3 | NEVER | NEVER |
| Settlement amounts | Plaintext BIGINT (paise) | TLS 1.3 | OK | OK (synthetic) |
| NP credentials | Secret Manager | TLS 1.3 | NEVER | STUBBED registry |

Encryption keys: rotated quarterly. Key IDs stored alongside ciphertext. Key rotation must not break decryption of existing records (envelope encryption).

---

## 9. Spec Discipline

- Every scenario in `tests/scenarios/` must reference a BRD section.
- Every ambiguity must have a policy flag in `spec/ambiguities.yaml`.
- **Silent guesses are banned.** If the spec doesn't say, it goes in the ambiguity register.
- The weekly CI job diffs ONDC's spec repos. New upstream commits open a GitHub Issue automatically.
- When spec changes, open an ADR and update affected scenarios before changing code.

---

## 10. The Correctness Report is the Product

The `docs/CORRECTNESS-REPORT.md` is the primary deliverable. Every invariant, attack, result, and bug found must appear there. It is generated in CI and never edited manually. If the report generation job fails, the build fails.

**If an invariant is violated in any test, the violation is never suppressed or skipped.** It is fixed or documented as a known limitation with a tracked issue.

---

## 11. What "Done" Means

A task is done when ALL of the following are true:
1. `make test-race` exits 0
2. `make lint` exits 0
3. The Correctness Report reflects the new scenario
4. The PR description links to the relevant BRD section
5. Any new ambiguity has an entry in `AMBIGUITIES.md`
6. Audit log entries are written and tested
7. The feature is observable via OpenTelemetry traces

"It compiles" is not done. "Tests pass locally" is not done. "I think it's correct" is not done.

---

_हिसाब साफ़ होना चाहिए। Every rupee must be accounted for._
