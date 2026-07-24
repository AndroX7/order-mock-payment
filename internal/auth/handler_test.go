package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestRouter(svc *Service) *gin.Engine {
	r := gin.New()
	NewHandler(svc).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestSignupHandler(t *testing.T) {
	const validBody = `{"email":"alice@example.com","password":"correct-horse","name":"Alice"}`
	longPassword := strings.Repeat("a", 73)

	cases := []struct {
		name        string
		body        string
		contentType string
		setupRepo   func(*fakeRepo)
		wantStatus  int
		wantSuccess bool
		wantCode    string
	}{
		{
			name:        "success",
			body:        validBody,
			contentType: "application/json",
			wantStatus:  http.StatusCreated,
			wantSuccess: true,
		},
		{
			name:        "invalid email",
			body:        `{"email":"not-an-email","password":"correct-horse","name":"Alice"}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodeInvalidEmail,
		},
		{
			name:        "empty name",
			body:        `{"email":"alice@example.com","password":"correct-horse","name":"   "}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodeInvalidName,
		},
		{
			name:        "short password",
			body:        `{"email":"alice@example.com","password":"short","name":"Alice"}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodePasswordTooShort,
		},
		{
			name:        "long password",
			body:        `{"email":"alice@example.com","password":"` + longPassword + `","name":"Alice"}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodePasswordTooLong,
		},
		{
			name:        "duplicate email",
			body:        validBody,
			contentType: "application/json",
			setupRepo: func(r *fakeRepo) {
				r.users["alice@example.com"] = &User{Email: "alice@example.com"}
			},
			wantStatus: http.StatusConflict,
			wantCode:   CodeEmailAlreadyExists,
		},
		{
			name:        "malformed JSON",
			body:        `{"email":"alice@example.com","passwo`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodeInvalidRequest,
		},
		{
			name:        "unsupported Content-Type",
			body:        validBody,
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    CodeInvalidContentType,
		},
		{
			name:        "internal service error",
			body:        validBody,
			contentType: "application/json",
			setupRepo:   func(r *fakeRepo) { r.createErr = errors.New("db down") },
			wantStatus:  http.StatusInternalServerError,
			wantCode:    CodeInternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tc.setupRepo != nil {
				tc.setupRepo(repo)
			}
			svc := NewService(repo, stubHasher{}, stubIssuer{})
			r := newTestRouter(svc)

			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/auth/signup",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			assertNoPasswordLeak(t, w.Body.Bytes())

			env := parseEnvelope(t, w.Body.Bytes())

			if tc.wantSuccess {
				assertSuccessEnvelope(t, env)
				return
			}
			assertErrorEnvelope(t, env, tc.wantCode)
		})
	}
}

func parseEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, body)
	}
	return out
}

func assertSuccessEnvelope(t *testing.T, env map[string]any) {
	t.Helper()
	if s, _ := env["success"].(bool); !s {
		t.Errorf("success = false, want true; env=%v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong type; env=%v", env)
	}
	user, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("data.user missing or wrong type; data=%v", data)
	}
	if got, _ := user["email"].(string); got != "alice@example.com" {
		t.Errorf("user.email = %q, want %q", got, "alice@example.com")
	}
	if got, _ := user["name"].(string); got != "Alice" {
		t.Errorf("user.name = %q, want %q", got, "Alice")
	}
	if id, _ := user["id"].(string); id == "" {
		t.Errorf("user.id missing/empty; user=%v", user)
	}
	// UserResponse fields only — no password/hash keys.
	if _, present := user["password"]; present {
		t.Errorf("user.password present in response")
	}
	if _, present := user["password_hash"]; present {
		t.Errorf("user.password_hash present in response")
	}
}

func assertErrorEnvelope(t *testing.T, env map[string]any, wantCode string) {
	t.Helper()
	if s, _ := env["success"].(bool); s {
		t.Errorf("success = true, want false; env=%v", env)
	}
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("error missing or wrong type; env=%v", env)
	}
	if got, _ := errObj["code"].(string); got != wantCode {
		t.Errorf("error.code = %q, want %q", got, wantCode)
	}
	if msg, _ := errObj["message"].(string); msg == "" {
		t.Error("error.message empty")
	}
}

// assertNoPasswordLeak scans the raw response body for anything that looks
// like a password or password hash. Belt-and-suspenders on top of the
// UserResponse field check.
func assertNoPasswordLeak(t *testing.T, body []byte) {
	t.Helper()
	lower := strings.ToLower(string(body))
	for _, needle := range []string{"password_hash", "passwordhash", "correct-horse"} {
		if strings.Contains(lower, needle) {
			t.Errorf("response body contains %q; body=%s", needle, body)
		}
	}
}

// --- Login handler tests ---

func TestLoginHandler(t *testing.T) {
	const validBody = `{"email":"alice@example.com","password":"correct-horse"}`

	// seedUser installs a user under the target email with a stub-compatible hash.
	seedUser := func(r *fakeRepo) {
		r.users["alice@example.com"] = &User{
			ID:           uuid.New(),
			Email:        "alice@example.com",
			PasswordHash: "HASH(correct-horse)",
			Name:         "Alice",
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}
	}

	cases := []struct {
		name        string
		body        string
		contentType string
		setupRepo   func(*fakeRepo)
		hasher      stubHasher
		issuer      stubIssuer
		wantStatus  int
		wantCode    string
		wantToken   bool
	}{
		{
			name:        "success",
			body:        validBody,
			contentType: "application/json",
			setupRepo:   seedUser,
			wantStatus:  http.StatusOK,
			wantToken:   true,
		},
		{
			name:        "unknown email",
			body:        validBody,
			contentType: "application/json",
			wantStatus:  http.StatusUnauthorized,
			wantCode:    CodeInvalidCredentials,
		},
		{
			name:        "wrong password",
			body:        validBody,
			contentType: "application/json",
			setupRepo:   seedUser,
			hasher:      stubHasher{compareErr: errors.New("mismatch")},
			wantStatus:  http.StatusUnauthorized,
			wantCode:    CodeInvalidCredentials,
		},
		{
			name:        "invalid email format",
			body:        `{"email":"not-an-email","password":"correct-horse"}`,
			contentType: "application/json",
			wantStatus:  http.StatusUnauthorized,
			wantCode:    CodeInvalidCredentials,
		},
		{
			name:        "empty password",
			body:        `{"email":"alice@example.com","password":""}`,
			contentType: "application/json",
			wantStatus:  http.StatusUnauthorized,
			wantCode:    CodeInvalidCredentials,
		},
		{
			name:        "malformed JSON",
			body:        `{"email":"alice@example.com","passw`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodeInvalidRequest,
		},
		{
			name:        "unsupported Content-Type",
			body:        validBody,
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    CodeInvalidContentType,
		},
		{
			name:        "repository error",
			body:        validBody,
			contentType: "application/json",
			setupRepo:   func(r *fakeRepo) { r.getErr = errors.New("db down") },
			wantStatus:  http.StatusInternalServerError,
			wantCode:    CodeInternalError,
		},
		{
			name:        "token issuer error",
			body:        validBody,
			contentType: "application/json",
			setupRepo:   seedUser,
			issuer:      stubIssuer{err: errors.New("sign failure")},
			wantStatus:  http.StatusInternalServerError,
			wantCode:    CodeInternalError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tc.setupRepo != nil {
				tc.setupRepo(repo)
			}
			svc := NewService(repo, tc.hasher, tc.issuer)
			r := newTestRouter(svc)

			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/auth/login",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			assertNoPasswordLeak(t, w.Body.Bytes())

			env := parseEnvelope(t, w.Body.Bytes())
			if tc.wantToken {
				assertLoginSuccessEnvelope(t, env)
				return
			}
			assertErrorEnvelope(t, env, tc.wantCode)
		})
	}
}

func assertLoginSuccessEnvelope(t *testing.T, env map[string]any) {
	t.Helper()
	if s, _ := env["success"].(bool); !s {
		t.Errorf("success = false, want true; env=%v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing; env=%v", env)
	}
	token, _ := data["token"].(string)
	if token == "" {
		t.Errorf("data.token empty; data=%v", data)
	}
	user, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("data.user missing; data=%v", data)
	}
	if got, _ := user["email"].(string); got != "alice@example.com" {
		t.Errorf("user.email = %q, want %q", got, "alice@example.com")
	}
	if _, present := user["password_hash"]; present {
		t.Error("user.password_hash present in response")
	}
}
