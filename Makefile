.PHONY: help run build test tidy fmt fmt-check vet lint migrate-up migrate-down

MIGRATE_DIR ?= ./migrations
POSTGRES_DSN ?= postgres://app:app@localhost:5432/order_mock_payment?sslmode=disable
BIN          ?= ./bin/server
VERSION      ?= dev

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the server locally
	go run ./cmd/server

build: ## Compile the server binary to $(BIN)
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/server

test: ## Run tests with race detector and coverage
	go test -race -cover ./...

tidy: ## Run go mod tidy
	go mod tidy

fmt: ## Format Go source in place
	gofmt -s -w .

fmt-check: ## Fail if any Go file is not gofmt-clean (CI-safe, non-mutating)
	@out=$$(gofmt -s -l .); \
	if [ -n "$$out" ]; then \
		echo "unformatted files:"; echo "$$out"; \
		exit 1; \
	fi

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run

migrate-up: ## Apply all pending migrations
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" up

migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" down 1
