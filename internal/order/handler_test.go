package order

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

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testEnv holds everything a handler test needs: repo (for seeding),
// token service (for issuing valid tokens), and a fully wired router.
type testEnv struct {
	repo     *fakeRepo
	tokenSvc *auth.HMACTokenService
	router   *gin.Engine
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(repo)
	tokenSvc := auth.NewHMACTokenService(testSecret, time.Hour)

	r := gin.New()
	api := r.Group("/api/v1")
	NewHandler(svc).RegisterRoutes(api, middleware.RequireAuth(tokenSvc, discardLogger()))
	return &testEnv{repo: repo, tokenSvc: tokenSvc, router: r}
}

func (e *testEnv) tokenFor(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := e.tokenSvc.Generate(auth.User{ID: userID, Email: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (e *testEnv) seed(t *testing.T, userID uuid.UUID) *Order {
	t.Helper()
	got, err := NewService(e.repo).Create(context.Background(), userID, validCreateReq())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return got
}

func doJSON(t *testing.T, e *testEnv, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
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
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("error missing; body=%s", w.Body.String())
	}
	if code, _ := errObj["code"].(string); code != wantCode {
		t.Errorf("code = %q, want %q", code, wantCode)
	}
}

// --- Create ---

func TestCreateHandler(t *testing.T) {
	userA := uuid.New()

	const validBody = `{"symbol":"BTCUSD","side":"BUY","quantity":"1.5","price":"30000"}`

	cases := []struct {
		name        string
		body        string
		contentType string
		bearer      func(*testEnv) string
		wantStatus  int
		wantCode    string // empty when success
	}{
		{name: "success", body: validBody, contentType: "application/json",
			bearer: func(e *testEnv) string { return e.tokenFor(t, userA) }, wantStatus: http.StatusCreated},
		{name: "missing token", body: validBody, contentType: "application/json",
			bearer: func(*testEnv) string { return "" }, wantStatus: http.StatusUnauthorized, wantCode: middleware.CodeUnauthorized},
		{name: "invalid symbol (lowercase)",
			body:        `{"symbol":"btcusd","side":"BUY","quantity":"1","price":"1"}`,
			contentType: "application/json",
			bearer:      func(e *testEnv) string { return e.tokenFor(t, userA) },
			wantStatus:  http.StatusBadRequest, wantCode: CodeInvalidSymbol},
		{name: "invalid side",
			body:        `{"symbol":"BTCUSD","side":"HOLD","quantity":"1","price":"1"}`,
			contentType: "application/json",
			bearer:      func(e *testEnv) string { return e.tokenFor(t, userA) },
			wantStatus:  http.StatusBadRequest, wantCode: CodeInvalidSide},
		{name: "malformed JSON",
			body:        `{"symbol":"BTCUSD","side":"BUY","quan`,
			contentType: "application/json",
			bearer:      func(e *testEnv) string { return e.tokenFor(t, userA) },
			wantStatus:  http.StatusBadRequest, wantCode: CodeInvalidRequest},
		{name: "wrong content type",
			body:        validBody,
			contentType: "text/plain",
			bearer:      func(e *testEnv) string { return e.tokenFor(t, userA) },
			wantStatus:  http.StatusUnsupportedMediaType, wantCode: CodeInvalidContentType},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			var r *http.Request
			r = httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			if bearer := tc.bearer(env); bearer != "" {
				r.Header.Set("Authorization", "Bearer "+bearer)
			}
			w := httptest.NewRecorder()
			env.router.ServeHTTP(w, r)

			if tc.wantCode == "" {
				if w.Code != tc.wantStatus {
					t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
				}
				env := parseEnv(t, w.Body.Bytes())
				data, _ := env["data"].(map[string]any)
				order, _ := data["order"].(map[string]any)
				if id, _ := order["id"].(string); id == "" {
					t.Errorf("order.id missing")
				}
				if uid, _ := order["user_id"].(string); uid != userA.String() {
					t.Errorf("order.user_id = %q, want %q", uid, userA.String())
				}
				return
			}
			assertErrorCode(t, w, tc.wantStatus, tc.wantCode)
		})
	}
}

// --- Get ---

func TestGetHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	env := newTestEnv(t)
	own := env.seed(t, userA)
	other := env.seed(t, userB)

	t.Run("owner sees order", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders/"+own.ID.String(), "", env.tokenFor(t, userA))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-owner sees 404", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders/"+other.ID.String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})

	t.Run("missing token", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders/"+own.ID.String(), "", "")
		assertErrorCode(t, w, http.StatusUnauthorized, middleware.CodeUnauthorized)
	})

	t.Run("invalid uuid param", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders/not-a-uuid", "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, CodeInvalidRequest)
	})

	t.Run("unknown id", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders/"+uuid.New().String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})
}

// --- List ---

func TestListHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	env := newTestEnv(t)
	env.seed(t, userA)
	env.seed(t, userA)
	env.seed(t, userB) // owned by someone else — must NOT appear

	t.Run("returns only own orders", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders", "", env.tokenFor(t, userA))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		envBody := parseEnv(t, w.Body.Bytes())
		data, _ := envBody["data"].(map[string]any)
		orders, _ := data["orders"].([]any)
		if len(orders) != 2 {
			t.Errorf("len = %d, want 2", len(orders))
		}
		for _, o := range orders {
			m := o.(map[string]any)
			if uid, _ := m["user_id"].(string); uid != userA.String() {
				t.Errorf("leaked order for user %q, want %q", uid, userA.String())
			}
		}
	})

	t.Run("empty for user with no orders", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders", "", env.tokenFor(t, uuid.New()))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		envBody := parseEnv(t, w.Body.Bytes())
		data, _ := envBody["data"].(map[string]any)
		orders, _ := data["orders"].([]any)
		if len(orders) != 0 {
			t.Errorf("len = %d, want 0", len(orders))
		}
	})

	t.Run("missing token", func(t *testing.T) {
		w := doJSON(t, env, http.MethodGet, "/api/v1/orders", "", "")
		assertErrorCode(t, w, http.StatusUnauthorized, middleware.CodeUnauthorized)
	})
}

// --- Update ---

func TestUpdateHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	const validBody = `{"symbol":"ETHUSD","side":"SELL","quantity":"2","price":"2000"}`

	t.Run("owner update succeeds", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)

		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+own.ID.String(), validBody, env.tokenFor(t, userA))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-owner sees 404", func(t *testing.T) {
		env := newTestEnv(t)
		other := env.seed(t, userB)

		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+other.ID.String(), validBody, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})

	t.Run("invalid input", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)

		bad := `{"symbol":"ETHUSD","side":"SELL","quantity":"0","price":"2000"}`
		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+own.ID.String(), bad, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, CodeInvalidQuantity)
	})

	t.Run("missing token", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)
		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+own.ID.String(), validBody, "")
		assertErrorCode(t, w, http.StatusUnauthorized, middleware.CodeUnauthorized)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)
		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+own.ID.String(), `{`, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusBadRequest, CodeInvalidRequest)
	})

	t.Run("internal error", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)
		env.repo.failErr = errors.New("db down")
		w := doJSON(t, env, http.MethodPut, "/api/v1/orders/"+own.ID.String(), validBody, env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusInternalServerError, CodeInternalError)
	})
}

// --- Delete ---

func TestDeleteHandler(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	t.Run("owner delete returns 204", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)
		w := doJSON(t, env, http.MethodDelete, "/api/v1/orders/"+own.ID.String(), "", env.tokenFor(t, userA))
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-owner sees 404", func(t *testing.T) {
		env := newTestEnv(t)
		other := env.seed(t, userB)
		w := doJSON(t, env, http.MethodDelete, "/api/v1/orders/"+other.ID.String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})

	t.Run("missing token", func(t *testing.T) {
		env := newTestEnv(t)
		own := env.seed(t, userA)
		w := doJSON(t, env, http.MethodDelete, "/api/v1/orders/"+own.ID.String(), "", "")
		assertErrorCode(t, w, http.StatusUnauthorized, middleware.CodeUnauthorized)
	})

	t.Run("unknown id", func(t *testing.T) {
		env := newTestEnv(t)
		w := doJSON(t, env, http.MethodDelete, "/api/v1/orders/"+uuid.New().String(), "", env.tokenFor(t, userA))
		assertErrorCode(t, w, http.StatusNotFound, CodeOrderNotFound)
	})
}
