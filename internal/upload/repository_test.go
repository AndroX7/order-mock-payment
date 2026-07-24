package upload

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
//     go test ./internal/upload/... -run PostgresRepository

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

// seedUserAndOrder creates a fresh user + order row and registers cleanup.
// Returns (userID, orderID).
func seedUserAndOrder(t *testing.T, db *sqlx.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	userID := uuid.New()
	email := fmt.Sprintf("test-%s@example.com", userID.String())
	if _, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, name)
		VALUES ($1, $2, $3, $4)
	`, userID, email, "not-a-real-hash", "Test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	orderID := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO orders (id, user_id, symbol, side, quantity, price)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, orderID, userID, "BTCUSD", "BUY",
		decimal.RequireFromString("1"), decimal.RequireFromString("50")); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return userID, orderID
}

func newUpload(orderID uuid.UUID) *Upload {
	return &Upload{
		OrderID:     orderID,
		Filename:    uuid.NewString() + ".pdf",
		ContentType: "application/pdf",
		Size:        123,
		Path:        "test/x.pdf",
	}
}

func TestPostgresRepository_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userID, orderID := seedUserAndOrder(t, db)
	u := newUpload(orderID)

	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == uuid.Nil || u.CreatedAt.IsZero() {
		t.Error("Create did not populate id/created_at")
	}

	got, err := repo.GetByID(ctx, userID, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.OrderID != orderID {
		t.Errorf("OrderID mismatch")
	}
	if got.Filename != u.Filename || got.Path != u.Path {
		t.Errorf("payload roundtrip failed: %+v", got)
	}
}

func TestPostgresRepository_OwnershipOnGet(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	// userA owns the upload; userB attempts to read it.
	userA, orderA := seedUserAndOrder(t, db)
	userB, _ := seedUserAndOrder(t, db)

	u := newUpload(orderA)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	// Owner can fetch.
	if _, err := repo.GetByID(ctx, userA, u.ID); err != nil {
		t.Errorf("owner GetByID: %v", err)
	}
	// Non-owner gets uniform "not found".
	_, err := repo.GetByID(ctx, userB, u.ID)
	if !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("cross-user GetByID err = %v, want ErrUploadNotFound", err)
	}
}

func TestPostgresRepository_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("err = %v, want ErrUploadNotFound", err)
	}
}
