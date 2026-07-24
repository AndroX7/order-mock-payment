package payment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testEnv wires a real router with real middleware + JWT + fake dependencies.
type testEnv struct {
	orders   *fakeOrderService
	repo     *fakePaymentRepo
	gateway  *stubGateway
	tokenSvc *auth.HMACTokenService
	router   *gin.Engine
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	orders := newFakeOrderService()
	repo := newFakePaymentRepo()
	gw := &stubGateway{}
	svc := NewService(repo, orders, gw)
	tokenSvc := auth.NewHMACTokenService(testSecret, time.Hour)

	r := gin.New()
	api := r.Group("/api/v1")
	NewHandler(svc).RegisterRoutes(api, middleware.RequireAuth(tokenSvc, discardLogger()))
	return &testEnv{orders: orders, repo: repo, gateway: gw, tokenSvc: tokenSvc, router: r}
}

func (e *testEnv) tokenFor(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := e.tokenSvc.Generate(auth.User{ID: userID, Email: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// seed an order + optional payment for the user.
func (e *testEnv) seedOrder(userID uuid.UUID, status string) *order.Order {
	return e.orders.seedOrder(userID, status, "1", "1")
}

func (e *testEnv) seedPayment(t *testing.T, userID uuid.UUID) *Payment {
	t.Helper()
	o := e.seedOrder(userID, order.StatusPendingPayment)
	p := &Payment{OrderID: o.ID, Provider: "stub", ProviderReference: "PAY-SEED",
		Amount: decimal.RequireFromString("1"), Currency: "USD", Status: StatusPending}
	if err := e.repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func doJSON(t *testing.T, e *testEnv, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
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

// --- Create ---

func TestCreatePaymentHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	t.Run("success returns 201", func(t *testing.T) {
		env := newTestEnv(t)
		ord := env.seedOrder(userA, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		envBody := parseEnv(t, w.Body.Bytes())
		data, _ := envBody["data"].(map[string]any)
		p, _ := data["payment"].(map[string]any)
		if id, _ := p["id"].(string); id == "" {
			t.Error("payment.id missing")
		}
		if oid, _ := p["order_id"].(string); oid != ord.ID.String() {
			t.Errorf("order_id = %q, want %q", oid, ord.ID.String())
		}
		if ref, _ := p["provider_reference"].(string); ref == "" {
			t.Error("provider_reference missing")
		}
		// order status must be transitioned.
		if env.orders.orders[ord.ID].Status != order.StatusPendingPayment {
			t.Errorf("order status = %q, want %q",
				env.orders.orders[ord.ID].Status, order.StatusPendingPayment)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		env := newTestEnv(t)
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", `{`, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, httpresp.CodeInvalidRequest)
	})

	t.Run("missing order_id", func(t *testing.T) {
		env := newTestEnv(t)
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", `{}`, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, httpresp.CodeInvalidRequest)
	})

	t.Run("missing token", func(t *testing.T) {
		env := newTestEnv(t)
		ord := env.seedOrder(userA, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, "")
		assertErrorCode(t, w, http.StatusUnauthorized, httpresp.CodeUnauthorized)
	})

	t.Run("forbidden ownership returns 404", func(t *testing.T) {
		env := newTestEnv(t)
		// order belongs to userB; userA attempts to pay for it.
		ord := env.seedOrder(userB, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})

	t.Run("duplicate payment returns 409", func(t *testing.T) {
		env := newTestEnv(t)
		ord := env.seedOrder(userA, order.StatusPending)
		env.repo.byOrder[ord.ID] = true // pretend a payment already exists
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusConflict, CodeDuplicatePayment)
	})

	t.Run("order not payable returns 409", func(t *testing.T) {
		env := newTestEnv(t)
		ord := env.seedOrder(userA, order.StatusPendingPayment) // already advanced
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusConflict, CodeOrderNotPayable)
	})

	t.Run("gateway failure returns 500", func(t *testing.T) {
		env := newTestEnv(t)
		env.gateway.err = errors.New("gateway down")
		ord := env.seedOrder(userA, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusInternalServerError, httpresp.CodeInternalError)
	})

	t.Run("repository failure returns 500", func(t *testing.T) {
		env := newTestEnv(t)
		env.repo.createErr = errors.New("db down")
		ord := env.seedOrder(userA, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		w := doJSON(t, env, http.MethodPost, "/api/v1/payments", body, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusInternalServerError, httpresp.CodeInternalError)
	})

	t.Run("wrong content-type", func(t *testing.T) {
		env := newTestEnv(t)
		ord := env.seedOrder(userA, order.StatusPending)
		body := `{"order_id":"` + ord.ID.String() + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", strings.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		assertErrorCode(t, w, http.StatusUnsupportedMediaType, httpresp.CodeInvalidContentType)
	})
}

// --- Get ---

func TestGetPaymentHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	t.Run("owner fetches payment", func(t *testing.T) {
		env := newTestEnv(t)
		p := env.seedPayment(t, userA)
		w := doJSON(t, env, http.MethodGet, "/api/v1/payments/"+p.ID.String(), "", env.tokenFor(t, userA))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-owner sees 404", func(t *testing.T) {
		env := newTestEnv(t)
		p := env.seedPayment(t, userB)
		w := doJSON(t, env, http.MethodGet, "/api/v1/payments/"+p.ID.String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodePaymentNotFound)
	})

	t.Run("missing token", func(t *testing.T) {
		env := newTestEnv(t)
		p := env.seedPayment(t, userA)
		w := doJSON(t, env, http.MethodGet, "/api/v1/payments/"+p.ID.String(), "", "")
		assertErrorCode(t, w, http.StatusUnauthorized, httpresp.CodeUnauthorized)
	})

	t.Run("unknown payment id", func(t *testing.T) {
		env := newTestEnv(t)
		w := doJSON(t, env, http.MethodGet, "/api/v1/payments/"+uuid.New().String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodePaymentNotFound)
	})

	t.Run("invalid uuid param", func(t *testing.T) {
		env := newTestEnv(t)
		w := doJSON(t, env, http.MethodGet, "/api/v1/payments/not-a-uuid", "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, httpresp.CodeInvalidRequest)
	})
}
