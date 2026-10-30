package domain

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	// maxPasswordLength is bcrypt's limit: it ignores bytes after the 72nd.
	maxPasswordLength = 72
)

// ValidatePassword checks the length of a plain password (bytes, because that
// is what bcrypt limits).
func ValidatePassword(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidUser, minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalidUser, maxPasswordLength)
	}
	return nil
}

// HashPassword returns the bcrypt hash that is stored in users.password_hash.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// CheckPassword reports whether password matches hash. An empty hash (a user
// without a password) never matches.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash is compared against when the email is unknown, so a wrong email
// takes as long as a wrong password.
var dummyHash, _ = HashPassword("not-a-real-password")

// BurnPasswordCheck spends the time of one password check.
func BurnPasswordCheck(password string) {
	CheckPassword(dummyHash, password)
}
