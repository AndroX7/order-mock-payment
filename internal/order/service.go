package order

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const maxSymbolLen = 20

// Service holds business rules: validation, defaults, and repository
// composition. Every method takes an authenticated userID so ownership
// is a mandatory input, not something derivable from the request body.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request and persists a new order owned by userID.
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

// Get returns the order if it exists and is owned by userID. Otherwise
// ErrOrderNotFound (never a distinct "forbidden" error — prevents
// ID enumeration).
func (s *Service) Get(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	return s.repo.GetByID(ctx, userID, orderID)
}

// List returns all orders owned by userID, newest first.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*Order, error) {
	return s.repo.List(ctx, userID)
}

// Update replaces the mutable fields of an order owned by userID.
// status is server-controlled and never touched here.
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

// Delete removes the order if it exists and is owned by userID.
func (s *Service) Delete(ctx context.Context, userID, orderID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, orderID)
}

// UpdateStatus mutates the order's status. Intended for cross-module
// callers (payment) to advance the order lifecycle. Ownership is
// enforced via the userID filter in the repository query.
func (s *Service) UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error {
	return s.repo.UpdateStatus(ctx, userID, orderID, status)
}

// --- validation ---

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
