package domain

import (
	"fmt"
	"net/mail"
)

// ValidateEmail checks that the email address is well-formed using
// net/mail.ParseAddress from the Go standard library (REQ-006).
// Returns ErrInvalidEmail if the address is invalid.
func ValidateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%q: %w", email, ErrInvalidEmail)
	}
	return nil
}
