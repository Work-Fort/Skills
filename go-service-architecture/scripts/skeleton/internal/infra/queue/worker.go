package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	infraemail "github.com/workfort/notifier/internal/infra/email"

	"github.com/workfort/notifier/internal/domain"
)

// EmailWorker processes email delivery jobs from the goqite queue.
type EmailWorker struct {
	store  domain.NotificationStore
	sender domain.EmailSender
}

// NewEmailWorker creates a new worker with the given store and sender.
func NewEmailWorker(store domain.NotificationStore, sender domain.EmailSender) *EmailWorker {
	return &EmailWorker{store: store, sender: sender}
}

// Handle processes a single email delivery job. It is registered with
// the jobs.Runner via runner.Register("send_notification", worker.Handle).
//
// Flow:
//  1. Deserialise payload
//  2. Load notification from store
//  3. Check retry limit (REQ-013a) -- if exceeded, mark failed and ack
//  4. Update status to sending
//  5. Render email template
//  6. Send via SMTP (includes 6s delay per REQ-016)
//  7. On success: update to delivered, return nil
//  8. On failure: update to not_sent, increment retry_count, return error
//     (goqite retries via visibility timeout per REQ-018)
func (w *EmailWorker) Handle(ctx context.Context, payload []byte) error {
	var job EmailJobPayload
	if err := json.Unmarshal(payload, &job); err != nil {
		slog.Error("unmarshal job payload", "error", err)
		return nil // bad payload, ack to avoid infinite retry
	}

	slog.Info("processing email job",
		"notification_id", job.NotificationID,
		"email", job.Email,
	)

	// Load notification from store.
	n, err := w.store.GetNotificationByEmail(ctx, job.Email)
	if err != nil {
		slog.Error("get notification for job", "error", err, "email", job.Email)
		return fmt.Errorf("get notification: %w", err)
	}

	// REQ-013a: check retry limit before attempting send.
	if n.RetryCount >= n.RetryLimit {
		slog.Info("retry limit reached, marking as failed",
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
			"retry_limit", n.RetryLimit,
		)
		n.Status = domain.StatusFailed
		if err := w.store.UpdateNotification(ctx, n); err != nil {
			return fmt.Errorf("update notification to failed: %w", err)
		}
		return nil // ack message
	}

	// Update to sending.
	n.Status = domain.StatusSending
	if err := w.store.UpdateNotification(ctx, n); err != nil {
		return fmt.Errorf("update notification to sending: %w", err)
	}

	// Render email templates.
	html, text, err := infraemail.RenderNotification(infraemail.NotificationData{
		Email:     job.Email,
		ID:        job.NotificationID,
		RequestID: job.RequestID,
	})
	if err != nil {
		slog.Error("render email template", "error", err)
		return fmt.Errorf("render template: %w", err)
	}

	// Send via SMTP.
	msg := &domain.EmailMessage{
		To:        []string{job.Email},
		Subject:   "You have a new notification",
		HTML:      html,
		Text:      text,
		RequestID: job.RequestID,
	}

	if err := w.sender.Send(ctx, msg); err != nil {
		slog.Warn("email send failed, will retry",
			"error", err,
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
		)

		// Check if this is a permanent failure (@example.com).
		if errors.Is(err, infraemail.ErrExampleDomain) {
			n.Status = domain.StatusFailed
			if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
				slog.Error("update notification to failed", "error", updateErr)
			}
			return nil // ack -- permanent failure, no retry
		}

		// Transient failure: mark as not_sent, increment retry.
		n.Status = domain.StatusNotSent
		n.RetryCount++
		if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
			slog.Error("update notification to not_sent", "error", updateErr)
		}
		return fmt.Errorf("send email: %w", err) // return error for goqite retry
	}

	// Success: mark as delivered.
	n.Status = domain.StatusDelivered
	if err := w.store.UpdateNotification(ctx, n); err != nil {
		slog.Error("update notification to delivered", "error", err)
		return fmt.Errorf("update notification to delivered: %w", err)
	}

	slog.Info("email delivered",
		"notification_id", n.ID,
		"email", job.Email,
	)
	return nil
}
