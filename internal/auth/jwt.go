package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID uuid.UUID
	Email  string
}

type HMACTokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewHMACTokenService(secret string, ttl time.Duration) *HMACTokenService {
	return &HMACTokenService{secret: []byte(secret), ttl: ttl}
}

type jwtClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

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

func (t *HMACTokenService) Parse(raw string) (Claims, error) {
	var jc jwtClaims
	tok, err := jwt.ParseWithClaims(raw, &jc, func(tok *jwt.Token) (any, error) {
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
