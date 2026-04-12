//go:build !qa

package email

import (
	"testing"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

func init() {
	// Override the 6-second delay for fast tests.
	sendDelay = 1 * time.Millisecond
}

func TestSMTPSenderImplementsInterface(t *testing.T) {
	// Compile-time check that SMTPSender implements domain.EmailSender.
	var _ domain.EmailSender = (*SMTPSender)(nil)
}

func TestRequestIDExtraction(t *testing.T) {
	msg := &domain.EmailMessage{
		To:        []string{"user@company.com"},
		Subject:   "Test",
		HTML:      "<p>test</p>",
		Text:      "test",
		RequestID: "req_abc-123",
	}
	got := RequestIDFromMessage(msg)
	if got != "req_abc-123" {
		t.Errorf("RequestIDFromMessage() = %q, want %q", got, "req_abc-123")
	}
}
