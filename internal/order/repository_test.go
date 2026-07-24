package order

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
// is set (typically pointing at the docker-compose Postgres instance).
//
//   TEST_POSTGRES_DSN='postgres://app:app@localhost:5432/order_mock_payment?sslmode=disable' \
//     go test ./internal/order/... -run PostgresRepository
//
// Each test isolates itself by creating a user row + a fresh set of orders.
// The users are cleaned up on completion via ON DELETE CASCADE on orders.

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

// seedUser inserts a fresh user row and returns its id. Registers cleanup
// to delete the user (which cascades to its orders).
func seedUser(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	email := fmt.Sprintf("test-%s@example.com", id.String())
	_, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, name)
		VALUES ($1, $2, $3, $4)
	`, id, email, "not-a-real-hash", "Test User")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func newOrder(userID uuid.UUID, symbol string) *Order {
	return &Order{
		UserID:   userID,
		Symbol:   symbol,
		Side:     SideBuy,
		Quantity: decimal.RequireFromString("1.5"),
		Price:    decimal.RequireFromString("30000"),
		Status:   StatusPending,
	}
}

func TestPostgresRepository_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userID := seedUser(t, db)
	o := newOrder(userID, "BTCUSD")

	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if o.ID == uuid.Nil {
		t.Error("ID not populated by Create")
	}
	if o.CreatedAt.IsZero() {
		t.Error("CreatedAt not populated by Create")
	}

	got, err := repo.GetByID(ctx, userID, o.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Symbol != "BTCUSD" || got.Side != SideBuy {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if !got.Quantity.Equal(o.Quantity) {
		t.Errorf("Quantity round-trip: got %s, want %s", got.Quantity, o.Quantity)
	}
}

func TestPostgresRepository_ListOnlyReturnsOwnOrders(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userA := seedUser(t, db)
	userB := seedUser(t, db)

	if err := repo.Create(ctx, newOrder(userA, "AAA")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newOrder(userA, "BBB")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newOrder(userB, "CCC")); err != nil {
		t.Fatal(err)
	}

	got, err := repo.List(ctx, userA)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, o := range got {
		if o.UserID != userA {
			t.Errorf("leaked order for user %v (expected %v)", o.UserID, userA)
		}
	}
}

func TestPostgresRepository_OwnershipOnGet(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userA := seedUser(t, db)
	userB := seedUser(t, db)

	o := newOrder(userA, "BTCUSD")
	if err := repo.Create(ctx, o); err != nil {
		t.Fatal(err)
	}

	// userB fetching userA's order must see ErrOrderNotFound (no distinct forbidden).
	_, err := repo.GetByID(ctx, userB, o.ID)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("cross-user GetByID err = %v, want ErrOrderNotFound", err)
	}
}

func TestPostgresRepository_Update_PreservesStatus(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userID := seedUser(t, db)
	o := newOrder(userID, "BTCUSD")
	if err := repo.Create(ctx, o); err != nil {
		t.Fatal(err)
	}

	o.Symbol = "ETHUSD"
	o.Side = SideSell
	o.Quantity = decimal.RequireFromString("2.5")
	o.Price = decimal.RequireFromString("2000")
	if err := repo.Update(ctx, o); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if o.Status != StatusPending {
		t.Errorf("Update changed Status: got %q, want %q", o.Status, StatusPending)
	}

	// Verify persisted state.
	got, err := repo.GetByID(ctx, userID, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "ETHUSD" || got.Side != SideSell {
		t.Errorf("update did not persist: %+v", got)
	}
}

func TestPostgresRepository_Update_OwnershipEnforced(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userA := seedUser(t, db)
	userB := seedUser(t, db)

	o := newOrder(userA, "BTCUSD")
	if err := repo.Create(ctx, o); err != nil {
		t.Fatal(err)
	}

	// Attempt update as userB — must fail with ErrOrderNotFound.
	attack := *o
	attack.UserID = userB
	attack.Symbol = "HACKED"
	if err := repo.Update(ctx, &attack); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("cross-user Update err = %v, want ErrOrderNotFound", err)
	}

	// Confirm nothing changed.
	got, err := repo.GetByID(ctx, userA, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "BTCUSD" {
		t.Errorf("row was mutated: symbol = %q", got.Symbol)
	}
}

func TestPostgresRepository_Delete_OwnershipEnforced(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	userA := seedUser(t, db)
	userB := seedUser(t, db)

	o := newOrder(userA, "BTCUSD")
	if err := repo.Create(ctx, o); err != nil {
		t.Fatal(err)
	}

	// userB attempt.
	if err := repo.Delete(ctx, userB, o.ID); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("cross-user Delete err = %v, want ErrOrderNotFound", err)
	}
	// Row still exists.
	if _, err := repo.GetByID(ctx, userA, o.ID); err != nil {
		t.Errorf("row was deleted despite ownership failure: %v", err)
	}

	// Real owner deletes cleanly.
	if err := repo.Delete(ctx, userA, o.ID); err != nil {
		t.Fatalf("owner Delete: %v", err)
	}
	// Row gone.
	if _, err := repo.GetByID(ctx, userA, o.ID); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("post-delete Get err = %v, want ErrOrderNotFound", err)
	}
}
