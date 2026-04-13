package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// mockSender is a controllable email sender for integration tests.
type mockSender struct {
	err error
}

func (m *mockSender) Send(_ context.Context, _ *domain.EmailMessage) error {
	return m.err
}

// TestWorkerIntegrationSuccessPath exercises the worker's Handle
// method with a real SQLite store (not spy/mock). Success path:
// pending -> sending -> delivered. Satisfies notification-state-machine
// REQ-028.
func TestWorkerIntegrationSuccessPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_worker-int-success",
		Email:      "worker-success@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{err: nil} // success
	worker := queue.NewEmailWorker(store, sender, nil)

	payload, _ := json.Marshal(queue.EmailJobPayload{
		NotificationID: n.ID,
		Email:          n.Email,
		RequestID:      "req_int-success",
	})

	if err := worker.Handle(ctx, payload); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// Verify final state in real database.
	got, err := store.GetNotificationByEmail(ctx, n.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log entries: pending->sending, sending->delivered.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}

// TestWorkerIntegrationSoftFailPath exercises the worker's Handle
// method with a real SQLite store. Soft-fail path: pending -> sending
// -> not_sent. Satisfies notification-state-machine REQ-028.
func TestWorkerIntegrationSoftFailPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_worker-int-softfail",
		Email:      "worker-softfail@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{err: errors.New("smtp timeout")} // transient fail
	worker := queue.NewEmailWorker(store, sender, nil)

	payload, _ := json.Marshal(queue.EmailJobPayload{
		NotificationID: n.ID,
		Email:          n.Email,
		RequestID:      "req_int-softfail",
	})

	// Should return error (goqite retries via visibility timeout).
	err = worker.Handle(ctx, payload)
	if err == nil {
		t.Fatal("expected error on soft fail, got nil")
	}

	// Verify final state in real database.
	got, err := store.GetNotificationByEmail(ctx, n.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusNotSent {
		t.Errorf("status = %v, want not_sent", got.Status)
	}
	if got.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", got.RetryCount)
	}

	// Verify audit log entries: pending->sending, sending->not_sent.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}
