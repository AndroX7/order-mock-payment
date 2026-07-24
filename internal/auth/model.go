package auth

import (
	"time"

	"github.com/google/uuid"
)

// User is the domain type persisted in the users table.
// PasswordHash is present here but MUST never be serialized in DTOs.
type User struct {
	ID           uuid.UUID `db:"id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	Name         string    `db:"name"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
