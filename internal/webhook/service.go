package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/claudiovaldi/order-mock-payment/internal/payment"
)

// SignatureVerifier verifies that a payload came from an authentic provider.
// Consumer-owned interface; HMACSignatureVerifier satisfies it.
type SignatureVerifier interface {
	Verify(payload []byte, signature string) error
}

// HMACSignatureVerifier verifies hex-encoded HMAC-SHA256 signatures.
// The signature is HMAC-SHA256(secret, raw request body).
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

// PaymentService is the minimal payment surface the webhook service needs.
// Consumer-owned; *payment.Service satisfies it.
type PaymentService interface {
	ApplyProviderCallback(ctx context.Context, reference, newStatus string) (*payment.Payment, error)
}

// Service handles the end-to-end webhook flow: signature verification,
// payload parsing, and delegation to the payment service.
type Service struct {
	verifier SignatureVerifier
	payments PaymentService
}

func NewService(verifier SignatureVerifier, payments PaymentService) *Service {
	return &Service{verifier: verifier, payments: payments}
}

// Process verifies the signature, parses the payload, and applies the
// callback via the payment service. Returns wire-mapped domain errors.
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
