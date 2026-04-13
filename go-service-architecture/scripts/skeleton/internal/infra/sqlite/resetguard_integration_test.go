package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// TestResetGuardRejectRetryInProgress verifies that the reset guard
// returns ErrRetriesRemaining when a notification is in not_sent state
// with retry_count < retry_limit, using a real SQLite database.
// Satisfies notification-management REQ-029.
func TestResetGuardRejectRetryInProgress(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create notification in not_sent with retries remaining.
	n := &domain.Notification{
		ID:         "ntf_guard-reject-1",
		Email:      "guard-reject@test.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Read it back from the database to confirm state.
	got, err := store.GetNotificationByEmail(ctx, "guard-reject@test.com")
	if err != nil {
		t.Fatal(err)
	}

	// Invoke the guard.
	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err == nil {
		t.Fatal("expected ErrRetriesRemaining, got nil")
	}
	if !errors.Is(err, domain.ErrRetriesRemaining) {
		t.Errorf("error = %v, want ErrRetriesRemaining", err)
	}
}

// TestResetGuardAllowRetriesExhausted verifies that the reset guard
// allows reset when retry_count >= retry_limit, using a real SQLite
// database. Satisfies notification-management REQ-029.
func TestResetGuardAllowRetriesExhausted(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create notification in not_sent with retries exhausted.
	n := &domain.Notification{
		ID:         "ntf_guard-allow-1",
		Email:      "guard-allow@test.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-allow@test.com")
	if err != nil {
		t.Fatal(err)
	}

	// Invoke the guard.
	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed), got %v", err)
	}
}

// TestResetGuardAllowFailedState verifies that failed notifications
// can always be reset regardless of retry_count.
func TestResetGuardAllowFailedState(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	n := &domain.Notification{
		ID:         "ntf_guard-failed-1",
		Email:      "guard-failed@test.com",
		Status:     domain.StatusFailed,
		RetryCount: 1,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}

	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed for failed), got %v", err)
	}
}

// TestResetGuardAllowDeliveredState verifies that delivered
// notifications can always be reset regardless of retry_count.
func TestResetGuardAllowDeliveredState(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	n := &domain.Notification{
		ID:         "ntf_guard-delivered-1",
		Email:      "guard-delivered@test.com",
		Status:     domain.StatusDelivered,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}

	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed for delivered), got %v", err)
	}
}
