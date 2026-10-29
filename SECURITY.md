# SECURITY.md — Hisaab Security Policy

> **This is a financial test system.** It never touches real money.  
> All data is synthetic. No real IFSC, account numbers, or UPI IDs are ever used.  
> Label: **BRD-faithful, not production-verified.**

---

## Threat Model

Hisaab's attack surface is the sandbox demo environment (Cloud Run + Cloud SQL, `asia-south1`).
It is a correctness testing tool, not a payment processor. Even so, it is operated at payment-service standards because:

1. It carries synthetic financial data that mirrors real formats
2. It demonstrates security practices that real NP engineers will copy
3. Weak security here signals weak security judgment everywhere

### Assets to Protect

| Asset | Sensitivity | Why |
|---|---|---|
| Settlement ledger state | High | Must be append-only; any mutation is a correctness violation |
| Audit log | High | Must be tamper-evident; operator action traceability |
| Synthetic NP credentials (test keys) | Medium | If real keys are ever accidentally used |
| Console session tokens | Medium | RBAC enforcement |
| Database connection string | Critical | Full read/write access to ledger |

### Threats We Model

1. **Confused-deputy via crafted `/settle` payloads** — unsigned or malformed requests that bypass matching logic
2. **Replay attacks** — duplicate `/settle` delivering twice without idempotency protection
3. **IDOR in console** — viewer role accessing approver endpoints
4. **Ledger mutation** — SQL injection or direct DB access attempting `UPDATE journal_entries`
5. **Secret exfiltration** — credentials leaked through logs, error messages, or headers
6. **Spec confusion** — an ambiguous BRD behavior exploited to produce an incorrect ledger state

---

## Security Controls Map

### Authentication and Authorization

| Control | Implementation | Location |
|---|---|---|
| Beckn Ed25519 request signing | Verify every inbound `/settle` before processing | `internal/middleware/signing.go` |
| RBAC (viewer/operator/approver) | JWT claims checked on every console route | `internal/auth/rbac.go` |
| Session cookies | `HttpOnly; Secure; SameSite=Strict` | `console/lib/auth.ts` |
| Maker-checker | Two distinct approvers; self-approval rejected | `internal/auth/makerchecker.go` |
| Registry key lookup | Public keys fetched from ONDC registry (stubbed in tests) | `internal/middleware/signing.go` |

### Input Validation

| Control | Implementation |
|---|---|
| Schema validation | ONDC log-validation-utility RSF endpoint before any processing |
| Paise-only amounts | `AmountPaise int64` type; `forbidigo` linter blocks `float64` |
| NP pair format | Regex validation on every claim |
| Message ID format | UUID v4 enforced; deduplicated via idempotency_keys |
| Payload size limit | 1MB max on `/settle` and `/on_settle` |

### Data Protection

| Data | At Rest | In Transit | In Logs |
|---|---|---|---|
| Synthetic IFSC codes | AES-256-GCM (envelope encryption) | TLS 1.3 | Masked (`HDFC****`) |
| Synthetic account numbers | AES-256-GCM | TLS 1.3 | Last 4 digits only |
| Settlement amounts | Plaintext BIGINT (paise) | TLS 1.3 | Allowed |
| Session tokens | httpOnly cookie | TLS 1.3 | Never |
| DB connection string | Secret Manager | — | Never |

### Ledger Immutability

```sql
-- Database-level: UPDATE/DELETE on core tables throw an exception
CREATE TRIGGER no_update_journal_entries
    BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

-- Application-level: only one stored function can write journal entries
-- REVOKE INSERT, UPDATE, DELETE ON journal_entries FROM app_role;
-- GRANT EXECUTE ON FUNCTION post_entries(...) TO app_role;
```

The application user (`hisaab_app`) has:
- `SELECT` on all tables
- `EXECUTE` on posting functions only (no direct `INSERT` on journal tables)
- `INSERT` on `idempotency_keys`, `outbox`, `audit_log`

### Rate Limiting

| Endpoint | Limit | Window |
|---|---|---|
| `POST /settle` | 10 requests | per NP, per minute |
| `POST /on_settle` | 10 requests | per NP, per minute |
| `GET /console/*` | 60 requests | per session, per minute |
| `POST /console/approve` | 5 requests | per approver, per minute |
| `POST /console/adjust` | 3 requests | per operator, per minute |

Rate limit state is in Redis (or `sync.Map` in the single-instance demo). Exceeding returns HTTP 429 with `Retry-After`.

### Secrets Management

```
# Secrets are NEVER in:
# - Source code (go, ts, yaml, json, .env files)
# - GitHub Actions env vars (except via ${{ secrets.* }})
# - Docker images
# - Log output
# - Error messages returned to callers

# Secrets live in:
# - Google Secret Manager (production demo)
# - GitHub Actions Secrets (CI)
# - Local .env.local (development, gitignored)
```

Required secrets:

| Secret name | Used for | Rotation |
|---|---|---|
| `HISAAB_DB_URL` | Postgres connection | On compromise |
| `HISAAB_SIGNING_KEY` | Outbound request signing | Quarterly |
| `HISAAB_FIELD_ENC_KEY` | Field encryption (IFSC, account) | Quarterly |
| `HISAAB_SESSION_SECRET` | Console JWT signing | Quarterly |
| `ONDC_VALIDATOR_URL` | ONDC schema validation endpoint | On URL change |

### HTTP Security Headers

Every API response includes:

```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'
Permissions-Policy: geolocation=(), microphone=(), camera=()
```

The SA simulator API additionally:
```
Cache-Control: no-store
Pragma: no-cache
```

### CORS Policy

The SA simulator API: **no CORS** (server-to-server only; browser access not expected).

The console API: allow only `https://hisaab.example.com` (configured via `HISAAB_ALLOWED_ORIGIN` secret).

---

## Security Testing Requirements

### Per-PR (Automated)
- `gosec` scan via golangci-lint — blocks on any G-category finding
- TruffleHog full history scan — blocks on any verified secret
- `check_real_data.py` — blocks on real IFSC/account patterns

### Per-Release (Manual)
- [ ] OWASP Top 10 review against the current attack surface
- [ ] Auth bypass attempts on all RBAC roles
- [ ] Idempotency replay attack simulation (`POST /settle` 3x with same message_id)
- [ ] Ledger mutation attempt via direct SQL (confirm triggers fire)
- [ ] Rate limit verification (confirm 429 after threshold)
- [ ] Request signing bypass attempt (tampered Ed25519 signature)
- [ ] Maker-checker self-approval attempt (confirm rejection)

### Invariant Security Checks (Part of Correctness Report)
Invariants 3, 8, and 14 are security-relevant:
- Invariant 3: effectively-once (no double-settle)
- Invariant 8: crash safety (no partial settlement without recovery)
- Invariant 14: callback integrity (no state change from late/duplicate callbacks)

These are attacked by the verification harness (M5) and reported in `CORRECTNESS-REPORT.md`.

---

## Vulnerability Reporting

This is an open-source sandbox project. There is no real money and no real user data.

If you find a security issue, please:
1. Open a GitHub Issue tagged `security`
2. Or email: `security@[your-domain].com`

We will acknowledge within 48 hours and fix within 14 days for any confirmed issue.

**Scope of responsible disclosure:**
- Ledger invariant violations achievable via the API
- Authentication/authorization bypass on the console
- Information leakage (real data patterns, stack traces, internal table names)
- Request signing bypass

**Out of scope:**
- Denial of service on the demo environment
- Vulnerabilities in ONDC's systems (report to ONDC directly)
- Theoretical attacks with no exploit path

---

## Compliance Notes

- **RBI PA Directions 2025:** Not applicable (no real payment processing).
- **ONDC NTS10 2.0.0:** Hisaab reuses ONDC's schemas but is not a certified participant.
- **Data Privacy (DPDPA 2023):** Synthetic data only. No personal data collected or processed.
- **PCI-DSS:** Not applicable (no cardholder data).

All compliance claims are bounded by: **"BRD-faithful, not production-verified."**

---

## Security ADRs

| ADR | Decision |
|---|---|
| ADR-002 | Field encryption: AES-256-GCM with envelope keys vs. Postgres `pgcrypto` |
| ADR-003 | Rate limiting: in-process `sync.Map` vs. Redis — decision based on deployment model |
| ADR-004 | Session management: JWT vs. opaque tokens for RBAC |
| ADR-005 | Request signing: Ed25519 verification library selection |

Each ADR is in `docs/adr/` and documents the security trade-offs explicitly.
