package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/qmuntal/stateless"

	infraemail "github.com/workfort/notifier/internal/infra/email"

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

// spyStore satisfies WorkerStore using in-memory maps. The accessor
// and mutator read/write the notification's Status field in the map,
// mimicking the database-backed versions in the real store.
type spyStore struct {
	notifications map[string]*domain.Notification
	transitions   []transitionRecord
}

type transitionRecord struct {
	entityType string
	entityID   string
	from       domain.Status
	to         domain.Status
	trigger    domain.Trigger
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

func (s *spyStore) LogTransition(_ context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	s.transitions = append(s.transitions, transitionRecord{
		entityType: entityType,
		entityID:   entityID,
		from:       from,
		to:         to,
		trigger:    trigger,
	})
	return nil
}

func (s *spyStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		n, ok := s.notifications[notificationID]
		if !ok {
			return nil, domain.ErrNotFound
		}
		return n.Status, nil
	}
}

func (s *spyStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, state stateless.State) error {
		n, ok := s.notifications[notificationID]
		if !ok {
			return domain.ErrNotFound
		}
		n.Status = state.(domain.Status)
		return nil
	}
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

	// Verify audit log: pending->sending, sending->delivered.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[0].from != domain.StatusPending || store.transitions[0].to != domain.StatusSending {
		t.Errorf("transition[0] = %v->%v, want pending->sending",
			store.transitions[0].from, store.transitions[0].to)
	}
	if store.transitions[1].from != domain.StatusSending || store.transitions[1].to != domain.StatusDelivered {
		t.Errorf("transition[1] = %v->%v, want sending->delivered",
			store.transitions[1].from, store.transitions[1].to)
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

	// Verify audit log: pending->sending, sending->not_sent.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
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

	// When retry_count >= retry_limit, the worker fires TriggerRetry
	// (not_sent -> sending), attempts send, send fails, then the
	// soft_fail guard rejects TriggerSoftFail, so it fires
	// TriggerFailed (sending -> failed) and acks.
	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected nil (ack) when retry limit exceeded, got: %v", err)
	}

	n := store.notifications["ntf_789"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}

	// Verify audit log: not_sent->sending, sending->failed.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[1].to != domain.StatusFailed {
		t.Errorf("transition[1].to = %v, want failed", store.transitions[1].to)
	}
}

func TestEmailWorkerExampleComAutoFail(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_ex1"] = &domain.Notification{
		ID:         "ntf_ex1",
		Email:      "test@example.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	// Sender returns ErrExampleDomain for @example.com addresses.
	sender := &spyEmailSender{err: fmt.Errorf("send to test@example.com: %w", infraemail.ErrExampleDomain)}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_ex1",
		Email:          "test@example.com",
		RequestID:      "req_ex1",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// @example.com path: pending -> sending -> failed (not not_sent).
	n := store.notifications["ntf_ex1"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}

	// Verify audit log: pending->sending, sending->failed.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[0].from != domain.StatusPending || store.transitions[0].to != domain.StatusSending {
		t.Errorf("transition[0] = %v->%v, want pending->sending",
			store.transitions[0].from, store.transitions[0].to)
	}
	if store.transitions[1].from != domain.StatusSending || store.transitions[1].to != domain.StatusFailed {
		t.Errorf("transition[1] = %v->%v, want sending->failed",
			store.transitions[1].from, store.transitions[1].to)
	}
}
