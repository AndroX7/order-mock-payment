package auth

import "golang.org/x/crypto/bcrypt"

// PasswordHasher hashes plaintext passwords and verifies them against a
// stored hash. The underlying algorithm and any tuning parameters are
// implementation details.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(password, hash string) error
}

// BcryptHasher is the production PasswordHasher, backed by bcrypt.
type BcryptHasher struct{}

const bcryptCost = 12

// Hash returns a bcrypt hash of password.
func (BcryptHasher) Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Compare reports whether password matches the stored bcrypt hash.
func (BcryptHasher) Compare(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

var _ PasswordHasher = BcryptHasher{}
