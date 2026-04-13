package domain

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyNotified is returned when a notification has already
	// been sent to the given email address.
	ErrAlreadyNotified = errors.New("already notified")

	// ErrInvalidEmail is returned when the email address fails format
	// validation.
	ErrInvalidEmail = errors.New("invalid email address")

	// ErrRetriesRemaining is returned when a reset is attempted on a
	// notification that still has automatic retries in progress.
	ErrRetriesRemaining = errors.New("notification has retries remaining")
)
