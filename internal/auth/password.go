package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is the default bcrypt cost factor.
	DefaultBcryptCost = 12

	// MinPasswordLength is the minimum password length.
	MinPasswordLength = 8
)

var (
	// ErrPasswordTooShort is returned when password is too short.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")

	// ErrPasswordNoUppercase is returned when password has no uppercase letter.
	ErrPasswordNoUppercase = errors.New("password must contain at least one uppercase letter")

	// ErrPasswordNoLowercase is returned when password has no lowercase letter.
	ErrPasswordNoLowercase = errors.New("password must contain at least one lowercase letter")

	// ErrPasswordNoDigit is returned when password has no digit.
	ErrPasswordNoDigit = errors.New("password must contain at least one digit")

	// ErrInvalidCredentials is returned when login credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// HashPassword hashes a password using bcrypt.
func HashPassword(password string, cost int) (string, error) {
	if cost <= 0 {
		cost = DefaultBcryptCost
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword verifies a password against a bcrypt hash.
func VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// ValidatePasswordStrength validates password meets minimum requirements.
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	var hasUpper, hasLower, hasDigit bool

	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUppercase
	}
	if !hasLower {
		return ErrPasswordNoLowercase
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}

	return nil
}
