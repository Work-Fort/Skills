package domain

import "context"

// EmailMessage holds the data needed to send one email.
type EmailMessage struct {
	To      []string
	Subject string
	HTML    string
	Text    string
}

// EmailSender sends email messages. Implementations live in infra/.
type EmailSender interface {
	Send(ctx context.Context, msg *EmailMessage) error
}
