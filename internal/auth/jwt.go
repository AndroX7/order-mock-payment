package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims are the validated fields extracted from a JWT.
type Claims struct {
	UserID uuid.UUID
	Email  string
}

// HMACTokenService issues and validates HS256 JWTs.
type HMACTokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewHMACTokenService(secret string, ttl time.Duration) *HMACTokenService {
	return &HMACTokenService{secret: []byte(secret), ttl: ttl}
}

// jwtClaims is the on-the-wire representation. Only sub, email, iat, exp
// per the design — no name, no role, no jti.
type jwtClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Generate issues a signed HS256 token for u.
func (t *HMACTokenService) Generate(u User) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		Email: u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	})
	signed, err := tok.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse validates raw and returns the extracted Claims. Any failure —
// bad signature, expired, malformed, wrong signing method — collapses
// to ErrInvalidToken so callers cannot distinguish failure modes.
func (t *HMACTokenService) Parse(raw string) (Claims, error) {
	var jc jwtClaims
	tok, err := jwt.ParseWithClaims(raw, &jc, func(tok *jwt.Token) (any, error) {
		// Alg-confusion defense: only HMAC methods are accepted.
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return t.secret, nil
	})
	if err != nil || tok == nil || !tok.Valid {
		return Claims{}, ErrInvalidToken
	}
	uid, err := uuid.Parse(jc.Subject)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: uid, Email: jc.Email}, nil
}
