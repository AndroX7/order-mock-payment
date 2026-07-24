package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

type TokenIssuer interface {
	Generate(user User) (string, error)
}

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
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

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

func normalizeName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptyName
	}
	return trimmed, nil
}

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
