package order

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const maxSymbolLen = 20

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req CreateOrderRequest) (*Order, error) {
	if err := validateSymbol(req.Symbol); err != nil {
		return nil, err
	}
	if err := validateSide(req.Side); err != nil {
		return nil, err
	}
	if err := validateQuantity(req.Quantity); err != nil {
		return nil, err
	}
	if err := validatePrice(req.Price); err != nil {
		return nil, err
	}

	o := &Order{
		UserID:   userID,
		Symbol:   req.Symbol,
		Side:     req.Side,
		Quantity: req.Quantity,
		Price:    req.Price,
		Status:   StatusPending,
	}
	if err := s.repo.Create(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) Get(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	return s.repo.GetByID(ctx, userID, orderID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*Order, error) {
	return s.repo.List(ctx, userID)
}

func (s *Service) Update(ctx context.Context, userID, orderID uuid.UUID, req UpdateOrderRequest) (*Order, error) {
	if err := validateSymbol(req.Symbol); err != nil {
		return nil, err
	}
	if err := validateSide(req.Side); err != nil {
		return nil, err
	}
	if err := validateQuantity(req.Quantity); err != nil {
		return nil, err
	}
	if err := validatePrice(req.Price); err != nil {
		return nil, err
	}

	o := &Order{
		ID:       orderID,
		UserID:   userID,
		Symbol:   req.Symbol,
		Side:     req.Side,
		Quantity: req.Quantity,
		Price:    req.Price,
	}
	if err := s.repo.Update(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) Delete(ctx context.Context, userID, orderID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, orderID)
}

func (s *Service) UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error {
	return s.repo.UpdateStatus(ctx, userID, orderID, status)
}

func (s *Service) AdvanceStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	return s.repo.AdvanceStatus(ctx, orderID, status)
}

func validateSymbol(s string) error {
	if s == "" || len(s) > maxSymbolLen || s != strings.ToUpper(s) {
		return ErrInvalidSymbol
	}
	return nil
}

func validateSide(s string) error {
	if s != SideBuy && s != SideSell {
		return ErrInvalidSide
	}
	return nil
}

func validateQuantity(q decimal.Decimal) error {
	if q.Sign() <= 0 {
		return ErrInvalidQuantity
	}
	return nil
}

func validatePrice(p decimal.Decimal) error {
	if p.Sign() < 0 {
		return ErrInvalidPrice
	}
	return nil
}
