package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qmuntal/stateless"

	"github.com/workfort/notifier/internal/domain"
)

// resetStubStore extends stubNotificationStore with the state machine
// accessor/mutator and transition logger required by HandleReset. The
// accessor/mutator read and write the notification's Status field in
// the in-memory map, so the state machine drives the actual transition.
type resetStubStore struct {
	stubNotificationStore
	transitions []string // records "from->to" for LogTransition calls
}

func newResetStubStore() *resetStubStore {
	return &resetStubStore{
		stubNotificationStore: stubNotificationStore{
			notifications: make(map[string]*domain.Notification),
		},
	}
}

func (s *resetStubStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				return n.Status, nil
			}
		}
		return nil, domain.ErrNotFound
	}
}

func (s *resetStubStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
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

func (s *resetStubStore) LogTransition(_ context.Context, _, _ string, from, to domain.Status, _ domain.Trigger) error {
	s.transitions = append(s.transitions, from.String()+"->"+to.String())
	return nil
}

func TestHandleResetSuccess(t *testing.T) {
	store := newResetStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_reset-1",
		Email:      "user@company.com",
		Status:     domain.StatusDelivered,
		RetryCount: 2,
		RetryLimit: domain.DefaultRetryLimit,
	}

	enqueuer := &stubEnqueuer{}
	handler := HandleReset(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// 204 must have an empty body.
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}

	// Verify the notification was reset.
	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}

	// Verify a transition was logged.
	if len(store.transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(store.transitions))
	}
	if store.transitions[0] != "delivered->pending" {
		t.Errorf("transition = %q, want %q", store.transitions[0], "delivered->pending")
	}

	// REQ-007: Verify a delivery job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(enqueuer.jobs))
	}
}

func TestHandleResetNotFound(t *testing.T) {
	store := newResetStubStore()
	handler := HandleReset(store, &stubEnqueuer{})

	body := `{"email": "nobody@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "not found") {
		t.Errorf("error = %q, want 'not found'", resp["error"])
	}
}

func TestHandleResetInvalidJSON(t *testing.T) {
	store := newResetStubStore()
	handler := HandleReset(store, &stubEnqueuer{})

	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetRetriesRemaining(t *testing.T) {
	store := newResetStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_reset-guard",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleReset(store, &stubEnqueuer{})

	body := `{"email": "retry@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "notification has retries remaining" {
		t.Errorf("error = %q, want %q", resp["error"], "notification has retries remaining")
	}
}

func TestHandleResetFromNotSentRetriesExhausted(t *testing.T) {
	store := newResetStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_reset-2",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: domain.DefaultRetryLimit,
	}

	enqueuer := &stubEnqueuer{}
	handler := HandleReset(store, enqueuer)

	body := `{"email": "retry@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	n := store.notifications["retry@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}

	// REQ-007: Verify a delivery job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(enqueuer.jobs))
	}
}

func TestHandleResetOversizedBody(t *testing.T) {
	store := newResetStubStore()
	handler := HandleReset(store, &stubEnqueuer{})

	// Create a body larger than 1 MB.
	body := make([]byte, 1<<20+1)
	for i := range body {
		body[i] = 'a'
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
