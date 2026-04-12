package domain

import (
	"context"
	"io"
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

// Store combines all storage interfaces for use at the composition
// root. Consumers (handlers, services) accept individual interfaces,
// not Store.
type Store interface {
	NotificationStore
	TransitionLogger
	HealthChecker
	io.Closer
}
