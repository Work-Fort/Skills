//go:build qa

package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

func init() {
	// Override the 6-second delay for fast tests.
	sendDelay = 1 * time.Millisecond
}

func TestConsoleSenderImplementsInterface(t *testing.T) {
	// Compile-time check that ConsoleSender implements domain.EmailSender.
	var _ domain.EmailSender = (*ConsoleSender)(nil)
}

func TestConsoleSenderSuccess(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"user@company.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestConsoleSenderExampleComPermanentFailure(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(context.Background(), msg)
	if sendErr == nil {
		t.Fatal("expected error for @example.com, got nil")
	}
	if !errors.Is(sendErr, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain, got: %v", sendErr)
	}
}

func TestConsoleSenderFailComTimeout(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@fail.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(context.Background(), msg)
	if sendErr == nil {
		t.Fatal("expected error for @fail.com, got nil")
	}
	// Must NOT be ErrExampleDomain so the worker treats it as transient.
	if errors.Is(sendErr, ErrExampleDomain) {
		t.Error("@fail.com error should not be ErrExampleDomain")
	}
	// Must wrap context.DeadlineExceeded per service-build REQ-029.
	if !errors.Is(sendErr, context.DeadlineExceeded) {
		t.Error("@fail.com error should wrap context.DeadlineExceeded")
	}
}

func TestConsoleSenderSlowComDelay(t *testing.T) {
	// Override the @slow.com delay for testing.
	orig := simulatedDomains["@slow.com"]
	simulatedDomains["@slow.com"] = domainAction{
		delay: 1 * time.Millisecond,
		label: orig.label,
	}
	defer func() { simulatedDomains["@slow.com"] = orig }()

	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@slow.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	if sendErr := sender.Send(context.Background(), msg); sendErr != nil {
		t.Fatalf("Send() error = %v, want nil (slow but success)", sendErr)
	}
}

func TestConsoleSenderSlowComCancellation(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	msg := &domain.EmailMessage{
		To:      []string{"test@slow.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(ctx, msg)
	if sendErr == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
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
