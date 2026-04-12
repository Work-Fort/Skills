package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/qmuntal/stateless"

	infraemail "github.com/workfort/notifier/internal/infra/email"

	"github.com/workfort/notifier/internal/domain"
)

// WorkerStore combines the notification storage and state machine
// accessor/mutator interfaces needed by the worker. The SQLite Store
// satisfies this interface.
type WorkerStore interface {
	domain.NotificationStore
	domain.TransitionLogger
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// EmailWorker processes email delivery jobs from the goqite queue.
// It uses the domain state machine for all state transitions and
// logs each transition to the audit log.
type EmailWorker struct {
	store  WorkerStore
	sender domain.EmailSender
}

// NewEmailWorker creates a new worker with the given store and email
// sender. The store provides notification CRUD, state machine
// accessor/mutator (database-backed), and transition logging.
func NewEmailWorker(store WorkerStore, sender domain.EmailSender) *EmailWorker {
	return &EmailWorker{store: store, sender: sender}
}

// fireAndLog fires a trigger on the state machine and logs the
// transition to the audit log. The accessor/mutator write to the
// database, so no separate UpdateNotification call is needed for
// the status change.
func (w *EmailWorker) fireAndLog(ctx context.Context, sm *stateless.StateMachine, n *domain.Notification, from domain.Status, trigger domain.Trigger, to domain.Status) error {
	if err := sm.FireCtx(ctx, trigger); err != nil {
		return fmt.Errorf("fire %v: %w", trigger, err)
	}
	if err := w.store.LogTransition(ctx, "notification", n.ID, from, to, trigger); err != nil {
		slog.Error("log transition failed", "error", err,
			"notification_id", n.ID, "from", from, "to", to)
		// Log failure is non-fatal; the state change already succeeded.
	}
	return nil
}

// Handle processes a single email delivery job. It is registered with
// the jobs.Runner via runner.Register("send_notification", worker.Handle).
//
// Flow:
//  1. Deserialise payload
//  2. Load notification from store
//  3. Create state machine with database-backed accessor/mutator
//  4. Fire send trigger (pending/not_sent -> sending)
//  5. Render email template
//  6. Send via SMTP (includes 6s delay)
//  7. On success: fire delivered trigger, return nil
//  8. On @example.com: fire failed trigger, return nil (permanent)
//  9. On transient failure: increment retry_count, fire soft_fail
//     trigger, return error (goqite retries via visibility timeout)
//  10. On retry limit exhausted: fire failed trigger, return nil
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

	// Create state machine with database-backed accessor/mutator
	// (REQ-015). The accessor reads the current status from the DB;
	// the mutator writes the new status. This keeps the state machine
	// and database synchronized on every transition.
	sm := domain.ConfigureStateMachine(
		w.store.NotificationStateAccessor(n.ID),
		w.store.NotificationStateMutator(n.ID),
		n.RetryLimit,
		n.RetryCount,
	)

	// Fire send trigger: pending -> sending or not_sent -> sending.
	// The retry-limit check happens implicitly: if the notification
	// is in not_sent with exhausted retries, the guard on
	// TriggerSoftFail would have already moved it to failed on the
	// previous attempt. If it arrives here in not_sent, retries are
	// still available.
	prevStatus := n.Status
	var sendTrigger domain.Trigger
	if n.Status == domain.StatusNotSent {
		sendTrigger = domain.TriggerRetry
	} else {
		sendTrigger = domain.TriggerSend
	}
	if err := w.fireAndLog(ctx, sm, n, prevStatus, sendTrigger, domain.StatusSending); err != nil {
		return err
	}
	// Re-read status from store after state machine transition.
	n.Status = domain.StatusSending

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
		slog.Warn("email send failed",
			"error", err,
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
		)

		// Check if this is a permanent failure (@example.com).
		if errors.Is(err, infraemail.ErrExampleDomain) {
			if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
				slog.Error("fire failed trigger", "error", fErr)
			}
			return nil // ack -- permanent failure, no retry
		}

		// Transient failure: fire soft_fail, increment retry.
		if smErr := sm.FireCtx(ctx, domain.TriggerSoftFail); smErr != nil {
			// Guard rejected: retry limit exhausted during this
			// attempt. Fire TriggerFailed from sending instead.
			slog.Info("soft_fail rejected by guard, firing failed",
				"notification_id", n.ID,
				"retry_count", n.RetryCount,
				"retry_limit", n.RetryLimit,
			)
			if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
				slog.Error("fire failed trigger after guard rejection", "error", fErr)
			}
			return nil // ack -- retries exhausted
		}
		// soft_fail succeeded (sending -> not_sent).
		if logErr := w.store.LogTransition(ctx, "notification", n.ID,
			domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail); logErr != nil {
			slog.Error("log transition failed", "error", logErr)
		}

		n.RetryCount++
		if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
			slog.Error("update notification retry_count", "error", updateErr)
		}
		return fmt.Errorf("send email: %w", err) // return error for goqite retry
	}

	// Success: fire delivered trigger.
	if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerDelivered, domain.StatusDelivered); fErr != nil {
		slog.Error("fire delivered trigger", "error", fErr)
		return fmt.Errorf("fire delivered: %w", fErr)
	}

	slog.Info("email delivered",
		"notification_id", n.ID,
		"email", job.Email,
	)
	return nil
}
