package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

// Default currency. Orders currently have no currency column; a real
// multi-currency flow would carry it on the order itself.
const defaultCurrency = "USD"

// OrderService is the minimal order-domain surface the payment service
// needs. Consumer-owned interface; *order.Service satisfies it.
type OrderService interface {
	Get(ctx context.Context, userID, orderID uuid.UUID) (*order.Order, error)
	UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error
	AdvanceStatus(ctx context.Context, orderID uuid.UUID, status string) error
}

// Service orchestrates payment creation and retrieval. It owns ownership
// enforcement (via OrderService) and lifecycle transitions.
type Service struct {
	repo    Repository
	orders  OrderService
	gateway PaymentGateway
}

func NewService(repo Repository, orders OrderService, gateway PaymentGateway) *Service {
	return &Service{repo: repo, orders: orders, gateway: gateway}
}

// Create initiates a payment for orderID owned by userID.
//
//	load order (also verifies ownership)
//	→ check payable state
//	→ compute amount from order
//	→ gateway.CreatePayment
//	→ persist payment
//	→ transition order status to pending_payment
func (s *Service) Create(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error) {
	o, err := s.orders.Get(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, order.ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if o.Status != order.StatusPending {
		return nil, ErrOrderNotPayable
	}

	amount := o.Quantity.Mul(o.Price)
	if amount.Sign() <= 0 {
		return nil, ErrInvalidAmount
	}

	gw, err := s.gateway.CreatePayment(ctx, o.ID, amount, defaultCurrency)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}

	p := &Payment{
		OrderID:           o.ID,
		Provider:          gw.Provider,
		ProviderReference: gw.Reference,
		Amount:            amount,
		Currency:          defaultCurrency,
		Status:            StatusPending,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	if err := s.orders.UpdateStatus(ctx, userID, o.ID, order.StatusPendingPayment); err != nil {
		return nil, fmt.Errorf("update order status: %w", err)
	}
	return p, nil
}

// ApplyProviderCallback applies a provider status notification to the
// referenced payment and cascades the order status. Idempotent: a callback
// whose newStatus already matches current status is a no-op success.
//
// Only pending → paid and pending → failed are allowed. Any other
// transition returns ErrInvalidStatusTransition. Unknown references
// return ErrPaymentNotFound.
func (s *Service) ApplyProviderCallback(ctx context.Context, reference, newStatus string) (*Payment, error) {
	if newStatus != StatusPaid && newStatus != StatusFailed {
		return nil, ErrInvalidStatusTransition
	}
	p, err := s.repo.GetByProviderReference(ctx, reference)
	if err != nil {
		return nil, err
	}
	// Idempotent: duplicate callback in the same terminal state is not an error.
	if p.Status == newStatus {
		return p, nil
	}
	if p.Status != StatusPending {
		return nil, ErrInvalidStatusTransition
	}
	if err := s.repo.UpdateStatus(ctx, p.ID, newStatus); err != nil {
		return nil, err
	}
	p.Status = newStatus

	orderStatus := order.StatusPaid
	if newStatus == StatusFailed {
		orderStatus = order.StatusPaymentFailed
	}
	if err := s.orders.AdvanceStatus(ctx, p.OrderID, orderStatus); err != nil {
		return nil, fmt.Errorf("advance order status: %w", err)
	}
	return p, nil
}

// Get returns the payment if it exists and its underlying order is
// owned by userID. Foreign / non-existent payments both surface as
// ErrPaymentNotFound (no distinct "forbidden" — prevents enumeration).
func (s *Service) Get(ctx context.Context, userID, paymentID uuid.UUID) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if _, err := s.orders.Get(ctx, userID, p.OrderID); err != nil {
		if errors.Is(err, order.ErrOrderNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	return p, nil
}
