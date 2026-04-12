package email

import (
	"strings"
	"testing"
)

func TestRenderNotification(t *testing.T) {
	data := NotificationData{
		Email:     "user@company.com",
		ID:        "ntf_abc-123",
		RequestID: "req_def-456",
	}

	html, text, err := RenderNotification(data)
	if err != nil {
		t.Fatalf("RenderNotification() error: %v", err)
	}

	// HTML body should contain the dynamic values.
	if !strings.Contains(html, "user@company.com") {
		t.Error("HTML does not contain email address")
	}
	if !strings.Contains(html, "ntf_abc-123") {
		t.Error("HTML does not contain notification ID")
	}
	if !strings.Contains(html, "req_def-456") {
		t.Error("HTML does not contain request ID")
	}

	// Plaintext body should contain the dynamic values.
	if !strings.Contains(text, "user@company.com") {
		t.Error("plaintext does not contain email address")
	}
	if !strings.Contains(text, "ntf_abc-123") {
		t.Error("plaintext does not contain notification ID")
	}
	if !strings.Contains(text, "req_def-456") {
		t.Error("plaintext does not contain request ID")
	}
}

func TestRenderNotificationHTMLNotEmpty(t *testing.T) {
	data := NotificationData{
		Email:     "a@b.com",
		ID:        "ntf_x",
		RequestID: "req_y",
	}
	html, _, err := RenderNotification(data)
	if err != nil {
		t.Fatalf("RenderNotification() error: %v", err)
	}
	if len(html) < 50 {
		t.Errorf("HTML body suspiciously short: %d bytes", len(html))
	}
}
