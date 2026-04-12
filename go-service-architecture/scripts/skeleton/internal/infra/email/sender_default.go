//go:build !qa

package email

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/workfort/notifier/internal/domain"
)

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
// delay (notification-delivery REQ-016) and propagates the
// X-Request-ID header (REQ-023).
//
// The @example.com rejection logic is NOT present in production/dev
// builds (notification-delivery REQ-017). That behavior is compiled
// only into QA builds via the ConsoleSender's domain map.
func (s *SMTPSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
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
