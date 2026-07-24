package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

const defaultCurrency = "USD"

type OrderService interface {
	Get(ctx context.Context, userID, orderID uuid.UUID) (*order.Order, error)
	UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error
	AdvanceStatus(ctx context.Context, orderID uuid.UUID, status string) error
}

type Service struct {
	repo    Repository
	orders  OrderService
	gateway PaymentGateway
}

func NewService(repo Repository, orders OrderService, gateway PaymentGateway) *Service {
	return &Service{repo: repo, orders: orders, gateway: gateway}
}

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

func (s *Service) ApplyProviderCallback(ctx context.Context, reference, newStatus string) (*Payment, error) {
	if newStatus != StatusPaid && newStatus != StatusFailed {
		return nil, ErrInvalidStatusTransition
	}
	p, err := s.repo.GetByProviderReference(ctx, reference)
	if err != nil {
		return nil, err
	}
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
