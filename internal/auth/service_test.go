package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeRepo struct {
	createErr error
	getErr    error
	users     map[string]*User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[string]*User{}}
}

func (r *fakeRepo) Create(_ context.Context, u *User) error {
	if r.createErr != nil {
		return r.createErr
	}
	if _, exists := r.users[u.Email]; exists {
		return ErrEmailAlreadyExists
	}
	u.ID = uuid.New()
	u.CreatedAt = time.Now().UTC()
	u.UpdatedAt = u.CreatedAt
	r.users[u.Email] = u
	return nil
}

func (r *fakeRepo) GetByEmail(_ context.Context, email string) (*User, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if u, ok := r.users[email]; ok {
		return u, nil
	}
	return nil, ErrUserNotFound
}

type stubHasher struct {
	compareErr error
}

func (stubHasher) Hash(p string) (string, error) { return "HASH(" + p + ")", nil }

func (h stubHasher) Compare(_, _ string) error { return h.compareErr }

type stubIssuer struct {
	token string
	err   error
}

func (s stubIssuer) Generate(_ User) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.token == "" {
		return "test-token-abc", nil
	}
	return s.token, nil
}

var (
	_ Repository     = (*fakeRepo)(nil)
	_ PasswordHasher = stubHasher{}
	_ TokenIssuer    = stubIssuer{}
)

func TestSignup_ValidationCases(t *testing.T) {
	repoBoom := errors.New("db down")

	valid := SignupRequest{
		Email:    "alice@example.com",
		Password: "correct-horse",
		Name:     "Alice",
	}

	cases := []struct {
		name      string
		req       SignupRequest
		setupRepo func(*fakeRepo)
		wantErrIs error // nil == success
	}{
		{
			name: "success",
			req:  valid,
		},
		{
			name: "duplicate email",
			req:  valid,
			setupRepo: func(r *fakeRepo) {
				r.users[valid.Email] = &User{Email: valid.Email}
			},
			wantErrIs: ErrEmailAlreadyExists,
		},
		{
			name:      "invalid email (garbage)",
			req:       SignupRequest{Email: "not-an-email", Password: valid.Password, Name: valid.Name},
			wantErrIs: ErrInvalidEmail,
		},
		{
			name:      "invalid email (empty after trim)",
			req:       SignupRequest{Email: "   ", Password: valid.Password, Name: valid.Name},
			wantErrIs: ErrInvalidEmail,
		},
		{
			name:      "invalid email (display-name form rejected)",
			req:       SignupRequest{Email: "Alice <alice@example.com>", Password: valid.Password, Name: valid.Name},
			wantErrIs: ErrInvalidEmail,
		},
		{
			name:      "empty name after trim",
			req:       SignupRequest{Email: valid.Email, Password: valid.Password, Name: "   "},
			wantErrIs: ErrEmptyName,
		},
		{
			name:      "password below minimum",
			req:       SignupRequest{Email: valid.Email, Password: "short", Name: valid.Name},
			wantErrIs: ErrPasswordTooShort,
		},
		{
			name:      "password above bcrypt max",
			req:       SignupRequest{Email: valid.Email, Password: strings.Repeat("a", 73), Name: valid.Name},
			wantErrIs: ErrPasswordTooLong,
		},
		{
			name:      "repository failure",
			req:       valid,
			setupRepo: func(r *fakeRepo) { r.createErr = repoBoom },
			wantErrIs: repoBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tc.setupRepo != nil {
				tc.setupRepo(repo)
			}
			svc := NewService(repo, stubHasher{}, stubIssuer{})

			got, err := svc.Signup(context.Background(), tc.req)

			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("Signup err = %v, want nil", err)
				}
				if got == nil {
					t.Fatal("Signup returned nil user on success")
				}
				if got.ID == uuid.Nil {
					t.Error("User.ID not populated by repository")
				}
				if got.PasswordHash == "" {
					t.Error("User.PasswordHash empty — hashing did not run")
				}
				return
			}

			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("Signup err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
			}
			if got != nil {
				t.Errorf("Signup returned user %+v on error, want nil", got)
			}
		})
	}
}

func TestSignup_NormalizesEmail(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, stubHasher{}, stubIssuer{})

	got, err := svc.Signup(context.Background(), SignupRequest{
		Email:    "  Alice@Example.COM  ",
		Password: "correct-horse",
		Name:     "  Alice  ",
	})
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("returned Email = %q, want %q", got.Email, "alice@example.com")
	}
	if got.Name != "Alice" {
		t.Errorf("returned Name = %q, want %q", got.Name, "Alice")
	}
	stored, ok := repo.users["alice@example.com"]
	if !ok {
		t.Fatalf("user not stored under canonical email; repo has %v", keys(repo.users))
	}
	if stored.Email != "alice@example.com" {
		t.Errorf("stored Email = %q, want canonical form", stored.Email)
	}
}

func keys(m map[string]*User) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLogin_Cases(t *testing.T) {
	repoBoom := errors.New("db down")
	issuerBoom := errors.New("signing failed")

	seedUser := func(r *fakeRepo) *User {
		u := &User{
			ID:           uuid.New(),
			Email:        "alice@example.com",
			PasswordHash: "HASH(correct-horse)",
			Name:         "Alice",
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}
		r.users[u.Email] = u
		return u
	}

	validReq := LoginRequest{Email: "alice@example.com", Password: "correct-horse"}

	cases := []struct {
		name      string
		req       LoginRequest
		setupRepo func(*fakeRepo)
		hasher    stubHasher
		issuer    stubIssuer
		wantErrIs error // nil == success
		wantToken bool  // when success, expect non-empty token
	}{
		{
			name:      "success",
			req:       validReq,
			setupRepo: func(r *fakeRepo) { seedUser(r) },
			wantToken: true,
		},
		{
			name:      "unknown email",
			req:       validReq,
			wantErrIs: ErrInvalidCredentials,
		},
		{
			name:      "wrong password",
			req:       validReq,
			setupRepo: func(r *fakeRepo) { seedUser(r) },
			hasher:    stubHasher{compareErr: errors.New("mismatch")},
			wantErrIs: ErrInvalidCredentials,
		},
		{
			name:      "invalid email format",
			req:       LoginRequest{Email: "not-an-email", Password: "correct-horse"},
			wantErrIs: ErrInvalidCredentials,
		},
		{
			name:      "empty email",
			req:       LoginRequest{Email: "   ", Password: "correct-horse"},
			wantErrIs: ErrInvalidCredentials,
		},
		{
			name:      "empty password",
			req:       LoginRequest{Email: validReq.Email, Password: ""},
			wantErrIs: ErrInvalidCredentials,
		},
		{
			name:      "repository failure",
			req:       validReq,
			setupRepo: func(r *fakeRepo) { r.createErr = repoBoom; r.getErr = repoBoom },
			wantErrIs: repoBoom,
		},
		{
			name:      "token issuer error",
			req:       validReq,
			setupRepo: func(r *fakeRepo) { seedUser(r) },
			issuer:    stubIssuer{err: issuerBoom},
			wantErrIs: issuerBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tc.setupRepo != nil {
				tc.setupRepo(repo)
			}
			svc := NewService(repo, tc.hasher, tc.issuer)

			user, token, err := svc.Login(context.Background(), tc.req)

			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("Login err = %v, want nil", err)
				}
				if user == nil {
					t.Fatal("nil user on success")
				}
				if tc.wantToken && token == "" {
					t.Error("empty token on success")
				}
				return
			}
			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("Login err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
			}
			if user != nil || token != "" {
				t.Errorf("Login returned (user=%v, token=%q) on error", user, token)
			}
		})
	}
}
