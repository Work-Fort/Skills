package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// spyEmailSender captures sent messages.
type spyEmailSender struct {
	messages []*domain.EmailMessage
	err      error
}

func (s *spyEmailSender) Send(_ context.Context, msg *domain.EmailMessage) error {
	s.messages = append(s.messages, msg)
	return s.err
}

// spyStore captures notification updates.
type spyStore struct {
	notifications map[string]*domain.Notification
}

func newSpyStore() *spyStore {
	return &spyStore{notifications: make(map[string]*domain.Notification)}
}

func (s *spyStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.ID] = n
	return nil
}

func (s *spyStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	for _, n := range s.notifications {
		if n.Email == email {
			return n, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *spyStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.ID] = n
	return nil
}

func (s *spyStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, nil
}

func TestEmailWorkerSuccess(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_123"] = &domain.Notification{
		ID:         "ntf_123",
		Email:      "user@company.com",
		Status:     domain.StatusPending,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_123",
		Email:          "user@company.com",
		RequestID:      "req_abc",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// Verify email was sent.
	if len(sender.messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.messages))
	}
	if sender.messages[0].RequestID != "req_abc" {
		t.Errorf("RequestID = %q, want %q", sender.messages[0].RequestID, "req_abc")
	}

	// Verify notification status updated to delivered.
	n := store.notifications["ntf_123"]
	if n.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusDelivered)
	}
}

func TestEmailWorkerSendFailure(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_456"] = &domain.Notification{
		ID:         "ntf_456",
		Email:      "user@company.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{err: errors.New("smtp timeout")}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_456",
		Email:          "user@company.com",
		RequestID:      "req_def",
	})

	err := worker.Handle(context.Background(), payload)
	// Should return an error so goqite retries via visibility timeout.
	if err == nil {
		t.Fatal("expected error on send failure, got nil")
	}

	// Verify notification transitioned to not_sent with incremented retry.
	n := store.notifications["ntf_456"]
	if n.Status != domain.StatusNotSent {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusNotSent)
	}
	if n.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", n.RetryCount)
	}
}

func TestEmailWorkerRetryLimitExceeded(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_789"] = &domain.Notification{
		ID:         "ntf_789",
		Email:      "user@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{err: errors.New("still failing")}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_789",
		Email:          "user@company.com",
		RequestID:      "req_ghi",
	})

	// REQ-013a: when retry_count >= retry_limit, transition to failed
	// and return nil to acknowledge the message.
	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected nil (ack) when retry limit exceeded, got: %v", err)
	}

	n := store.notifications["ntf_789"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}
}
