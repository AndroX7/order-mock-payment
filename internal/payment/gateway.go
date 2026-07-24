package payment

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type GatewayPayment struct {
	Provider  string
	Reference string
}

type PaymentGateway interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amount decimal.Decimal, currency string) (GatewayPayment, error)
}

type MockGateway struct {
	mu      sync.Mutex
	counter uint64
}

func NewMockGateway() *MockGateway {
	return &MockGateway{}
}

const mockProviderName = "mock"

func (g *MockGateway) CreatePayment(_ context.Context, _ uuid.UUID, _ decimal.Decimal, _ string) (GatewayPayment, error) {
	g.mu.Lock()
	g.counter++
	n := g.counter
	g.mu.Unlock()
	return GatewayPayment{
		Provider:  mockProviderName,
		Reference: fmt.Sprintf("PAY-%06d", n),
	}, nil
}

var _ PaymentGateway = (*MockGateway)(nil)
