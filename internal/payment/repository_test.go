package payment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
)

// Integration tests for PostgresRepository. Skipped unless TEST_POSTGRES_DSN
// is set.
//
//   TEST_POSTGRES_DSN='postgres://app:app@localhost:5432/order_mock_payment?sslmode=disable' \
//     go test ./internal/payment/... -run PostgresRepository

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping integration test")
	}
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

// seed a user + order and register cleanup. Returns the orderID.
func seedOrder(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	email := fmt.Sprintf("test-%s@example.com", userID.String())
	_, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, name)
		VALUES ($1, $2, $3, $4)
	`, userID, email, "not-a-real-hash", "Test")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	orderID := uuid.New()
	_, err = db.Exec(`
		INSERT INTO orders (id, user_id, symbol, side, quantity, price)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, orderID, userID, "BTCUSD", "BUY",
		decimal.RequireFromString("1"), decimal.RequireFromString("50"))
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return orderID
}

func newPayment(orderID uuid.UUID) *Payment {
	return &Payment{
		OrderID:           orderID,
		Provider:          "mock",
		ProviderReference: "PAY-000001",
		Amount:            decimal.RequireFromString("50"),
		Currency:          "USD",
		Status:            StatusPending,
	}
}

func TestPostgresRepository_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	orderID := seedOrder(t, db)
	p := newPayment(orderID)

	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == uuid.Nil || p.CreatedAt.IsZero() {
		t.Error("Create did not populate id/created_at")
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.OrderID != orderID {
		t.Errorf("OrderID = %v, want %v", got.OrderID, orderID)
	}
	if !got.Amount.Equal(p.Amount) {
		t.Errorf("Amount = %s, want %s", got.Amount, p.Amount)
	}
	if got.Provider != "mock" || got.ProviderReference != "PAY-000001" {
		t.Errorf("provider fields mismatch: %+v", got)
	}
}

func TestPostgresRepository_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Errorf("err = %v, want ErrPaymentNotFound", err)
	}
}

func TestPostgresRepository_DuplicatePayment(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	orderID := seedOrder(t, db)

	// First payment succeeds.
	first := newPayment(orderID)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Second payment for the same order violates UNIQUE(order_id).
	second := newPayment(orderID)
	second.ProviderReference = "PAY-000002"
	if err := repo.Create(ctx, second); !errors.Is(err, ErrDuplicatePayment) {
		t.Errorf("second Create err = %v, want ErrDuplicatePayment", err)
	}
}
