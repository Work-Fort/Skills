package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/qmuntal/stateless"

	gomcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/workfort/notifier/internal/domain"
)

// --- Stub store for MCP handler tests ---

type stubNotificationStore struct {
	notifications map[string]*domain.Notification
	transitions   []string
}

func newStubStore() *stubNotificationStore {
	return &stubNotificationStore{
		notifications: make(map[string]*domain.Notification),
	}
}

func (s *stubNotificationStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	if _, exists := s.notifications[n.Email]; exists {
		return domain.ErrAlreadyNotified
	}
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (s *stubNotificationStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	var list []*domain.Notification
	for _, n := range s.notifications {
		list = append(list, n)
	}
	return list, nil
}

func (s *stubNotificationStore) Ping(_ context.Context) error {
	return nil
}

func (s *stubNotificationStore) LogTransition(_ context.Context, _, _ string, from, to domain.Status, _ domain.Trigger) error {
	s.transitions = append(s.transitions, from.String()+"->"+to.String())
	return nil
}

func (s *stubNotificationStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				return n.Status, nil
			}
		}
		return nil, domain.ErrNotFound
	}
}

func (s *stubNotificationStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, state stateless.State) error {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				n.Status = state.(domain.Status)
				return nil
			}
		}
		return domain.ErrNotFound
	}
}

// --- Stub enqueuer ---

type stubEnqueuer struct {
	jobs [][]byte
}

func (e *stubEnqueuer) Enqueue(_ context.Context, payload []byte) error {
	e.jobs = append(e.jobs, payload)
	return nil
}

// --- Tests ---

func TestSendNotificationTool(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should contain the notification ID.
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	// Verify the notification was created.
	if len(store.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(store.notifications))
	}

	// Verify a job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(enqueuer.jobs))
	}
}

func TestSendNotificationToolDuplicate(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		Email: "user@company.com",
	}
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return an error result, not a Go error.
	if result.IsError != true {
		t.Error("expected IsError to be true for duplicate")
	}
}

func TestSendNotificationToolInvalidEmail(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "not-an-email"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError != true {
		t.Error("expected IsError for invalid email")
	}
}

func TestResetNotificationTool(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_mcp-reset-1",
		Email:      "user@company.com",
		Status:     domain.StatusDelivered,
		RetryCount: 2,
		RetryLimit: domain.DefaultRetryLimit,
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError == true {
		t.Error("unexpected error result for reset")
	}

	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", n.Status)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}
}

func TestResetNotificationToolNotFound(t *testing.T) {
	store := newStubStore()
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "nobody@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError != true {
		t.Error("expected IsError for not found")
	}
}

func TestListNotificationsTool(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:     "ntf_list-1",
		Email:  "user@company.com",
		Status: domain.StatusDelivered,
	}
	handler := HandleListNotifications(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	// The result text should be JSON containing the notification.
	text := result.Content[0].(gomcp.TextContent).Text
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	notifications, ok := parsed["notifications"].([]any)
	if !ok || len(notifications) == 0 {
		t.Error("expected notifications array with items")
	}
}

func TestCheckHealthTool(t *testing.T) {
	store := newStubStore()
	handler := HandleCheckHealth(store)

	req := gomcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError == true {
		t.Error("unexpected error result for healthy check")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text == "" {
		t.Error("expected non-empty health result text")
	}
}
