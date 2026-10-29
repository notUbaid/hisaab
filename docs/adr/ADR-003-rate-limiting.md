---
# ADR-003: Rate Limiting Strategy
# Status: Proposed
# Date: 2026-10-30
# Decider: Ubaid
---

## Context

The SA simulator exposes HTTP endpoints (`/settle`, `/on_settle`) that must be rate-limited
to prevent abuse and demonstrate correct payment API behavior. Two approaches:

1. **In-process `sync.Map` + sliding window**
2. **Redis with token bucket**

## Decision

**In-process rate limiter** for the demo (single-instance Cloud Run), with a clear upgrade path to Redis.

## Rationale

The demo runs as a single Cloud Run instance. Redis adds operational complexity and cost for no benefit at this scale. The rate limiter interface is defined to allow Redis injection later.

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string, limit Rate) (bool, time.Duration, error)
}

// Demo implementation: in-process sliding window
type InProcessLimiter struct { mu sync.Mutex; windows map[string]*Window }

// Production upgrade: Redis token bucket
// type RedisLimiter struct { client *redis.Client }
```

## Consequences

- Multi-instance deployments would require Redis (document in runbook)
- The interface boundary means no code change when upgrading — only injection
- In-process state is lost on Cloud Run restart (acceptable for demo)

## Limits Defined

| Endpoint | Limit | Window |
|---|---|---|
| POST /settle | 10 | per NP per minute |
| POST /on_settle | 10 | per NP per minute |
| GET /console/* | 60 | per session per minute |
| POST /console/approve | 5 | per approver per minute |
