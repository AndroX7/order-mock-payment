package order

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// --- Test doubles ---

type fakeRepo struct {
	orders  map[uuid.UUID]*Order
	failErr error // if non-nil, every method returns this
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{orders: map[uuid.UUID]*Order{}}
}

func (r *fakeRepo) Create(_ context.Context, o *Order) error {
	if r.failErr != nil {
		return r.failErr
	}
	o.ID = uuid.New()
	o.CreatedAt = time.Now().UTC()
	o.UpdatedAt = o.CreatedAt
	// Copy stored so callers can't accidentally mutate the fake's state.
	stored := *o
	r.orders[o.ID] = &stored
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, userID, orderID uuid.UUID) (*Order, error) {
	if r.failErr != nil {
		return nil, r.failErr
	}
	o, ok := r.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, ErrOrderNotFound
	}
	out := *o
	return &out, nil
}

func (r *fakeRepo) List(_ context.Context, userID uuid.UUID) ([]*Order, error) {
	if r.failErr != nil {
		return nil, r.failErr
	}
	var out []*Order
	for _, o := range r.orders {
		if o.UserID == userID {
			cp := *o
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, o *Order) error {
	if r.failErr != nil {
		return r.failErr
	}
	existing, ok := r.orders[o.ID]
	if !ok || existing.UserID != o.UserID {
		return ErrOrderNotFound
	}
	existing.Symbol = o.Symbol
	existing.Side = o.Side
	existing.Quantity = o.Quantity
	existing.Price = o.Price
	existing.UpdatedAt = time.Now().UTC()
	// Populate the caller's struct with the DB-returned fields.
	o.Status = existing.Status
	o.CreatedAt = existing.CreatedAt
	o.UpdatedAt = existing.UpdatedAt
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, userID, orderID uuid.UUID) error {
	if r.failErr != nil {
		return r.failErr
	}
	o, ok := r.orders[orderID]
	if !ok || o.UserID != userID {
		return ErrOrderNotFound
	}
	delete(r.orders, orderID)
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, userID, orderID uuid.UUID, status string) error {
	if r.failErr != nil {
		return r.failErr
	}
	o, ok := r.orders[orderID]
	if !ok || o.UserID != userID {
		return ErrOrderNotFound
	}
	o.Status = status
	o.UpdatedAt = time.Now().UTC()
	return nil
}

var _ Repository = (*fakeRepo)(nil)

// --- helpers ---

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func validCreateReq() CreateOrderRequest {
	return CreateOrderRequest{
		Symbol:   "BTCUSD",
		Side:     SideBuy,
		Quantity: dec("1.5"),
		Price:    dec("30000"),
	}
}

// --- Create ---

func TestCreate_Cases(t *testing.T) {
	userID := uuid.New()
	repoBoom := errors.New("db down")

	cases := []struct {
		name      string
		req       CreateOrderRequest
		setupRepo func(*fakeRepo)
		wantErrIs error
	}{
		{name: "success", req: validCreateReq()},
		{
			name:      "empty symbol",
			req:       CreateOrderRequest{Symbol: "", Side: SideBuy, Quantity: dec("1"), Price: dec("1")},
			wantErrIs: ErrInvalidSymbol,
		},
		{
			name:      "lowercase symbol",
			req:       CreateOrderRequest{Symbol: "btcusd", Side: SideBuy, Quantity: dec("1"), Price: dec("1")},
			wantErrIs: ErrInvalidSymbol,
		},
		{
			name:      "symbol too long",
			req:       CreateOrderRequest{Symbol: strings.Repeat("A", 21), Side: SideBuy, Quantity: dec("1"), Price: dec("1")},
			wantErrIs: ErrInvalidSymbol,
		},
		{
			name:      "invalid side",
			req:       CreateOrderRequest{Symbol: "BTCUSD", Side: "HOLD", Quantity: dec("1"), Price: dec("1")},
			wantErrIs: ErrInvalidSide,
		},
		{
			name:      "quantity zero",
			req:       CreateOrderRequest{Symbol: "BTCUSD", Side: SideBuy, Quantity: dec("0"), Price: dec("1")},
			wantErrIs: ErrInvalidQuantity,
		},
		{
			name:      "quantity negative",
			req:       CreateOrderRequest{Symbol: "BTCUSD", Side: SideBuy, Quantity: dec("-1"), Price: dec("1")},
			wantErrIs: ErrInvalidQuantity,
		},
		{
			name:      "price negative",
			req:       CreateOrderRequest{Symbol: "BTCUSD", Side: SideBuy, Quantity: dec("1"), Price: dec("-0.01")},
			wantErrIs: ErrInvalidPrice,
		},
		{
			name:      "repository failure",
			req:       validCreateReq(),
			setupRepo: func(r *fakeRepo) { r.failErr = repoBoom },
			wantErrIs: repoBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tc.setupRepo != nil {
				tc.setupRepo(repo)
			}
			svc := NewService(repo)

			got, err := svc.Create(context.Background(), userID, tc.req)

			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if got.ID == uuid.Nil {
					t.Error("ID not populated")
				}
				if got.UserID != userID {
					t.Errorf("UserID = %v, want %v", got.UserID, userID)
				}
				if got.Status != StatusPending {
					t.Errorf("Status = %q, want %q", got.Status, StatusPending)
				}
				return
			}
			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
			}
			if got != nil {
				t.Errorf("got user %+v on error, want nil", got)
			}
		})
	}
}

// --- Get ---

func TestGet_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	repo := newFakeRepo()
	svc := NewService(repo)

	own, err := svc.Create(context.Background(), userA, validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(context.Background(), userB, validCreateReq())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("owner can fetch", func(t *testing.T) {
		got, err := svc.Get(context.Background(), userA, own.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != own.ID {
			t.Errorf("ID = %v, want %v", got.ID, own.ID)
		}
	})

	t.Run("non-owner sees not found", func(t *testing.T) {
		_, err := svc.Get(context.Background(), userA, other.ID)
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("unknown id returns not found", func(t *testing.T) {
		_, err := svc.Get(context.Background(), userA, uuid.New())
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("repository failure surfaces", func(t *testing.T) {
		boom := errors.New("db down")
		repo.failErr = boom
		defer func() { repo.failErr = nil }()
		_, err := svc.Get(context.Background(), userA, own.ID)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}

// --- List ---

func TestList_OwnershipAndErrors(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	repo := newFakeRepo()
	svc := NewService(repo)

	_, _ = svc.Create(context.Background(), userA, validCreateReq())
	_, _ = svc.Create(context.Background(), userA, validCreateReq())
	_, _ = svc.Create(context.Background(), userB, validCreateReq())

	t.Run("returns only own orders", func(t *testing.T) {
		got, err := svc.List(context.Background(), userA)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("len = %d, want 2", len(got))
		}
		for _, o := range got {
			if o.UserID != userA {
				t.Errorf("order for wrong user: %v", o.UserID)
			}
		}
	})

	t.Run("empty for user with no orders", func(t *testing.T) {
		got, err := svc.List(context.Background(), uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("repository failure surfaces", func(t *testing.T) {
		boom := errors.New("db down")
		repo.failErr = boom
		defer func() { repo.failErr = nil }()
		_, err := svc.List(context.Background(), userA)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}

// --- Update ---

func TestUpdate_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	repo := newFakeRepo()
	svc := NewService(repo)

	own, _ := svc.Create(context.Background(), userA, validCreateReq())
	other, _ := svc.Create(context.Background(), userB, validCreateReq())

	validUpdate := UpdateOrderRequest{
		Symbol:   "ETHUSD",
		Side:     SideSell,
		Quantity: dec("2"),
		Price:    dec("2000"),
	}

	t.Run("owner update succeeds and preserves status", func(t *testing.T) {
		got, err := svc.Update(context.Background(), userA, own.ID, validUpdate)
		if err != nil {
			t.Fatal(err)
		}
		if got.Symbol != "ETHUSD" || got.Side != SideSell {
			t.Errorf("update did not apply: %+v", got)
		}
		if got.Status != StatusPending {
			t.Errorf("status changed: got %q, want %q", got.Status, StatusPending)
		}
	})

	t.Run("non-owner update sees not found", func(t *testing.T) {
		_, err := svc.Update(context.Background(), userA, other.ID, validUpdate)
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("unknown id returns not found", func(t *testing.T) {
		_, err := svc.Update(context.Background(), userA, uuid.New(), validUpdate)
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("invalid input rejected before repo call", func(t *testing.T) {
		bad := validUpdate
		bad.Quantity = dec("0")
		_, err := svc.Update(context.Background(), userA, own.ID, bad)
		if !errors.Is(err, ErrInvalidQuantity) {
			t.Errorf("err = %v, want ErrInvalidQuantity", err)
		}
	})

	t.Run("repository failure surfaces", func(t *testing.T) {
		boom := errors.New("db down")
		repo.failErr = boom
		defer func() { repo.failErr = nil }()
		_, err := svc.Update(context.Background(), userA, own.ID, validUpdate)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}

// --- Delete ---

func TestDelete_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	repo := newFakeRepo()
	svc := NewService(repo)

	own, _ := svc.Create(context.Background(), userA, validCreateReq())
	other, _ := svc.Create(context.Background(), userB, validCreateReq())

	t.Run("non-owner cannot delete", func(t *testing.T) {
		err := svc.Delete(context.Background(), userA, other.ID)
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("owner deletes successfully", func(t *testing.T) {
		if err := svc.Delete(context.Background(), userA, own.ID); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Get(context.Background(), userA, own.ID)
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("after delete, err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("unknown id returns not found", func(t *testing.T) {
		err := svc.Delete(context.Background(), userA, uuid.New())
		if !errors.Is(err, ErrOrderNotFound) {
			t.Errorf("err = %v, want ErrOrderNotFound", err)
		}
	})

	t.Run("repository failure surfaces", func(t *testing.T) {
		boom := errors.New("db down")
		repo.failErr = boom
		defer func() { repo.failErr = nil }()
		err := svc.Delete(context.Background(), userA, uuid.New())
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}
