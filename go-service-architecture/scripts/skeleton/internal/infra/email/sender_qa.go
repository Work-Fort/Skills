//go:build qa

package email

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// domainAction describes a simulated behavior for a recipient domain.
type domainAction struct {
	delay time.Duration
	err   error
	label string
}

// simulatedDomains maps recipient domains to simulated behaviors
// (service-build REQ-029). This map is compiled only into QA builds
// (REQ-030).
var simulatedDomains = map[string]domainAction{
	"@example.com": {
		err:   ErrExampleDomain,
		label: "simulated permanent failure for @example.com",
	},
	"@fail.com": {
		err:   fmt.Errorf("simulated timeout: %w", context.DeadlineExceeded),
		label: "simulated timeout for @fail.com",
	},
	"@slow.com": {
		delay: 30 * time.Second,
		label: "simulated slow delivery for @slow.com",
	},
}

// matchDomain returns the domainAction for the first recipient whose
// address matches a simulated domain, or nil if no match.
func matchDomain(recipients []string) *domainAction {
	for _, addr := range recipients {
		lower := strings.ToLower(addr)
		for suffix, action := range simulatedDomains {
			if strings.HasSuffix(lower, suffix) {
				a := action // copy to avoid loop variable capture
				return &a
			}
		}
	}
	return nil
}

// ConsoleSender implements domain.EmailSender by logging to slog
// instead of sending over SMTP. It requires no external dependencies
// (service-build REQ-032).
type ConsoleSender struct{}

// NewSMTPSender returns a ConsoleSender in QA builds. The function
// name matches the production signature so the daemon compiles
// without build-tag-conditional wiring. The SMTP parameters are
// accepted but ignored (service-build REQ-032).
func NewSMTPSender(_ string, _ int, _ string) (*ConsoleSender, error) {
	slog.Info("QA build: using ConsoleSender (no SMTP)")
	return &ConsoleSender{}, nil
}

// Send logs the email to slog and applies simulated domain behaviors.
//
// Flow:
//  1. Check recipients against the simulated domain map (REQ-029).
//     If matched, log and apply the simulated action (REQ-031).
//  2. Apply the standard 6-second delay (REQ-028).
//  3. Log the email content (REQ-027) and return nil.
func (s *ConsoleSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
	// REQ-029: check simulated failure domain map.
	if action := matchDomain(msg.To); action != nil {
		// REQ-031: log the simulated action.
		slog.Info(action.label, "to", msg.To)

		// Domain-specific delay (e.g., @slow.com 30s) replaces the
		// standard 6s delay -- the total delay is the domain delay,
		// not domain delay + 6s.
		if action.delay > 0 {
			slog.Info("email send delay starting", "delay", action.delay)
			select {
			case <-time.After(action.delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if action.err != nil {
			return fmt.Errorf("send to %s: %w", msg.To[0], action.err)
		}

		// Domain matched with delay but no error (e.g., @slow.com):
		// log and return success after the delay.
		slog.Info("email sent (console)",
			"to", msg.To,
			"subject", msg.Subject,
			"body", msg.Text,
		)
		return nil
	}

	// REQ-028: standard 6-second artificial delay.
	slog.Info("email send delay starting", "delay", sendDelay)
	select {
	case <-time.After(sendDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	// REQ-027: log the email content.
	slog.Info("email sent (console)",
		"to", msg.To,
		"subject", msg.Subject,
		"body", msg.Text,
	)
	return nil
}
