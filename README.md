# order-mock-payment

Order and mock-payment service — take-home assignment.

## Tech stack

- **Language:** Go 1.25
- **HTTP router:** Gin
- **Database:** PostgreSQL 17 (via `sqlx` + `pgx/v5`)
- **Cache / rate limiter:** Redis 8 (via `go-redis/v9`)
- **Config:** Viper
- **Logging:** `log/slog`
- **Migrations:** `golang-migrate`

## Setup

_Coming soon — expanded in the final milestone._

Minimum required tooling:

- Go 1.25+
- Docker + Docker Compose
- `golang-migrate` CLI (only for local `make migrate-*`)

## Run

_Coming soon — expanded in the final milestone._

Quick start:

```bash
cp .env.example .env
docker compose up --build
```

Smoke test:

```bash
curl http://localhost:8080/            # service info
curl http://localhost:8080/healthz     # liveness
curl http://localhost:8080/readyz      # readiness (Postgres + Redis)
```
