package sqlite

import (
	"context"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestStateMachineIntegrationDeliveryPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-1",
		Email:      "sm-int@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Build the state machine with real accessor/mutator.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit,
		n.RetryCount,
	)

	// pending -> sending
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// sending -> delivered
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Verify final state in database.
	got, err := store.GetNotificationByEmail(ctx, "sm-int@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log has 2 entries.
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}

func TestStateMachineIntegrationRetryPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-2",
		Email:      "sm-retry@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// pending -> sending
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend)

	// sending -> not_sent (soft fail)
	if err := sm.FireCtx(ctx, domain.TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail)

	// not_sent -> sending (retry) -- need new SM with updated state.
	n.RetryCount = 1
	sm = domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerRetry); err != nil {
		t.Fatalf("Fire(TriggerRetry): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry)

	// sending -> delivered
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered)

	// Verify 4 audit log entries (pending->sending, sending->not_sent,
	// not_sent->sending, sending->delivered).
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("audit log entries = %d, want 4", count)
	}
}

func TestStateMachineIntegrationRejectedTransition(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-3",
		Email:      "sm-reject@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)

	// pending -> failed should be rejected (REQ-009).
	err = sm.FireCtx(ctx, domain.TriggerFailed)
	if err == nil {
		t.Fatal("expected error for pending -> failed, got nil")
	}

	// Verify state unchanged in database.
	got, err := store.GetNotificationByEmail(ctx, "sm-reject@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending (unchanged)", got.Status)
	}
}

func TestStateMachineIntegrationResetFromFailed(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-reset-failed",
		Email:      "reset-failed@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Drive to failed: pending -> sending -> failed.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerFailed); err != nil {
		t.Fatalf("Fire(TriggerFailed): %v", err)
	}

	// Simulate the handler pattern: get a fresh copy, fire reset,
	// then sync the local struct and update.
	got, err := store.GetNotificationByEmail(ctx, "reset-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed {
		t.Fatalf("pre-reset status = %v, want failed", got.Status)
	}

	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatalf("Fire(TriggerReset): %v", err)
	}

	// This is the bug fix line -- without it, got.Status is still "failed".
	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	// Verify final state in database via a fresh read.
	final, err := store.GetNotificationByEmail(ctx, "reset-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", final.Status)
	}
	if final.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", final.RetryCount)
	}
}

func TestStateMachineIntegrationResetFromDelivered(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-reset-delivered",
		Email:      "reset-delivered@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Drive to delivered: pending -> sending -> delivered.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}

	// Simulate the handler pattern with fresh copy.
	got, err := store.GetNotificationByEmail(ctx, "reset-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Fatalf("pre-reset status = %v, want delivered", got.Status)
	}

	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatalf("Fire(TriggerReset): %v", err)
	}

	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	final, err := store.GetNotificationByEmail(ctx, "reset-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", final.Status)
	}
	if final.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", final.RetryCount)
	}
}

func TestStateMachineIntegrationRedeliveryAfterReset(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-redeliver",
		Email:      "redeliver@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// First delivery: pending -> sending -> delivered.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatal(err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatal(err)
	}

	// Reset: delivered -> pending (simulating handler pattern).
	got, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatal(err)
	}
	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatal(err)
	}

	// Re-delivery: pending -> sending -> delivered (simulating worker).
	reread, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	workerSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(reread.ID),
		store.NotificationStateMutator(reread.ID),
		reread.RetryLimit, reread.RetryCount,
	)
	if err := workerSM.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("re-delivery TriggerSend: %v", err)
	}

	// Verify intermediate state is sending.
	mid, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if mid.Status != domain.StatusSending {
		t.Errorf("mid-delivery status = %v, want sending", mid.Status)
	}

	if err := workerSM.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("re-delivery TriggerDelivered: %v", err)
	}

	// Verify final state.
	final, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusDelivered {
		t.Errorf("final status = %v, want delivered", final.Status)
	}
}
