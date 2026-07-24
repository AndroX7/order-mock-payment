package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/claudiovaldi/order-mock-payment/internal/payment"
)

const testWebhookSecret = "test-webhook-secret-at-least-32-bytes!"

func signBody(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

type fakePayments struct {
	calls []applyCall
	err   error
}

type applyCall struct {
	reference string
	status    string
}

func (p *fakePayments) ApplyProviderCallback(_ context.Context, reference, newStatus string) (*payment.Payment, error) {
	p.calls = append(p.calls, applyCall{reference, newStatus})
	if p.err != nil {
		return nil, p.err
	}
	return &payment.Payment{Status: newStatus}, nil
}

var _ PaymentService = (*fakePayments)(nil)

func TestHMACSignatureVerifier(t *testing.T) {
	v := NewHMACSignatureVerifier(testWebhookSecret)
	body := `{"provider_reference":"PAY-000001","status":"paid"}`
	good := signBody(testWebhookSecret, body)

	cases := []struct {
		name      string
		payload   string
		signature string
		wantErr   bool
	}{
		{"valid signature", body, good, false},
		{"empty signature", body, "", true},
		{"non-hex signature", body, "not-hex-!!!", true},
		{"wrong signature (different secret)", body, signBody("other-secret-32-bytes-different!!!!", body), true},
		{"tampered payload", body + " ", good, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Verify([]byte(tc.payload), tc.signature)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidSignature) {
					t.Errorf("err = %v, want ErrInvalidSignature", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		})
	}
}

func TestServiceProcess_Cases(t *testing.T) {
	repoBoom := errors.New("db down")

	validBody := `{"provider_reference":"PAY-000001","status":"paid"}`
	validSig := signBody(testWebhookSecret, validBody)

	cases := []struct {
		name        string
		payload     string
		signature   string
		paymentsErr error
		wantErrIs   error // nil == success
		wantCalls   int   // expected ApplyProviderCallback invocations
	}{
		{
			name:      "valid signature and paid transition",
			payload:   validBody,
			signature: validSig,
			wantCalls: 1,
		},
		{
			name:      "valid signature and failed transition",
			payload:   `{"provider_reference":"PAY-000002","status":"failed"}`,
			signature: signBody(testWebhookSecret, `{"provider_reference":"PAY-000002","status":"failed"}`),
			wantCalls: 1,
		},
		{
			name:      "invalid signature",
			payload:   validBody,
			signature: signBody("wrong-secret-32-bytes-!!!!!!!!!!!!", validBody),
			wantErrIs: ErrInvalidSignature,
		},
		{
			name:      "missing signature",
			payload:   validBody,
			signature: "",
			wantErrIs: ErrInvalidSignature,
		},
		{
			name:      "malformed JSON",
			payload:   `{"provider_reference":`,
			signature: signBody(testWebhookSecret, `{"provider_reference":`),
			wantErrIs: ErrInvalidRequest,
		},
		{
			name:      "missing provider_reference",
			payload:   `{"status":"paid"}`,
			signature: signBody(testWebhookSecret, `{"status":"paid"}`),
			wantErrIs: ErrInvalidRequest,
		},
		{
			name:      "unsupported status",
			payload:   `{"provider_reference":"PAY-000001","status":"weird"}`,
			signature: signBody(testWebhookSecret, `{"provider_reference":"PAY-000001","status":"weird"}`),
			wantErrIs: ErrInvalidStatus,
		},
		{
			name:        "payment not found",
			payload:     validBody,
			signature:   validSig,
			paymentsErr: payment.ErrPaymentNotFound,
			wantErrIs:   payment.ErrPaymentNotFound,
			wantCalls:   1,
		},
		{
			name:        "invalid transition",
			payload:     validBody,
			signature:   validSig,
			paymentsErr: payment.ErrInvalidStatusTransition,
			wantErrIs:   payment.ErrInvalidStatusTransition,
			wantCalls:   1,
		},
		{
			name:        "repository failure surfaces",
			payload:     validBody,
			signature:   validSig,
			paymentsErr: repoBoom,
			wantErrIs:   repoBoom,
			wantCalls:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakePayments{err: tc.paymentsErr}
			svc := NewService(NewHMACSignatureVerifier(testWebhookSecret), fp)

			err := svc.Process(context.Background(), []byte(tc.payload), tc.signature)

			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
				}
			}
			if len(fp.calls) != tc.wantCalls {
				t.Errorf("ApplyProviderCallback calls = %d, want %d", len(fp.calls), tc.wantCalls)
			}
		})
	}
}

func TestServiceProcess_DuplicateCallback(t *testing.T) {
	body := `{"provider_reference":"PAY-000001","status":"paid"}`
	sig := signBody(testWebhookSecret, body)

	fp := &fakePayments{}
	svc := NewService(NewHMACSignatureVerifier(testWebhookSecret), fp)

	for i := range 2 {
		if err := svc.Process(context.Background(), []byte(body), sig); err != nil {
			t.Fatalf("call %d: err = %v", i, err)
		}
	}
	if len(fp.calls) != 2 {
		t.Errorf("delegate calls = %d, want 2 (idempotency handled inside payment.Service)", len(fp.calls))
	}
}
