package email

import (
	"errors"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// sendDelay is the artificial delay before sending an email, making
// state transitions visible in the dashboard. Extracted as a package
// variable so tests can override it. Shared across both the SMTP
// sender (production/dev) and the ConsoleSender (QA).
var sendDelay = 6 * time.Second

// ErrExampleDomain is returned when the recipient address ends in
// @example.com, simulating an undeliverable address. Only the QA
// ConsoleSender returns this error; it is never returned in
// production/dev builds. It lives in the shared file because the
// worker (which is not build-tag-gated) checks errors.Is to
// distinguish permanent from transient failures in all builds.
var ErrExampleDomain = errors.New("example.com: permanent delivery failure (simulated)")

// RequestIDFromMessage extracts the request ID from the email message.
func RequestIDFromMessage(msg *domain.EmailMessage) string {
	return msg.RequestID
}
