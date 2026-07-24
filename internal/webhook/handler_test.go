package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
	"github.com/claudiovaldi/order-mock-payment/internal/payment"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestRouter(fp *fakePayments) *gin.Engine {
	r := gin.New()
	svc := NewService(NewHMACSignatureVerifier(testWebhookSecret), fp)
	rg := r.Group("/webhooks")
	NewHandler(svc).RegisterRoutes(rg)
	return r
}

func doWebhook(t *testing.T, r *gin.Engine, body string, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/payment", strings.NewReader(body))
	if sig != "" {
		req.Header.Set(SignatureHeader, sig)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseEnv(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	return out
}

func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, wantStatus, w.Body.String())
	}
	env := parseEnv(t, w.Body.Bytes())
	if s, _ := env["success"].(bool); s {
		t.Errorf("success = true, want false")
	}
	errObj, _ := env["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != wantCode {
		t.Errorf("code = %q, want %q", code, wantCode)
	}
}

func TestCallbackHandler(t *testing.T) {
	repoBoom := errors.New("db down")

	const validBody = `{"provider_reference":"PAY-000001","status":"paid"}`
	validSig := signBody(testWebhookSecret, validBody)

	const failedBody = `{"provider_reference":"PAY-000002","status":"failed"}`
	failedSig := signBody(testWebhookSecret, failedBody)

	cases := []struct {
		name        string
		body        string
		signature   string
		paymentsErr error
		wantStatus  int
		wantCode    string // empty when success
	}{
		{
			name:       "valid paid callback returns 200",
			body:       validBody,
			signature:  validSig,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid failed callback returns 200",
			body:       failedBody,
			signature:  failedSig,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid signature returns 401",
			body:       validBody,
			signature:  signBody("wrong-secret-32-bytes-not-real!!!!!", validBody),
			wantStatus: http.StatusUnauthorized,
			wantCode:   CodeInvalidSignature,
		},
		{
			name:       "missing signature returns 401",
			body:       validBody,
			signature:  "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   CodeInvalidSignature,
		},
		{
			name:       "malformed JSON returns 400",
			body:       `{"provider_reference":`,
			signature:  signBody(testWebhookSecret, `{"provider_reference":`),
			wantStatus: http.StatusBadRequest,
			wantCode:   httpresp.CodeInvalidRequest,
		},
		{
			name:       "unsupported status returns 400",
			body:       `{"provider_reference":"PAY-000001","status":"weird"}`,
			signature:  signBody(testWebhookSecret, `{"provider_reference":"PAY-000001","status":"weird"}`),
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidStatus,
		},
		{
			name:        "payment not found returns 404",
			body:        validBody,
			signature:   validSig,
			paymentsErr: payment.ErrPaymentNotFound,
			wantStatus:  http.StatusNotFound,
			wantCode:    CodePaymentNotFound,
		},
		{
			name:        "invalid transition returns 409",
			body:        validBody,
			signature:   validSig,
			paymentsErr: payment.ErrInvalidStatusTransition,
			wantStatus:  http.StatusConflict,
			wantCode:    CodeInvalidStatusTransition,
		},
		{
			name:        "repository failure returns 500",
			body:        validBody,
			signature:   validSig,
			paymentsErr: repoBoom,
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpresp.CodeInternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakePayments{err: tc.paymentsErr}
			r := newTestRouter(fp)
			w := doWebhook(t, r, tc.body, tc.signature)

			if tc.wantCode == "" {
				if w.Code != tc.wantStatus {
					t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
				}
				env := parseEnv(t, w.Body.Bytes())
				if s, _ := env["success"].(bool); !s {
					t.Errorf("success = false, want true; body=%s", w.Body.String())
				}
				return
			}
			assertErrorCode(t, w, tc.wantStatus, tc.wantCode)
		})
	}
}

// Duplicate callback: send the same paid callback twice; both return 200.
// Idempotency is enforced by payment.Service; here the fake accepts both
// calls without error, verifying the handler's success path is stable.
func TestCallbackHandler_DuplicateCallback(t *testing.T) {
	body := `{"provider_reference":"PAY-000001","status":"paid"}`
	sig := signBody(testWebhookSecret, body)

	fp := &fakePayments{}
	r := newTestRouter(fp)

	for i := range 2 {
		w := doWebhook(t, r, body, sig)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200; body=%s", i, w.Code, w.Body.String())
		}
	}
}
