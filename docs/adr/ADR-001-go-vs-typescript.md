# ADR-001: Go vs TypeScript for Ledger Core & SA Simulator

**Status:** Accepted  
**Date:** 2026-10-29  
**Decider:** Ubaid

## Context

The core of Hisaab requires:
1. Strict integer arithmetic (zero floating-point operations anywhere near money).
2. Predictable, high-throughput concurrency under hot-account contention.
3. First-class fault injection, property-based testing with shrinking, and native fuzzing.
4. Minimal memory overhead and fast startup in CI/CD environments.

## Decision

Use **Go 1.23+** for the Ledger Core (`internal/ledger`), Settlement Agency Simulator (`internal/settlement`), and Verification Harness (`internal/verification`). Use **TypeScript (Next.js 15)** exclusively for the Ops Console (`console`).

## Rationale

- **Numeric Safety:** Go provides explicit integer types (`int64`, `int32`) with compiler-enforced typing. In contrast, JavaScript/TypeScript's default `number` is an IEEE 754 float, creating severe risks of precision loss unless wrapped in external BigInt libraries at every layer.
- **Concurrency & Deadlock Safety:** Go's goroutines, channels, and built-in race detector (`go test -race`) provide robust primitives for simulating asynchronous Beckn callbacks and concurrent bank callbacks.
- **Industry Standard:** Settlement and ledger infrastructure roles at Razorpay, OpenFX, and global fintechs predominantly require Go or Java.
- **Tooling:** `pgx/v5` offers battle-tested PostgreSQL connection pooling, native COPY protocol support for batched inserts, and prepared statement caching. Native fuzzing (`go test -fuzz`) and property testing via `rapid` allow automatic test-case shrinking.

## Fallback Trigger (Week 2 Gate)

If by the end of Week 2 the vertical slice (M1.1–M1.4 with 3 invariants passing under race detector) cannot be stabilized in Go, the project will fall back to TypeScript + Kysely for the backend.

## Consequences

- Learning curve: Week 0 is dedicated to Go fundamentals and pgx exploration.
- Next.js console will interact with PostgreSQL via a dedicated read-only role or HTTP API.
