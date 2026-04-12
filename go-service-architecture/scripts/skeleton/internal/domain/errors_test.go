package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		message  string
	}{
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrAlreadyNotified", ErrAlreadyNotified, "already notified"},
		{"ErrInvalidEmail", ErrInvalidEmail, "invalid email address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sentinel.Error() != tt.message {
				t.Errorf("Error() = %q, want %q", tt.sentinel.Error(), tt.message)
			}
		})
	}
}

func TestErrorSentinelsUnwrap(t *testing.T) {
	wrapped := fmt.Errorf("get notification abc: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped error should match ErrNotFound via errors.Is")
	}

	wrapped2 := fmt.Errorf("create notification: %w", ErrAlreadyNotified)
	if !errors.Is(wrapped2, ErrAlreadyNotified) {
		t.Error("wrapped error should match ErrAlreadyNotified via errors.Is")
	}
}
