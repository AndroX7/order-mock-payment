package payment

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

type fakeOrderService struct {
	orders    map[uuid.UUID]*order.Order
	getErr    error
	updateErr error
	updates   []statusUpdate
}

type statusUpdate struct {
	userID  uuid.UUID
	orderID uuid.UUID
	status  string
}

func newFakeOrderService() *fakeOrderService {
	return &fakeOrderService{orders: map[uuid.UUID]*order.Order{}}
}

func (s *fakeOrderService) Get(_ context.Context, userID, orderID uuid.UUID) (*order.Order, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	o, ok := s.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, order.ErrOrderNotFound
	}
	cp := *o
	return &cp, nil
}

func (s *fakeOrderService) UpdateStatus(_ context.Context, userID, orderID uuid.UUID, status string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	o, ok := s.orders[orderID]
	if !ok || o.UserID != userID {
		return order.ErrOrderNotFound
	}
	o.Status = status
	s.updates = append(s.updates, statusUpdate{userID, orderID, status})
	return nil
}

func (s *fakeOrderService) AdvanceStatus(_ context.Context, orderID uuid.UUID, status string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	o, ok := s.orders[orderID]
	if !ok {
		return order.ErrOrderNotFound
	}
	o.Status = status
	s.updates = append(s.updates, statusUpdate{uuid.Nil, orderID, status})
	return nil
}

func (s *fakeOrderService) seedOrder(userID uuid.UUID, status string, qty, price string) *order.Order {
	o := &order.Order{
		ID:        uuid.New(),
		UserID:    userID,
		Symbol:    "BTCUSD",
		Side:      order.SideBuy,
		Quantity:  decimal.RequireFromString(qty),
		Price:     decimal.RequireFromString(price),
		Status:    status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.orders[o.ID] = o
	return o
}

type fakePaymentRepo struct {
	payments  map[uuid.UUID]*Payment
	byOrder   map[uuid.UUID]bool // simulates UNIQUE(order_id)
	createErr error
	getErr    error
	updateErr error
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{
		payments: map[uuid.UUID]*Payment{},
		byOrder:  map[uuid.UUID]bool{},
	}
}

func (r *fakePaymentRepo) Create(_ context.Context, p *Payment) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.byOrder[p.OrderID] {
		return ErrDuplicatePayment
	}
	p.ID = uuid.New()
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	cp := *p
	r.payments[p.ID] = &cp
	r.byOrder[p.OrderID] = true
	return nil
}

func (r *fakePaymentRepo) GetByID(_ context.Context, paymentID uuid.UUID) (*Payment, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	p, ok := r.payments[paymentID]
	if !ok {
		return nil, ErrPaymentNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakePaymentRepo) GetByProviderReference(_ context.Context, reference string) (*Payment, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	for _, p := range r.payments {
		if p.ProviderReference == reference {
			cp := *p
			return &cp, nil
		}
	}
	return nil, ErrPaymentNotFound
}

func (r *fakePaymentRepo) UpdateStatus(_ context.Context, paymentID uuid.UUID, status string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	p, ok := r.payments[paymentID]
	if !ok {
		return ErrPaymentNotFound
	}
	p.Status = status
	p.UpdatedAt = time.Now().UTC()
	return nil
}

type stubGateway struct {
	ref string
	err error
}

func (g stubGateway) CreatePayment(_ context.Context, _ uuid.UUID, _ decimal.Decimal, _ string) (GatewayPayment, error) {
	if g.err != nil {
		return GatewayPayment{}, g.err
	}
	if g.ref == "" {
		return GatewayPayment{Provider: "stub", Reference: "PAY-TEST-001"}, nil
	}
	return GatewayPayment{Provider: "stub", Reference: g.ref}, nil
}

var (
	_ Repository     = (*fakePaymentRepo)(nil)
	_ OrderService   = (*fakeOrderService)(nil)
	_ PaymentGateway = stubGateway{}
)

func TestCreate_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	gatewayBoom := errors.New("gateway down")
	repoBoom := errors.New("db down")

	cases := []struct {
		name         string
		setup        func(orders *fakeOrderService, repo *fakePaymentRepo, gw *stubGateway) uuid.UUID // returns orderID to pay for
		caller       uuid.UUID
		wantErrIs    error
		wantOrderTxn bool // whether order status should have been updated
	}{
		{
			name: "success",
			setup: func(o *fakeOrderService, _ *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				return o.seedOrder(userA, order.StatusPending, "2", "50").ID
			},
			caller:       userA,
			wantOrderTxn: true,
		},
		{
			name: "order not found",
			setup: func(_ *fakeOrderService, _ *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				return uuid.New()
			},
			caller:    userA,
			wantErrIs: ErrOrderNotFound,
		},
		{
			name: "foreign order",
			setup: func(o *fakeOrderService, _ *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				return o.seedOrder(userB, order.StatusPending, "1", "1").ID
			},
			caller:    userA,
			wantErrIs: ErrOrderNotFound,
		},
		{
			name: "order not payable",
			setup: func(o *fakeOrderService, _ *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				return o.seedOrder(userA, order.StatusPendingPayment, "1", "1").ID
			},
			caller:    userA,
			wantErrIs: ErrOrderNotPayable,
		},
		{
			name: "duplicate payment",
			setup: func(o *fakeOrderService, r *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				ord := o.seedOrder(userA, order.StatusPending, "1", "1")
				r.byOrder[ord.ID] = true // pre-mark: unique constraint would reject a second insert
				return ord.ID
			},
			caller:    userA,
			wantErrIs: ErrDuplicatePayment,
		},
		{
			name: "gateway failure",
			setup: func(o *fakeOrderService, _ *fakePaymentRepo, gw *stubGateway) uuid.UUID {
				gw.err = gatewayBoom
				return o.seedOrder(userA, order.StatusPending, "1", "1").ID
			},
			caller:    userA,
			wantErrIs: gatewayBoom,
		},
		{
			name: "repository failure",
			setup: func(o *fakeOrderService, r *fakePaymentRepo, _ *stubGateway) uuid.UUID {
				r.createErr = repoBoom
				return o.seedOrder(userA, order.StatusPending, "1", "1").ID
			},
			caller:    userA,
			wantErrIs: repoBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orders := newFakeOrderService()
			repo := newFakePaymentRepo()
			gw := &stubGateway{}
			orderID := tc.setup(orders, repo, gw)
			svc := NewService(repo, orders, *gw)

			got, err := svc.Create(context.Background(), tc.caller, orderID)

			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if got.ID == uuid.Nil {
					t.Error("payment.ID not populated")
				}
				if got.Status != StatusPending {
					t.Errorf("Status = %q, want %q", got.Status, StatusPending)
				}
				if !tc.wantOrderTxn {
					return
				}
				if len(orders.updates) != 1 {
					t.Fatalf("order updates = %d, want 1", len(orders.updates))
				}
				u := orders.updates[0]
				if u.userID != tc.caller || u.orderID != orderID || u.status != order.StatusPendingPayment {
					t.Errorf("bad status update: %+v", u)
				}
				return
			}
			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
			}
			if got != nil {
				t.Errorf("got payment on error: %+v", got)
			}
			if len(orders.updates) != 0 {
				t.Errorf("order status mutated on error path: %+v", orders.updates)
			}
		})
	}
}

func TestGet_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	orders := newFakeOrderService()
	repo := newFakePaymentRepo()
	svc := NewService(repo, orders, &stubGateway{})

	ownedOrder := orders.seedOrder(userA, order.StatusPendingPayment, "1", "1")
	ownedPayment := &Payment{OrderID: ownedOrder.ID, Provider: "stub", ProviderReference: "PAY-1",
		Amount: decimal.RequireFromString("1"), Currency: "USD", Status: StatusPending}
	if err := repo.Create(context.Background(), ownedPayment); err != nil {
		t.Fatal(err)
	}
	otherOrder := orders.seedOrder(userB, order.StatusPendingPayment, "1", "1")
	otherPayment := &Payment{OrderID: otherOrder.ID, Provider: "stub", ProviderReference: "PAY-2",
		Amount: decimal.RequireFromString("1"), Currency: "USD", Status: StatusPending}
	if err := repo.Create(context.Background(), otherPayment); err != nil {
		t.Fatal(err)
	}

	t.Run("owner fetches own payment", func(t *testing.T) {
		got, err := svc.Get(context.Background(), userA, ownedPayment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != ownedPayment.ID {
			t.Errorf("ID mismatch")
		}
	})

	t.Run("non-owner sees not found", func(t *testing.T) {
		_, err := svc.Get(context.Background(), userA, otherPayment.ID)
		if !errors.Is(err, ErrPaymentNotFound) {
			t.Errorf("err = %v, want ErrPaymentNotFound", err)
		}
	})

	t.Run("unknown payment id", func(t *testing.T) {
		_, err := svc.Get(context.Background(), userA, uuid.New())
		if !errors.Is(err, ErrPaymentNotFound) {
			t.Errorf("err = %v, want ErrPaymentNotFound", err)
		}
	})

	t.Run("repository failure surfaces", func(t *testing.T) {
		boom := errors.New("db down")
		repo.getErr = boom
		defer func() { repo.getErr = nil }()
		_, err := svc.Get(context.Background(), userA, ownedPayment.ID)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}

func TestMockGateway_ReferencesAreSequential(t *testing.T) {
	g := NewMockGateway()
	for i := 1; i <= 3; i++ {
		got, err := g.CreatePayment(context.Background(), uuid.New(), decimal.RequireFromString("1"), "USD")
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("PAY-%06d", i)
		if got.Reference != want {
			t.Errorf("iteration %d: got %q, want %q", i, got.Reference, want)
		}
		if got.Provider != mockProviderName {
			t.Errorf("provider = %q, want %q", got.Provider, mockProviderName)
		}
	}
}

func TestApplyProviderCallback_Cases(t *testing.T) {
	repoBoom := errors.New("db down")

	seed := func(orders *fakeOrderService, repo *fakePaymentRepo, userID uuid.UUID, currentStatus string) string {
		ord := orders.seedOrder(userID, order.StatusPendingPayment, "1", "1")
		ref := "PAY-" + uuid.NewString()[:8]
		p := &Payment{
			OrderID:           ord.ID,
			Provider:          "stub",
			ProviderReference: ref,
			Amount:            decimal.RequireFromString("1"),
			Currency:          "USD",
			Status:            currentStatus,
		}
		if err := repo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		return ref
	}

	cases := []struct {
		name          string
		setup         func(o *fakeOrderService, r *fakePaymentRepo) string // returns reference
		newStatus     string
		wantErrIs     error
		wantOrderTxn  string // expected order status after; empty means no txn
		wantPayStatus string // expected payment status after successful application
	}{
		{
			name: "paid transition applies to payment and order",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				return seed(o, r, uuid.New(), StatusPending)
			},
			newStatus:     StatusPaid,
			wantOrderTxn:  order.StatusPaid,
			wantPayStatus: StatusPaid,
		},
		{
			name: "failed transition applies to payment and order",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				return seed(o, r, uuid.New(), StatusPending)
			},
			newStatus:     StatusFailed,
			wantOrderTxn:  order.StatusPaymentFailed,
			wantPayStatus: StatusFailed,
		},
		{
			name: "duplicate callback in same terminal state is idempotent",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				return seed(o, r, uuid.New(), StatusPaid) // already paid
			},
			newStatus:     StatusPaid,
			wantPayStatus: StatusPaid, // unchanged, no error
		},
		{
			name: "transition from paid to failed is rejected",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				return seed(o, r, uuid.New(), StatusPaid)
			},
			newStatus: StatusFailed,
			wantErrIs: ErrInvalidStatusTransition,
		},
		{
			name: "unsupported target status is rejected",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				return seed(o, r, uuid.New(), StatusPending)
			},
			newStatus: "cancelled",
			wantErrIs: ErrInvalidStatusTransition,
		},
		{
			name: "unknown reference returns not found",
			setup: func(_ *fakeOrderService, _ *fakePaymentRepo) string {
				return "PAY-DOES-NOT-EXIST"
			},
			newStatus: StatusPaid,
			wantErrIs: ErrPaymentNotFound,
		},
		{
			name: "repository failure on lookup surfaces",
			setup: func(_ *fakeOrderService, r *fakePaymentRepo) string {
				r.getErr = repoBoom
				return "PAY-000001"
			},
			newStatus: StatusPaid,
			wantErrIs: repoBoom,
		},
		{
			name: "repository failure on update surfaces",
			setup: func(o *fakeOrderService, r *fakePaymentRepo) string {
				ref := seed(o, r, uuid.New(), StatusPending)
				r.updateErr = repoBoom
				return ref
			},
			newStatus: StatusPaid,
			wantErrIs: repoBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orders := newFakeOrderService()
			repo := newFakePaymentRepo()
			ref := tc.setup(orders, repo)
			svc := NewService(repo, orders, &stubGateway{})

			got, err := svc.ApplyProviderCallback(context.Background(), ref, tc.newStatus)

			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
				}
				if len(orders.updates) != 0 {
					t.Errorf("order was mutated on error path: %+v", orders.updates)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got == nil {
				t.Fatal("got nil payment on success")
			}
			if got.Status != tc.wantPayStatus {
				t.Errorf("payment.Status = %q, want %q", got.Status, tc.wantPayStatus)
			}
			if tc.wantOrderTxn != "" {
				if len(orders.updates) != 1 {
					t.Fatalf("order updates = %d, want 1", len(orders.updates))
				}
				if orders.updates[0].status != tc.wantOrderTxn {
					t.Errorf("order status = %q, want %q", orders.updates[0].status, tc.wantOrderTxn)
				}
			} else {
				if len(orders.updates) != 0 {
					t.Errorf("order mutated on idempotent path: %+v", orders.updates)
				}
			}
		})
	}
}
