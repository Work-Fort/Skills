package sqlite

import (
	"context"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// TestRetryLifecycleIntegration exercises the full retry lifecycle
// (pending -> sending -> not_sent -> sending -> delivered) using real
// SQLite storage. Satisfies notification-state-machine REQ-026.
func TestRetryLifecycleIntegration(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_retry-lifecycle-1",
		Email:      "retry-lifecycle@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Transition 1: pending -> sending.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Transition 2: sending -> not_sent (soft fail, retries remain).
	if err := sm.FireCtx(ctx, domain.TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}
	// Increment retry_count in DB (mimics worker behaviour).
	n.RetryCount = 1
	n.Status = domain.StatusNotSent
	if err := store.UpdateNotification(ctx, n); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	// Transition 3: not_sent -> sending (retry).
	// Need new SM with updated retry_count.
	sm = domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerRetry); err != nil {
		t.Fatalf("Fire(TriggerRetry): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Transition 4: sending -> delivered.
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Verify final state in database.
	got, err := store.GetNotificationByEmail(ctx, "retry-lifecycle@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log has exactly 4 entries.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("audit log entries = %d, want 4", count)
	}
}
