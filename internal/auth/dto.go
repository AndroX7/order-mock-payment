package auth

import (
	"time"

	"github.com/google/uuid"
)

// SignupRequest is the JSON payload for POST /api/v1/auth/signup.
// No `binding:` tags: business validation lives in the service so the
// same rules apply to any future non-HTTP caller.
type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginRequest is the JSON payload for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse is the client-facing view of a User. PasswordHash is
// intentionally absent — the DTO cannot leak it.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewUserResponse builds the safe DTO from a domain User.
func NewUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
