package middleware

import (
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
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newProtectedRouter mounts /protected behind RequireAuth. On success the
// handler echoes claims so a test can verify they were stored.
func newProtectedRouter(parser TokenParser) *gin.Engine {
	r := gin.New()
	r.GET("/protected",
		RequireAuth(parser, discardLogger()),
		func(c *gin.Context) {
			claims, ok := GetClaims(c)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "no claims"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"user_id": claims.UserID.String(),
				"email":   claims.Email,
			})
		})
	return r
}

func TestRequireAuth(t *testing.T) {
	tokenSvc := auth.NewHMACTokenService(testSecret, time.Hour)
	user := auth.User{ID: uuid.New(), Email: "alice@example.com"}
	validToken, err := tokenSvc.Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	expiredSvc := auth.NewHMACTokenService(testSecret, -time.Hour)
	expiredToken, err := expiredSvc.Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	wrongSecretSvc := auth.NewHMACTokenService("different-secret-at-least-32-bytes!!", time.Hour)
	wrongSigToken, err := wrongSecretSvc.Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		header     string // empty means "do not send Authorization header"
		wantStatus int
	}{
		{"valid Bearer token", "Bearer " + validToken, http.StatusOK},
		{"lowercase bearer scheme accepted", "bearer " + validToken, http.StatusOK},
		{"missing header", "", http.StatusUnauthorized},
		{"missing scheme prefix", validToken, http.StatusUnauthorized},
		{"wrong scheme (Basic)", "Basic abc", http.StatusUnauthorized},
		{"expired token", "Bearer " + expiredToken, http.StatusUnauthorized},
		{"wrong signature", "Bearer " + wrongSigToken, http.StatusUnauthorized},
		{"malformed token", "Bearer not.a.valid.jwt", http.StatusUnauthorized},
		{"empty bearer value", "Bearer   ", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newProtectedRouter(tokenSvc)
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			// Unauthorized responses must carry the standard envelope.
			if tc.wantStatus == http.StatusUnauthorized {
				body := w.Body.String()
				if !containsAll(body, `"success":false`, `"code":"UNAUTHORIZED"`) {
					t.Errorf("unauthorized body missing envelope fields: %s", body)
				}
			}
		})
	}
}

func TestGetClaims_MissingReturnsFalse(t *testing.T) {
	c := &gin.Context{}
	_, ok := GetClaims(c)
	if ok {
		t.Error("GetClaims returned ok on empty context; want false")
	}
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(s, n) {
			return false
		}
	}
	return true
}
