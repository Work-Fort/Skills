package domain

import (
	"errors"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid simple", "user@example.com", false},
		{"valid with name", "user@company.co.uk", false},
		{"empty string", "", true},
		{"no at sign", "not-an-email", true},
		{"no domain", "user@", true},
		{"no local part", "@example.com", true},
		{"spaces only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("expected ErrInvalidEmail, got: %v", err)
			}
		})
	}
}
