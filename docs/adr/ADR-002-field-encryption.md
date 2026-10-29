---
# ADR-002: Field Encryption for Sensitive Financial Identifiers
# Status: Proposed
# Date: 2026-10-30
# Decider: Ubaid
---

## Context

Hisaab stores synthetic IFSC codes, bank account numbers, and similar financial identifiers.
Even though this is a sandbox with synthetic data, we store these in production-like fashion
to demonstrate correct data handling — this is part of the hiring signal.

Two approaches:

1. **AES-256-GCM with envelope encryption (application-level)**
2. **Postgres `pgcrypto` / Transparent Data Encryption (database-level)**

## Decision

Use **AES-256-GCM envelope encryption at the application layer** in `internal/crypto/`.

## Rationale

| Factor | Application-level (chosen) | DB-level |
|---|---|---|
| Column-granular encryption | ✓ Encrypt only sensitive columns | ✗ Often all-or-nothing |
| Key rotation without DB downtime | ✓ Re-encrypt rows in background | ✗ Requires DB restart or complex procedure |
| Queryable (partial) | ✓ Store deterministic IV for equality checks if needed | ✓ |
| Cloud SQL support | ✓ Works with any Postgres | ✓ Requires pgcrypto extension |
| Portfolio signal | ✓ Shows you implement crypto correctly | Lower signal |
| Complexity | Medium | Low |

**Envelope encryption pattern:**
- A **Data Encryption Key (DEK)** is generated per record (or per NP)
- The DEK is encrypted with a **Key Encryption Key (KEK)** from Secret Manager
- Both the ciphertext and the encrypted DEK are stored
- Key rotation replaces only the KEK; existing DEKs are re-wrapped

**Implementation contract:**
```go
type FieldEncryptor interface {
    Encrypt(ctx context.Context, plaintext []byte) (ciphertext, encryptedDEK []byte, err error)
    Decrypt(ctx context.Context, ciphertext, encryptedDEK []byte) ([]byte, error)
    RotateKEK(ctx context.Context, newKEKID string) error
}
```

## Consequences

- Encrypted fields cannot be indexed for exact-match queries without a deterministic scheme
- Sort and range queries on encrypted fields are impossible
- Key rotation is a background job, not a blocking migration
- `internal/crypto/` is a security-critical package — review every change with the security checklist

## Alternatives Considered

- **pgcrypto:** Simpler but key management lives in the DB, not in Secret Manager. Harder to demonstrate envelope encryption in the portfolio.
- **No encryption:** Acceptable for a pure sandbox, but violates the "operate like a payment service" principle.
