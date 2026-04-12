package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// stubNotificationStore is a minimal in-memory store for handler tests.
type stubNotificationStore struct {
	notifications map[string]*domain.Notification
	enqueued      []string // emails enqueued for delivery
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
	return nil, nil
}

// stubEnqueuer captures enqueue calls without a real goqite queue.
type stubEnqueuer struct {
	jobs [][]byte
}

func (e *stubEnqueuer) Enqueue(_ context.Context, payload []byte) error {
	e.jobs = append(e.jobs, payload)
	return nil
}

func TestHandleNotifySuccess(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp["id"], "ntf_") {
		t.Errorf("id = %q, want ntf_ prefix", resp["id"])
	}

	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(enqueuer.jobs))
	}
}

func TestHandleNotifyDuplicate(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		Email: "user@company.com",
	}
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "already notified") {
		t.Errorf("error = %q, want 'already notified'", resp["error"])
	}
}

func TestHandleNotifyInvalidEmail(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "not-an-email"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleNotifyEmptyBody(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// Verify the stub implements the interface used by the handler.
func TestStubStoreImplementsNotificationStore(t *testing.T) {
	var _ domain.NotificationStore = newStubStore()
}

func TestStubEnqueuerImplementsEnqueuer(t *testing.T) {
	var _ Enqueuer = &stubEnqueuer{}
}

func TestHandleNotifyRequestIDPropagation(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Set a request ID in the context (simulates the middleware).
	ctx := context.WithValue(req.Context(), requestIDKey, "req_test-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Verify the enqueued job payload contains the request ID.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(enqueuer.jobs))
	}
	var payload map[string]string
	if err := json.Unmarshal(enqueuer.jobs[0], &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["request_id"] != "req_test-123" {
		t.Errorf("request_id = %q, want %q", payload["request_id"], "req_test-123")
	}
}
