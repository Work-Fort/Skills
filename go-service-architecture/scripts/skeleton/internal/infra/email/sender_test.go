package email

import (
	"context"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestSMTPSenderImplementsInterface(t *testing.T) {
	// Compile-time check that SMTPSender implements domain.EmailSender.
	var _ domain.EmailSender = (*SMTPSender)(nil)
}

func TestExampleComAutoFail(t *testing.T) {
	sender := &SMTPSender{} // no SMTP client needed for this check
	msg := &domain.EmailMessage{
		To:      []string{"test@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for @example.com, got nil")
	}
	if !errors.Is(err, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain, got: %v", err)
	}
}

func TestExampleComCheckMultipleRecipients(t *testing.T) {
	sender := &SMTPSender{}
	msg := &domain.EmailMessage{
		To:      []string{"real@company.com", "fail@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	err := sender.Send(context.Background(), msg)
	if !errors.Is(err, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain when any recipient is @example.com, got: %v", err)
	}
}
