package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

const queueName = "notifications"

// EmailJobPayload is the JSON structure enqueued by the notify handler
// and deserialised by the email worker. Defined in this package as the
// queue owns the job format; the httpapi package imports it.
type EmailJobPayload struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}

// Flavor selects the SQL dialect for goqite.
type Flavor int

const (
	FlavorSQLite   Flavor = iota // default
	FlavorPostgres
)

// NotificationQueue wraps a goqite queue and implements
// httpapi.Enqueuer so the handler can enqueue jobs without depending
// on goqite directly.
type NotificationQueue struct {
	q *goqite.Queue
}

// NewNotificationQueue creates a goqite queue named "notifications"
// sharing the given *sql.DB (service-database REQ-019). The flavor
// parameter selects the SQL dialect for goqite.
func NewNotificationQueue(db *sql.DB, flavor Flavor) (*NotificationQueue, error) {
	opts := goqite.NewOpts{
		DB:         db,
		Name:       queueName,
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	}
	if flavor == FlavorPostgres {
		opts.SQLFlavor = goqite.SQLFlavorPostgreSQL
	}
	q := goqite.New(opts)
	return &NotificationQueue{q: q}, nil
}

// Queue returns the underlying goqite.Queue for use with
// jobs.NewRunner.
func (nq *NotificationQueue) Queue() *goqite.Queue {
	return nq.q
}

// Enqueue serialises the payload and creates a job in the goqite queue
// using the jobs.Create envelope format. The runner dispatches jobs by
// name, so Create must be used instead of raw q.Send(). The returned
// goqite message ID is logged for operational observability.
func (nq *NotificationQueue) Enqueue(ctx context.Context, payload []byte) error {
	msgID, err := jobs.Create(ctx, nq.q, "send_notification", goqite.Message{Body: payload})
	if err != nil {
		return fmt.Errorf("enqueue notification: %w", err)
	}
	// Log the queue message ID to correlate queue messages with
	// notification requests during debugging and operational triage.
	var p EmailJobPayload
	if jsonErr := json.Unmarshal(payload, &p); jsonErr == nil {
		slog.Info("notification job enqueued",
			"queue_message_id", string(msgID),
			"notification_id", p.NotificationID,
		)
	}
	return nil
}

// NewJobRunner creates a jobs.Runner configured per REQ-013:
// Limit 5 (max concurrent jobs), PollInterval 500ms.
func NewJobRunner(q *goqite.Queue) *jobs.Runner {
	return jobs.NewRunner(jobs.NewRunnerOpts{
		Limit:        5,
		Log:          slog.Default(),
		PollInterval: 500 * time.Millisecond,
		Queue:        q,
	})
}
