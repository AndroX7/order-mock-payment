package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func TestHMACTokenService_RoundTrip(t *testing.T) {
	svc := NewHMACTokenService(testSecret, time.Hour)
	user := User{ID: uuid.New(), Email: "alice@example.com"}

	token, err := svc.Generate(user)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", claims.UserID, user.ID)
	}
	if claims.Email != user.Email {
		t.Errorf("Email = %q, want %q", claims.Email, user.Email)
	}
}

func TestHMACTokenService_Parse_Failures(t *testing.T) {
	svc := NewHMACTokenService(testSecret, time.Hour)
	user := User{ID: uuid.New(), Email: "alice@example.com"}
	valid, err := svc.Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	// Expired token: issue with negative TTL.
	expired, err := NewHMACTokenService(testSecret, -time.Hour).Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	// Signed with a different secret.
	wrongSig, err := NewHMACTokenService("different-secret-at-least-32-bytes!!", time.Hour).Generate(user)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"expired", expired},
		{"wrong signature", wrongSig},
		{"malformed structure", "not.a.jwt"},
		{"empty string", ""},
		{"random bytes", "xxx.yyy.zzz"},
		// Tamper with a valid token by flipping the last byte.
		{"tampered payload", valid[:len(valid)-1] + "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Parse(tc.token)
			if err == nil {
				t.Fatalf("Parse(%q) got nil err, want ErrInvalidToken", tc.name)
			}
			if !errors.Is(err, ErrInvalidToken) {
				t.Errorf("err = %v, want errors.Is(_, ErrInvalidToken)", err)
			}
		})
	}
}
