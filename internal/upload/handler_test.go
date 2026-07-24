package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
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

type testEnv struct {
	orders   *fakeOrderService
	repo     *fakeRepo
	storage  *fakeStorage
	tokenSvc *auth.HMACTokenService
	router   *gin.Engine
}

func newTestEnv(t *testing.T, maxSize int64) *testEnv {
	t.Helper()
	orders := newFakeOrderService()
	repo := newFakeRepo()
	storage := newFakeStorage()
	svc := NewService(repo, storage, orders, maxSize)
	tokenSvc := auth.NewHMACTokenService(testSecret, time.Hour)

	r := gin.New()
	api := r.Group("/api/v1")
	NewHandler(svc).RegisterRoutes(api, middleware.RequireAuth(tokenSvc, discardLogger()))
	return &testEnv{orders: orders, repo: repo, storage: storage, tokenSvc: tokenSvc, router: r}
}

func (e *testEnv) tokenFor(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := e.tokenSvc.Generate(auth.User{ID: userID, Email: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// buildMultipart returns a multipart body containing order_id + a file part
// (name=file) whose contents are payload. If omitFile is true, the file
// part is skipped entirely.
func buildMultipart(t *testing.T, orderID string, filename string, payload []byte, omitFile bool) (body *bytes.Buffer, contentType string) {
	t.Helper()
	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if orderID != "" {
		if err := w.WriteField("order_id", orderID); err != nil {
			t.Fatal(err)
		}
	}
	if !omitFile {
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
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

// --- Upload ---

func TestUploadHandler(t *testing.T) {
	userA := uuid.New()

	t.Run("upload success returns 201", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		ord := env.orders.seedOrder(userA)

		body, ct := buildMultipart(t, ord.ID.String(), "anything.pdf", pdfMagic, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))

		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		envBody := parseEnv(t, w.Body.Bytes())
		data, _ := envBody["data"].(map[string]any)
		up, _ := data["upload"].(map[string]any)
		if id, _ := up["id"].(string); id == "" {
			t.Error("upload.id missing")
		}
		if ctype, _ := up["content_type"].(string); ctype != "application/pdf" {
			t.Errorf("content_type = %q, want application/pdf", ctype)
		}
		// server-side generated filename, not the client's "anything.pdf"
		fname, _ := up["filename"].(string)
		if fname == "anything.pdf" {
			t.Error("filename echoed client value; server should regenerate")
		}
		if !strings.HasSuffix(fname, ".pdf") {
			t.Errorf("filename = %q, want .pdf suffix", fname)
		}
	})

	t.Run("invalid multipart (wrong Content-Type)", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", strings.NewReader("plain text body"))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing file field", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		ord := env.orders.seedOrder(userA)

		body, ct := buildMultipart(t, ord.ID.String(), "", nil, true)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusBadRequest, CodeMissingFile)
	})

	t.Run("unauthorized (no token)", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		ord := env.orders.seedOrder(userA)
		body, ct := buildMultipart(t, ord.ID.String(), "x.pdf", pdfMagic, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusUnauthorized, httpresp.CodeUnauthorized)
	})

	t.Run("unsupported content type", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		ord := env.orders.seedOrder(userA)
		body, ct := buildMultipart(t, ord.ID.String(), "x.txt", textBytes, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusUnsupportedMediaType, CodeUnsupportedType)
	})

	t.Run("oversized file", func(t *testing.T) {
		env := newTestEnv(t, 32) // tiny limit for fast test
		ord := env.orders.seedOrder(userA)
		// A PDF whose bytes exceed the configured limit.
		payload := append([]byte{}, pdfMagic...)
		payload = append(payload, bytes.Repeat([]byte("A"), 128)...)

		body, ct := buildMultipart(t, ord.ID.String(), "x.pdf", payload, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusRequestEntityTooLarge, CodeFileTooLarge)
	})

	t.Run("internal error (repository failure)", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		ord := env.orders.seedOrder(userA)
		env.repo.createErr = errors.New("db down")

		body, ct := buildMultipart(t, ord.ID.String(), "x.pdf", pdfMagic, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusInternalServerError, httpresp.CodeInternalError)
	})

	t.Run("invalid order_id (bad UUID)", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		body, ct := buildMultipart(t, "not-a-uuid", "x.pdf", pdfMagic, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusBadRequest, httpresp.CodeInvalidRequest)
	})
}

// --- Get ---

func TestGetHandler(t *testing.T) {
	userA := uuid.New()

	t.Run("owner fetches upload", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		u := &Upload{OrderID: uuid.New(), Filename: "x.pdf", ContentType: "application/pdf", Size: 10, Path: "test/x.pdf"}
		if err := env.repo.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+u.ID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+uuid.New().String(), nil)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusNotFound, CodeUploadNotFound)
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+uuid.New().String(), nil)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusUnauthorized, httpresp.CodeUnauthorized)
	})

	t.Run("invalid uuid param", func(t *testing.T) {
		env := newTestEnv(t, 1024)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/not-a-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+env.tokenFor(t, userA))
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		assertErrorCode(t, w, http.StatusBadRequest, httpresp.CodeInvalidRequest)
	})
}
