.PHONY: run build test lint fmt vet tidy migrate-up migrate-down migrate-install

APP_NAME := order-mock-payment
DB_URL   := postgres://postgres:postgres@localhost:5432/order_mock_payment?sslmode=disable

run:
	go run ./cmd/server

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(APP_NAME) ./cmd/server

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

migrate-install:
	@echo "brew install golang-migrate   # macOS"
	@echo "or: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
