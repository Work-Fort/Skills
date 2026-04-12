package domain

import (
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	tests := []struct {
		prefix string
	}{
		{"ntf"},
		{"req"},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			id := NewID(tt.prefix)
			if !strings.HasPrefix(id, tt.prefix+"_") {
				t.Errorf("NewID(%q) = %q, want prefix %q_", tt.prefix, id, tt.prefix)
			}
			// UUID v4 is 36 chars: prefix + _ + 36 = len(prefix) + 37
			wantLen := len(tt.prefix) + 1 + 36
			if len(id) != wantLen {
				t.Errorf("NewID(%q) length = %d, want %d", tt.prefix, len(id), wantLen)
			}
		})
	}
}

func TestNewIDUniqueness(t *testing.T) {
	a := NewID("ntf")
	b := NewID("ntf")
	if a == b {
		t.Errorf("two calls to NewID returned the same value: %q", a)
	}
}
