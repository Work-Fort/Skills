package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/workfort/notifier/internal/domain"
)

// ErrExampleDomain is returned when the recipient address ends in
// @example.com, simulating an undeliverable address.
var ErrExampleDomain = errors.New("example.com: permanent delivery failure (simulated)")

// sendDelay is the artificial delay before sending an email, making
// state transitions visible in the dashboard. Extracted as a package
// variable so tests can override it.
var sendDelay = 6 * time.Second

// SMTPSender implements domain.EmailSender via go-mail SMTP.
type SMTPSender struct {
	client *gomail.Client
	from   string
}

// NewSMTPSender creates a new SMTP sender. For Mailpit (local dev),
// use port 1025 with no auth and TLSOpportunistic.
func NewSMTPSender(host string, port int, from string) (*SMTPSender, error) {
	c, err := gomail.NewClient(host,
		gomail.WithPort(port),
		gomail.WithTLSPolicy(gomail.TLSOpportunistic),
	)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return &SMTPSender{client: c, from: from}, nil
}

// Send delivers an email message via SMTP. It enforces the 6-second
// delay (REQ-016) and rejects @example.com recipients (REQ-017).
// The X-Request-ID header from the context is added to the email
// (REQ-023).
func (s *SMTPSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
	// REQ-017: reject @example.com.
	for _, addr := range msg.To {
		if strings.HasSuffix(strings.ToLower(addr), "@example.com") {
			return fmt.Errorf("send to %s: %w", addr, ErrExampleDomain)
		}
	}

	// REQ-016: artificial delay to simulate real delivery latency.
	slog.Info("email send delay starting", "delay", sendDelay)
	select {
	case <-time.After(sendDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	m := gomail.NewMsg()
	if err := m.From(s.from); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := m.To(msg.To...); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(gomail.TypeTextHTML, msg.HTML)
	m.AddAlternativeString(gomail.TypeTextPlain, msg.Text)

	// REQ-023: propagate request ID into email header.
	if reqID := RequestIDFromMessage(msg); reqID != "" {
		m.SetGenHeader(gomail.Header("X-Request-ID"), reqID)
	}

	if err := s.client.DialAndSend(m); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// RequestIDFromMessage extracts the request ID from the email message.
// For now this is a placeholder that returns empty string. Task 5
// adds the RequestID field to EmailMessage and this function reads it.
func RequestIDFromMessage(_ *domain.EmailMessage) string {
	return ""
}
