package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// TokenIssuer is the minimal token-generation surface the Service needs.
// Consumer-owned interface; the concrete *HMACTokenService satisfies it.
type TokenIssuer interface {
	Generate(user User) (string, error)
}

// Service orchestrates user signup and login. Business rules live here so
// any future transport (HTTP, gRPC, CLI) inherits identical behavior.
type Service struct {
	repo        Repository
	hasher      PasswordHasher
	tokenIssuer TokenIssuer
}

func NewService(repo Repository, hasher PasswordHasher, issuer TokenIssuer) *Service {
	return &Service{repo: repo, hasher: hasher, tokenIssuer: issuer}
}

const (
	minPasswordLen = 8
	maxPasswordLen = 72
)

// Signup validates the request, hashes the password, and persists the user.
// On success the returned *User has ID, CreatedAt, UpdatedAt populated by the
// repository.
func (s *Service) Signup(ctx context.Context, req SignupRequest) (*User, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	name, err := normalizeName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &User{
		Email:        email,
		PasswordHash: hash,
		Name:         name,
	}
	// Rely on the users_email_uidx UNIQUE constraint for duplicate detection —
	// no SELECT-before-INSERT race window. The repository maps SQLSTATE 23505
	// to ErrEmailAlreadyExists.
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// normalizeEmail trims, lower-cases, and validates the email. Returns the
// canonical form (bare address, all lowercase). Rejects display-name forms
// like "Alice <alice@example.com>" that net/mail otherwise accepts.
func normalizeEmail(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidEmail
	}
	if addr.Address != trimmed {
		return "", ErrInvalidEmail
	}
	return addr.Address, nil
}

// normalizeName trims whitespace and rejects empty names.
func normalizeName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptyName
	}
	return trimmed, nil
}

// validatePassword checks byte length only. The password is treated as
// opaque data — never trimmed, never transformed.
func validatePassword(p string) error {
	switch {
	case len(p) < minPasswordLen:
		return ErrPasswordTooShort
	case len(p) > maxPasswordLen:
		return ErrPasswordTooLong
	default:
		return nil
	}
}

// Login validates credentials and issues a JWT. All authentication
// failures — bad email format, unknown email, wrong password — collapse
// to ErrInvalidCredentials so callers cannot distinguish "no such user"
// from "wrong password".
func (s *Service) Login(ctx context.Context, req LoginRequest) (*User, string, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}
	if req.Password == "" {
		return nil, "", ErrInvalidCredentials
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	if err := s.hasher.Compare(req.Password, user.PasswordHash); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.tokenIssuer.Generate(*user)
	if err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	return user, token, nil
}
