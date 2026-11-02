package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// dummyHash is compared against when a user does not exist so that login
// timing does not reveal which emails are registered.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcryptCost)

func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(h), err
}

// CheckPassword reports whether plain matches hash. A nil hash still performs
// a full bcrypt comparison to keep timing uniform.
func CheckPassword(hash *string, plain string) bool {
	if hash == nil || *hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*hash), []byte(plain)) == nil
}

var ErrWeakPassword = errors.New("password must be 8-72 characters and contain upper-case, lower-case, digit and special characters")

// ValidatePasswordStrength enforces a basic password policy.
func ValidatePasswordStrength(p string) error {
	if len(p) < 8 || len(p) > 72 { // bcrypt only uses the first 72 bytes
		return ErrWeakPassword
	}
	var upper, lower, digit, special bool
	for _, r := range p {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r):
			special = true
		}
	}
	if !upper || !lower || !digit || !special {
		return ErrWeakPassword
	}
	return nil
}
