package payment

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GatewayPayment is the minimal response a gateway returns after
// accepting a payment. Provider identifies the concrete backend
// ("mock" for MockGateway); Reference is the provider-side id.
type GatewayPayment struct {
	Provider  string
	Reference string
}

// PaymentGateway abstracts the external payment provider. One
// implementation for M4 (MockGateway); real integrations plug in later
// without touching service or repository.
type PaymentGateway interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amount decimal.Decimal, currency string) (GatewayPayment, error)
}

// MockGateway is a deterministic, in-process PaymentGateway. It performs
// no I/O; references are generated from a monotonic counter (PAY-000001,
// PAY-000002, ...). The counter resets on process restart.
type MockGateway struct {
	mu      sync.Mutex
	counter uint64
}

func NewMockGateway() *MockGateway {
	return &MockGateway{}
}

const mockProviderName = "mock"

// CreatePayment returns a deterministic reference for the given order.
// The mock never returns an error; error paths are exercised in tests
// via a separate stub.
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
