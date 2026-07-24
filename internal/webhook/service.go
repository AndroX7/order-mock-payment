package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/claudiovaldi/order-mock-payment/internal/payment"
)

type SignatureVerifier interface {
	Verify(payload []byte, signature string) error
}

type HMACSignatureVerifier struct {
	secret []byte
}

func NewHMACSignatureVerifier(secret string) *HMACSignatureVerifier {
	return &HMACSignatureVerifier{secret: []byte(secret)}
}

func (v *HMACSignatureVerifier) Verify(payload []byte, signature string) error {
	if signature == "" {
		return ErrInvalidSignature
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, v.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	if !hmac.Equal(provided, expected) {
		return ErrInvalidSignature
	}
	return nil
}

var _ SignatureVerifier = (*HMACSignatureVerifier)(nil)

type PaymentService interface {
	ApplyProviderCallback(ctx context.Context, reference, newStatus string) (*payment.Payment, error)
}

type Service struct {
	verifier SignatureVerifier
	payments PaymentService
}

func NewService(verifier SignatureVerifier, payments PaymentService) *Service {
	return &Service{verifier: verifier, payments: payments}
}

func (s *Service) Process(ctx context.Context, payload []byte, signature string) error {
	if err := s.verifier.Verify(payload, signature); err != nil {
		return err
	}
	var req CallbackRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return ErrInvalidRequest
	}
	if req.ProviderReference == "" {
		return ErrInvalidRequest
	}
	if req.Status != StatusPaid && req.Status != StatusFailed {
		return ErrInvalidStatus
	}
	if _, err := s.payments.ApplyProviderCallback(ctx, req.ProviderReference, req.Status); err != nil {
		return err
	}
	return nil
}
