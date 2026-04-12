package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func (s *stubNotificationStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
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

// --- Fail store for error-leakage tests ---

type failStore struct {
	stubNotificationStore
}

func (s *failStore) CreateNotification(_ context.Context, _ *domain.Notification) error {
	return fmt.Errorf("pq: connection refused to 10.0.0.5:5432")
}

func (s *failStore) GetNotificationByEmail(_ context.Context, _ string) (*domain.Notification, error) {
	return nil, fmt.Errorf("pq: connection refused to 10.0.0.5:5432")
}

func (s *failStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, fmt.Errorf("pq: relation \"notifications\" does not exist")
}

func (s *failStore) CountNotifications(_ context.Context) (int, error) {
	return 0, fmt.Errorf("pq: relation \"notifications\" does not exist")
}

// --- Fail enqueuer for error-leakage tests ---

type failEnqueuer struct{}

func (e *failEnqueuer) Enqueue(_ context.Context, _ []byte) error {
	return fmt.Errorf("goqite: queue full, 10.0.0.5:5432 not responding")
}

// --- Fail store for state machine error path (tools.go line 86) ---

type failStateMachineStore struct {
	stubNotificationStore
}

func (s *failStateMachineStore) NotificationStateMutator(_ string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, _ stateless.State) error {
		return fmt.Errorf("pq: could not serialize access due to concurrent update on 10.0.0.5:5432")
	}
}

// --- Fail store for update error path (tools.go line 96) ---

type failUpdateStore struct {
	stubNotificationStore
}

func (s *failUpdateStore) UpdateNotification(_ context.Context, _ *domain.Notification) error {
	return fmt.Errorf("pq: deadlock detected on 10.0.0.5:5432")
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

func TestSendNotificationToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestSendNotificationToolEnqueueErrorSanitized(t *testing.T) {
	store := newStubStore()
	enqueuer := &failEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for enqueue failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "goqite") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestResetNotificationToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestListNotificationsToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	handler := HandleListNotifications(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "relation") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestResetNotificationToolStateMachineErrorSanitized(t *testing.T) {
	store := &failStateMachineStore{
		stubNotificationStore: stubNotificationStore{
			notifications: map[string]*domain.Notification{
				"user@company.com": {
					ID:         "ntf_sm",
					Email:      "user@company.com",
					Status:     domain.StatusDelivered,
					RetryCount: 0,
					RetryLimit: domain.DefaultRetryLimit,
				},
			},
		},
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for state machine failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") || strings.Contains(text, "serialize") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestListNotificationsToolTotalCount(t *testing.T) {
	store := newStubStore()
	for i := 0; i < 5; i++ {
		store.notifications[fmt.Sprintf("user%d@test.com", i)] = &domain.Notification{
			ID:     fmt.Sprintf("ntf_mcp-%d", i),
			Email:  fmt.Sprintf("user%d@test.com", i),
			Status: domain.StatusPending,
		}
	}
	handler := HandleListNotifications(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"limit": 2}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := result.Content[0].(gomcp.TextContent).Text
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	meta, ok := parsed["meta"].(map[string]any)
	if !ok {
		t.Fatal("missing meta object in response")
	}

	totalCount := int(meta["total_count"].(float64))
	if totalCount != 5 {
		t.Errorf("total_count = %d, want 5", totalCount)
	}

	totalPages := int(meta["total_pages"].(float64))
	if totalPages != 3 {
		t.Errorf("total_pages = %d, want 3", totalPages)
	}
}

func TestResetNotificationToolUpdateErrorSanitized(t *testing.T) {
	store := &failUpdateStore{
		stubNotificationStore: stubNotificationStore{
			notifications: map[string]*domain.Notification{
				"user@company.com": {
					ID:         "ntf_upd",
					Email:      "user@company.com",
					Status:     domain.StatusDelivered,
					RetryCount: 0,
					RetryLimit: domain.DefaultRetryLimit,
				},
			},
		},
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for update failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") || strings.Contains(text, "deadlock") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}
