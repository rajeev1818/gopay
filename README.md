# GoPay

A production-grade digital wallet and payment processing backend written in Go. Built to demonstrate real-world backend engineering patterns — not a tutorial project.

---

## What This Is

GoPay is a multi-currency wallet service with peer-to-peer transfers, external bank top-ups, a double-entry accounting ledger, and async webhook delivery. Every design decision prioritizes correctness: money should never be created, lost, or duplicated.

---

## Architecture

```
cmd/server/          HTTP server bootstrap + graceful shutdown
internal/
  config/            Env-based configuration (caarlos0/env)
  domain/            Pure domain models — no DB tags, no HTTP coupling
  repository/        PostgreSQL queries (pgx/v5, native driver)
  service/           Core business logic — owns transactions and invariants
  integration/       External payment gateway abstraction + mock bank client
  middleware/        Auth, idempotency, rate limiting, structured logging
  worker/            Async webhook dispatcher (goroutine pool + channel queue)
migrations/          SQL migrations applied at container startup
```

Clean layered architecture: domain → repository → service → handler. Each layer depends only inward.

---

## Core Features

### Double-Entry Ledger

Every money movement writes two `ledger_entries` rows — one debit, one credit — inside the same serializable transaction. Each entry snapshots `balance_after` at write time. This makes auditing and reconciliation exact: you can reconstruct any wallet's balance at any point in history by replaying its ledger.

### ACID Transfers with Deadlock-Safe Locking

Peer-to-peer transfers use `SELECT ... FOR UPDATE` inside a **serializable PostgreSQL transaction**. To prevent deadlocks when two concurrent transfers touch the same pair of wallets in opposite directions, wallets are always locked in **lexicographic order by ID** — a standard technique for total-ordering lock acquisition.

```go
// Always lock in the same order regardless of transfer direction
ids := []string{req.SourceWalletID, req.DestWalletID}
sort.Strings(ids)
for _, id := range ids {
    wallet, err := s.walletRepo.LockWallet(ctx, tx, id)
    ...
}
```

Checks inside the transaction: both wallets unfrozen, currency matches, sufficient balance. Failure at any check rolls back atomically.

### Idempotent Payments

A DB-backed idempotency middleware intercepts all `POST`/`PUT` requests. Clients send an `Idempotency-Key` header; the middleware checks the `idempotency_keys` table for a matching key created within the last 24 hours. On a duplicate, it replays the original response (`X-Idempotent-Replayed: true`) without re-executing business logic. Safe to retry from any client.

### Circuit Breaker

The external bank client is wrapped in a hand-rolled circuit breaker:

| State | Behavior |
|---|---|
| **Closed** | Calls pass through; failures increment a counter |
| **Open** | All calls fail fast; no network attempt made |
| **Half-Open** | After `resetTimeout`, one probe call is allowed through |

On probe success → back to Closed. On probe failure → back to Open. This protects the payment service from cascading failures when the upstream bank is degraded.

### Async Webhook Dispatcher (Worker Pool)

Payment events are delivered to external endpoints asynchronously via a goroutine pool backed by a buffered channel:

```
Emit(event) → channel (capacity: 1000, non-blocking)
                    ↓
         N worker goroutines (fan-out)
                    ↓
         POST to each endpoint with:
           - HMAC-SHA256 signature  (X-Webhook-Signature)
           - Timestamp              (X-Webhook-Timestamp)
           - Exponential backoff retry (up to 3 attempts)
```

`Emit` uses a `select`/`default` to drop events when the queue is full rather than blocking the caller — backpressure is explicit, not hidden.

### Token-Bucket Rate Limiter

Per-IP rate limiter using the token bucket algorithm. Each remote address gets its own bucket (capacity + refill rate configurable). Implemented with a `sync.Mutex`-protected map. Returns HTTP 429 when the bucket is empty.

---

## Technology Choices

| Concern | Choice | Why |
|---|---|---|
| HTTP router | `go-chi/chi` | Lightweight, idiomatic, zero-allocation routing |
| PostgreSQL driver | `jackc/pgx/v5` | Native Go driver, no `database/sql` overhead, named arguments |
| JWT | `golang-jwt/jwt/v5` | Standard, maintained |
| Config | `caarlos0/env/v10` | Zero-boilerplate env parsing into typed structs |
| Password hashing | `golang.org/x/crypto/bcrypt` | Industry standard |
| Logging | `log/slog` | Structured JSON logging, stdlib, no extra dependency |

---

## Database Schema

```sql
users         — id (UUID), email (unique), kyc_status, password_hash
wallets       — id (UUID), user_id (FK), currency, balance (BIGINT), frozen
              — UNIQUE(user_id, currency), CHECK(balance >= 0)
transactions  — id (UUID), idempotency_key (UNIQUE), type, status,
              — source_wallet_id (nullable), dest_wallet_id, amount
              — CHECK(amount > 0), partner_reference
ledger_entries — id (UUID), transaction_id (FK), wallet_id (FK),
               — entry_type (debit/credit), amount, balance_after
```

Balances stored as `BIGINT` in the smallest currency unit (paise for INR, cents for USD) — no floating point anywhere near money.

---

## Middleware Stack

```
chi.RequestID         ← assign request ID
chi.Logger            ← chi's built-in logger
chi.Recoverer         ← panic recovery
RequestLogger         ← structured slog logging (method, path, status, duration_ms)
RateLimiter           ← token bucket per IP, HTTP 429 on exhaustion
AuthMiddleware        ← JWT Bearer validation, injects user_id into context
IdempotencyMiddleware ← DB-backed replay for POST/PUT
```

---

## Go Patterns Demonstrated

- **Interfaces for testability** — `PaymentGateway` interface abstracts the bank client; swap mock for real without touching service code
- **Context propagation** — timeouts and cancellation flow from HTTP handler through service to DB and external calls
- **Goroutines + channels** — webhook worker pool with a buffered queue and graceful drain
- **Serializable transactions** — used where consistency matters most (money movement)
- **Error wrapping** — `fmt.Errorf("...: %w", err)` throughout for stack-traceable error chains
- **Graceful shutdown** — `os.Signal` channel + `server.Shutdown(ctx)` with a 30-second drain window
- **Multi-stage Docker build** — builder on `golang:1.22-alpine`, runtime on `alpine:3.19`, results in a ~13 MB image

---

## Running Locally

**Prerequisites:** Docker + Docker Compose

```bash
git clone https://github.com/rajeev1818/gopay
cd gopay
docker-compose up --build
```

Migrations run automatically at container startup. The app starts on `:8080`.

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## Configuration

| Variable | Default | Required |
|---|---|---|
| `PORT` | `8080` | No |
| `DATABASE_URL` | — | Yes |
| `JWT_SECRET` | — | Yes |
| `ENVIRONMENT` | `development` | No |

---

## Status

The core infrastructure layer is complete and production-ready. HTTP route handlers are the next thing to wire up.

**Done:**
- Domain models + 4-table DB schema with constraints
- Payment service (Transfer, TopUp) with full ACID guarantees
- Double-entry ledger
- Circuit breaker (Closed / Open / Half-Open)
- Mock bank client with configurable failure rate + simulated latency
- Webhook dispatcher with HMAC-SHA256 signing + exponential backoff retry
- Auth, idempotency, rate limiter, and request logger middleware
- Multi-stage Docker build + Compose setup

**Next:**
- HTTP handlers for transfer, topup, wallet balance
- User registration + login (bcrypt + JWT issuance)
- Wiring everything into the chi router
