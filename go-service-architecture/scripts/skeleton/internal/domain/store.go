package domain

import (
	"context"
	"io"

	"github.com/qmuntal/stateless"
)

// NotificationStore persists and retrieves notification records.
type NotificationStore interface {
	CreateNotification(ctx context.Context, n *Notification) error
	GetNotificationByEmail(ctx context.Context, email string) (*Notification, error)
	UpdateNotification(ctx context.Context, n *Notification) error
	ListNotifications(ctx context.Context, after string, limit int) ([]*Notification, error)
}

// TransitionLogger records state transitions for audit purposes.
type TransitionLogger interface {
	LogTransition(ctx context.Context, entityType, entityID string, from, to Status, trigger Trigger) error
}

// HealthChecker verifies the backing store is reachable.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Broadcaster pushes real-time state change messages to connected
// clients. Implementations live in infra/ (e.g., WebSocket hub).
type Broadcaster interface {
	Broadcast(msg []byte)
}

// Enqueuer abstracts the job queue. Both httpapi and mcp handlers
// accept this interface rather than a concrete goqite type.
type Enqueuer interface {
	Enqueue(ctx context.Context, payload []byte) error
}

// ResetStore defines the storage interface needed by reset
// operations. It combines notification CRUD, state machine
// accessor/mutator, and transition logging.
type ResetStore interface {
	NotificationStore
	TransitionLogger
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// Store combines all storage interfaces for use at the composition
// root. Consumers (handlers, services) accept individual interfaces,
// not Store.
type Store interface {
	NotificationStore
	TransitionLogger
	HealthChecker
	io.Closer
}
