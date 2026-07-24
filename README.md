# order-mock-payment

Fintech take-home: order + mock-payment service with signature-verified webhook processing and file uploads.

## Overview

Users sign up, place orders, initiate payments through a pluggable gateway (mock in this repo), and receive provider callbacks via signed webhooks. Users can also attach files (invoices, receipts) to their orders. Everything is JWT-authenticated except signup, login, and the webhook (which is HMAC-signed).

## Architecture

Layered vertical slices behind a single composition root.

```
                             ┌─────────────────────────────┐
                             │        HTTP (Gin)           │
                             └──────────────┬──────────────┘
                                            │
                    ┌──────────┬────────────┴────────────┬────────────┐
                    ▼          ▼                         ▼            ▼
                 auth       order   payment  webhook  upload    middleware
                    │          │        │       │        │           │
                    └──────────┴────┬───┴───────┴────────┘           │
                                    ▼                                 │
                              Service layer  ◀──────  consumer-owned interfaces
                                    │
                                    ▼
                              Repository layer  (sqlx)
                                    │
                                    ▼
                              PostgreSQL 17
```

- **Vertical slices**: one package per domain (`auth`, `order`, `payment`, `webhook`, `upload`). Each contains `model`, `dto`, `errors`, `repository`, `service`, `handler`, `routes` + tests.
- **Consumer-owned interfaces**: every service/handler declares the interface it depends on; concrete impls satisfy structurally. No mocking frameworks.
- **No cyclic imports.** DAG: `payment`/`upload` depend on `order`; `webhook` depends on `payment`; `middleware` depends on `auth`; `server` composes everything.

## Folder structure

```
cmd/server/main.go              composition root
internal/
├── auth/                       signup, login, JWT, bcrypt hasher
├── cache/                      Redis client bootstrap
├── config/                     Viper loader + validation
├── database/                   sqlx over pgx/v5 bootstrap
├── httpresp/                   shared response envelope + cross-cutting codes
├── logger/                     slog handler factory
├── middleware/                 RequireAuth + RequireUserID + ParseIDParam
├── order/                      order CRUD + status transitions
├── payment/                    payment initiation + provider callback + MockGateway
├── server/                     Gin + net/http server with graceful shutdown
├── upload/                     multipart uploads + LocalStorage
└── webhook/                    HMAC signature verification + payment status updates
migrations/                     SQL migrations (golang-migrate format)
```

## Dependencies

| Purpose | Library |
|---|---|
| HTTP router | `github.com/gin-gonic/gin` |
| SQL access | `github.com/jmoiron/sqlx` |
| Postgres driver | `github.com/jackc/pgx/v5` (stdlib mode) |
| Redis client | `github.com/redis/go-redis/v9` |
| Config | `github.com/spf13/viper` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| Bcrypt | `golang.org/x/crypto/bcrypt` |
| UUIDs | `github.com/google/uuid` |
| Decimals | `github.com/shopspring/decimal` |
| Structured logging | `log/slog` (stdlib) |

## Requirements

- Go **1.25+**
- Docker + Docker Compose
- (Optional) `golang-migrate` CLI for `make migrate-*`
- (Optional) `golangci-lint` v2 for `make lint`

## Configuration

All configuration is via environment variables. A `.env` file is loaded automatically when present. See `.env.example` for defaults.

### Environment variables

| Variable | Required | Default | Purpose |
|---|:---:|---|---|
| `APP_ENV` |   | `development` | `development` or `production` |
| `HTTP_PORT` |   | `8080` | TCP port |
| `HTTP_READ_TIMEOUT` |   | `10s` | Request read timeout |
| `HTTP_WRITE_TIMEOUT` |   | `15s` | Response write timeout |
| `HTTP_IDLE_TIMEOUT` |   | `60s` | Keep-alive idle timeout |
| `HTTP_SHUTDOWN_TIMEOUT` |   | `30s` | Graceful shutdown deadline |
| `LOG_LEVEL` |   | `info` | `debug`/`info`/`warn`/`error` |
| `POSTGRES_HOST` | ✓ | `localhost` | |
| `POSTGRES_PORT` |   | `5432` | |
| `POSTGRES_USER` | ✓ | | |
| `POSTGRES_PASSWORD` | ✓ | | Special chars are URL-encoded automatically |
| `POSTGRES_DB` | ✓ | | Database name |
| `POSTGRES_SSLMODE` |   | `disable` | libpq sslmode |
| `POSTGRES_MAX_OPEN_CONNS` |   | `25` | Must be > 0 |
| `POSTGRES_MAX_IDLE_CONNS` |   | `5` | Must be ≤ open conns |
| `POSTGRES_CONN_MAX_LIFETIME` |   | `5m` | |
| `POSTGRES_CONN_MAX_IDLE_TIME` |   | `5m` | |
| `REDIS_ADDR` |   | `localhost:6379` | |
| `REDIS_PASSWORD` |   | | |
| `REDIS_DB` |   | `0` | |
| `REDIS_DIAL_TIMEOUT` |   | `5s` | |
| `JWT_SECRET` | ✓ | | ≥ 32 bytes, enforced at boot |
| `JWT_TTL` |   | `24h` | Token lifetime |
| `WEBHOOK_SECRET` | ✓ | | ≥ 32 bytes, HMAC-SHA256 key |
| `UPLOAD_BASE_DIR` |   | `./uploads` | Where uploaded files land |
| `UPLOAD_MAX_SIZE` |   | `5242880` | Bytes (5 MB default) |

Config is validated at startup — invalid values fail fast with an actionable message.

## Docker

The compose stack brings up Postgres 17, Redis 8, a one-shot migrator, and the app.

```bash
cp .env.example .env
docker compose up --build
```

Services and ports:

| Service | Host port | Notes |
|---|---|---|
| `postgres` | `127.0.0.1:5432` | Bound to loopback; named volume `postgres_data` |
| `redis` | `127.0.0.1:6379` | Bound to loopback; named volume `redis_data` |
| `migrate` | – | One-shot; runs before `app` starts (`service_completed_successfully`) |
| `app` | `0.0.0.0:8080` | Distroless nonroot image; uploads in named volume `uploads_data` |

Smoke test:

```bash
curl http://localhost:8080/            # {service, version, status}
curl http://localhost:8080/healthz     # liveness
curl http://localhost:8080/readyz      # readiness (Postgres + Redis reachable)
```

## Database migrations

Migrations use `golang-migrate` conventions (`NNNNNN_name.up.sql` / `.down.sql`).

```bash
make migrate-up    # apply all pending
make migrate-down  # roll back one step
```

Migration set:

1. `000001_extensions` — `pgcrypto`, `citext`
2. `000002_create_users` — user accounts + unique email index
3. `000003_create_orders` — orders with `(user_id, created_at desc)` index
4. `000004_create_payments` — payments with `UNIQUE(order_id)` for 1:1 relationship
5. `000005_create_uploads` — uploads with `order_id` index

## Running locally

```bash
make run                                # go run ./cmd/server
make build                              # produce ./bin/server
make test                               # go test -race -cover ./...
make lint                               # golangci-lint (v2)
make fmt / fmt-check / vet / tidy
```

## Running tests

Unit tests run with no external dependencies:

```bash
go test ./...
```

Integration tests are env-gated on `TEST_POSTGRES_DSN`; they exercise the real Postgres repositories:

```bash
TEST_POSTGRES_DSN=postgres://app:app@127.0.0.1:5432/order_mock_payment?sslmode=disable \
  go test ./internal/order/ ./internal/payment/ ./internal/upload/ -run PostgresRepository
```

## API overview

Response envelope for every endpoint:

```json
// success
{"success": true,  "data": { ... }}
// error
{"success": false, "error": {"code": "...", "message": "..."}}
```

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/` | – | Service metadata (smoke) |
| GET | `/healthz` | – | Liveness |
| GET | `/readyz` | – | Readiness (checks Postgres + Redis) |
| POST | `/api/v1/auth/signup` | – | Create user account |
| POST | `/api/v1/auth/login` | – | Get JWT for existing account |
| POST | `/api/v1/orders` | Bearer | Create an order |
| GET | `/api/v1/orders` | Bearer | List caller's orders |
| GET | `/api/v1/orders/:id` | Bearer | Get one order (404 if not owned) |
| PUT | `/api/v1/orders/:id` | Bearer | Full-replace update |
| DELETE | `/api/v1/orders/:id` | Bearer | Remove order |
| POST | `/api/v1/payments` | Bearer | Initiate a payment for one of caller's orders |
| GET | `/api/v1/payments/:id` | Bearer | Fetch payment metadata |
| POST | `/api/v1/uploads` | Bearer | Upload a file (multipart) attached to caller's order |
| GET | `/api/v1/uploads/:id` | Bearer | Fetch upload metadata |
| POST | `/webhooks/payment` | HMAC sig | Provider callback for payment status |

## Authentication flow

1. **Signup** `POST /api/v1/auth/signup` with `{email, password, name}`. Password is bcrypt-hashed at cost 12; email is normalized (`TrimSpace` + `ToLower`) and validated via `net/mail.ParseAddress`. Duplicate emails return 409 `EMAIL_ALREADY_EXISTS`.
2. **Login** `POST /api/v1/auth/login` with `{email, password}`. Any failure (unknown email, wrong password, malformed) returns a uniform 401 `INVALID_CREDENTIALS` — no enumeration signal.
3. Server issues an **HS256 JWT** with `sub` (user UUID), `email`, `iat`, `exp`. TTL is `JWT_TTL` (default 24h).
4. Client sends `Authorization: Bearer <token>` on every protected route. Middleware validates signature + expiration and stores `auth.Claims` in the request context. `user_id` is **never** read from the request body or query.

## Payment flow

```
POST /api/v1/payments  {order_id}
    ↓
  service.Create(userID, orderID)
    ↓
  orders.Get(userID, orderID)         ← ownership check (foreign → 404)
    ↓
  verify order.Status == "pending"    ← payable check
    ↓
  amount := order.Quantity * order.Price
    ↓
  gateway.CreatePayment(orderID, amount, currency)
    ↓
  repo.Create(payment)                ← UNIQUE(order_id) enforces 1:1
    ↓
  orders.AdvanceStatus(orderID, "pending_payment")
    ↓
  return payment
```

- `MockGateway` returns deterministic `PAY-000001, PAY-000002, ...` references (sync.Mutex-guarded counter). Real providers can drop in behind the `PaymentGateway` interface.
- Duplicate attempts return 409 `DUPLICATE_PAYMENT` (via the UNIQUE constraint).

## Webhook flow

`POST /webhooks/payment` (unauthenticated by JWT; signed by HMAC):

```
Headers: X-Signature: <hex hmac-sha256(body, WEBHOOK_SECRET)>
Body:    {"provider_reference": "PAY-000001", "status": "paid" | "failed"}
```

Flow:

1. Read raw body.
2. Verify HMAC-SHA256 signature using **`hmac.Equal`** (constant-time). Failure → 401.
3. Parse payload. Only `provider_reference` and `status` are trusted; amount/currency/user_id in the payload (if present) are ignored — see the security section.
4. `payment.Service.ApplyProviderCallback` looks up the payment by reference, validates the transition (`pending → paid` or `pending → failed` only), updates the payment, cascades the order status (`paid` or `payment_failed`).
5. Idempotency: if the payment is already in the target state, return 200 without side effects. `paid ↔ failed` transitions return 409 `INVALID_STATUS_TRANSITION`.

## Upload flow

`POST /api/v1/uploads` (multipart/form-data, JWT-protected):

```
Fields:
  order_id   (form)   UUID of the target order (must be owned)
  file       (part)   the file bytes
```

1. Verify the order exists and is owned by the caller.
2. Reject empty files and files > `UPLOAD_MAX_SIZE` (default 5 MB).
3. Sniff content type from the **actual first 512 bytes** via `http.DetectContentType` — the client's `Content-Type` is ignored. Accepted: `application/pdf`, `image/png`, `image/jpeg`.
4. Generate a server-side filename: `<uuid>.<ext>`. The client's original filename is discarded.
5. `LocalStorage.Save` writes to `<UPLOAD_BASE_DIR>/YYYY/MM/DD/<uuid>.<ext>`. Path-traversal defense: filename is validated (no `/`, no `\`, no `..`), then `filepath.Abs(fullPath)` must remain under `filepath.Abs(baseDir)`.
6. Metadata persisted in `uploads` table; `GET /api/v1/uploads/:id` returns it (JOIN with `orders` enforces ownership).

## Security decisions

| Concern | Mitigation |
|---|---|
| Password brute force | bcrypt cost 12; passwords ≤ 72 bytes enforced (bcrypt limit); no plaintext ever logged or returned. |
| JWT forgery | HS256 with ≥ 32-byte secret enforced at startup. Alg-confusion attack blocked (`Parse` rejects non-`*jwt.SigningMethodHMAC`). |
| Login enumeration | Uniform `INVALID_CREDENTIALS` for unknown-email / wrong-password / malformed-email; identical message. |
| ID enumeration | 404 (not 403) for foreign resources on all `GET`/`PUT`/`DELETE`. |
| SQL injection | Parameterized (`$N`) queries only; no dynamic SQL; no `SELECT *` (explicit columns). |
| Webhook forgery / replay | HMAC-SHA256 with `hmac.Equal`; unknown reference → 404; invalid transition → 409; idempotent duplicates → 200 no-op. |
| Password / hash leakage | DTOs deliberately omit `password_hash`; handler tests scan every response body for `password_hash` / `passwordhash` / raw password. |
| Path traversal on uploads | Filename generated server-side (`uuid.ext`); storage validates + `Abs()`-checks the final path stays under baseDir. |
| Container attack surface | Distroless static + `nonroot` UID; no shell inside image. |
| Dev-mode secret leakage | Postgres/Redis ports bound to `127.0.0.1` in compose (won't leak on café Wi-Fi). |
| Startup deadlock | Bounded 30 s startup context for DB/Redis init; unreachable dependency exits with a clear error instead of hanging. |
| Config drift | Load-time validation: secret length, port range, pool sizes, `APP_ENV` whitelist. |
| Panic recovery | `gin.CustomRecoveryWithWriter` routes panics through slog (not stderr) for a single-stream log pipeline. |

## Trade-offs

- **No transactions across payment + order updates.** The design (`summary.txt` line 620) called for wrapping payment create + order status advance in a single DB transaction. Not implemented in the current milestone set. In a production port, `payment.Service.Create` and `ApplyProviderCallback` would take a `*sqlx.Tx`. Currently a mid-flight failure between the payment insert and the order status update leaves split-brain state.
- **No dummy-bcrypt on login unknown-email.** Login's ambiguous-error property is enforced at the message level but not the timing level. A patient attacker could probably enumerate emails via response-time differences.
- **In-process mock provider.** No real HTTP call. Real integrations plug in behind `payment.PaymentGateway`.
- **No pagination on `GET /orders`.** Every order is returned; fine at the scales this take-home targets.
- **No rate limiting.** Design mentions Redis token-bucket on signup/login/webhook; not implemented.
- **No request-body cap on `POST /uploads`.** Service rejects oversized files but only *after* the multipart parser has run. A `http.MaxBytesReader` in a general middleware would cut this off at the socket.
- **In-memory JWT revocation.** No denylist; a stolen token is valid until `exp`. The token has a `jti`-shaped hole in the claims for future Redis-backed revocation.
- **Cache is wired but unused.** Redis is available; no read/write paths cache today.
- **File-per-milestone**: schema stays lean; only 5 indexes total across the whole schema, each justified by a specific query.

## Future improvements

Ordered by impact:

1. **Wrap payment + order status updates in a database transaction.** Highest correctness win.
2. **Rate limiting middleware** (Redis token bucket) on `/signup`, `/login`, and `/webhooks/payment`.
3. **Request-ID + structured access-log middleware** for correlating panics/errors across requests.
4. **`http.MaxBytesReader` on `/uploads`.**
5. **Dummy bcrypt on login unknown-email** to close the timing side-channel.
6. **Metrics (Prometheus) + tracing (OTel)** at handler + service boundaries.
7. **Pagination on `GET /orders`.**
8. **Cache list-orders in Redis** (design already has key schema).
9. **JWT `jti` + Redis denylist** for revocation.
10. **CI workflow** (fmt-check, vet, lint, test, integration, docker build).
11. **API docs** (OpenAPI) generated from handler comments.
12. **E2E and fuzz tests.**
