package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// listStubStore returns pre-populated notifications for list tests.
type listStubStore struct {
	stubNotificationStore
}

func newListStubStore(notifications ...*domain.Notification) *listStubStore {
	s := &listStubStore{
		stubNotificationStore: stubNotificationStore{
			notifications: make(map[string]*domain.Notification),
		},
	}
	for _, n := range notifications {
		s.notifications[n.Email] = n
	}
	return s
}

// Override ListNotifications to return ordered results with pagination.
func (s *listStubStore) ListNotifications(_ context.Context, after string, limit int) ([]*domain.Notification, error) {
	var all []*domain.Notification
	for _, n := range s.notifications {
		all = append(all, n)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	start := 0
	if after != "" {
		for i, n := range all {
			if n.ID == after {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], nil
}

func (s *listStubStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
}

func makeNotifications(count int) []*domain.Notification {
	var result []*domain.Notification
	for i := 0; i < count; i++ {
		result = append(result, &domain.Notification{
			ID:         fmt.Sprintf("ntf_%03d", i+1),
			Email:      fmt.Sprintf("user%03d@test.com", i+1),
			Status:     domain.StatusPending,
			RetryCount: 0,
			RetryLimit: domain.DefaultRetryLimit,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
	}
	return result
}

func TestHandleListDefault(t *testing.T) {
	notifications := makeNotifications(3)
	store := newListStubStore(notifications...)
	handler := HandleList(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Notifications) != 3 {
		t.Fatalf("len = %d, want 3", len(resp.Notifications))
	}

	// Verify each notification has the required fields.
	n := resp.Notifications[0]
	if n.ID == "" {
		t.Error("notification missing id")
	}
	if n.Email == "" {
		t.Error("notification missing email")
	}

	if resp.Meta.HasMore {
		t.Error("has_more = true, want false (only 3 items)")
	}
}

func TestHandleListPagination(t *testing.T) {
	notifications := makeNotifications(5)
	store := newListStubStore(notifications...)
	handler := HandleList(store)

	// First page: limit=2.
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?limit=2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("page 1 status = %d, want %d", rec.Code, http.StatusOK)
	}

	var page1 listResponse
	if err := json.NewDecoder(rec.Body).Decode(&page1); err != nil {
		t.Fatalf("decode page 1: %v", err)
	}

	if len(page1.Notifications) != 2 {
		t.Fatalf("page 1 len = %d, want 2", len(page1.Notifications))
	}
	if !page1.Meta.HasMore {
		t.Fatal("page 1 has_more = false, want true")
	}
	if page1.Meta.NextCursor == "" {
		t.Fatal("page 1 next_cursor is empty")
	}

	// Second page: use the cursor.
	req2 := httptest.NewRequest(http.MethodGet,
		"/v1/notifications?limit=2&after="+page1.Meta.NextCursor, nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	var page2 listResponse
	if err := json.NewDecoder(rec2.Body).Decode(&page2); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}

	if len(page2.Notifications) != 2 {
		t.Fatalf("page 2 len = %d, want 2", len(page2.Notifications))
	}
	if !page2.Meta.HasMore {
		t.Fatal("page 2 has_more = false, want true")
	}

	// Third page: last item.
	req3 := httptest.NewRequest(http.MethodGet,
		"/v1/notifications?limit=2&after="+page2.Meta.NextCursor, nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	var page3 listResponse
	if err := json.NewDecoder(rec3.Body).Decode(&page3); err != nil {
		t.Fatalf("decode page 3: %v", err)
	}

	if len(page3.Notifications) != 1 {
		t.Fatalf("page 3 len = %d, want 1", len(page3.Notifications))
	}
	if page3.Meta.HasMore {
		t.Error("page 3 has_more = true, want false")
	}
}

func TestHandleListEmpty(t *testing.T) {
	store := newListStubStore()
	handler := HandleList(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Notifications) != 0 {
		t.Errorf("len = %d, want 0", len(resp.Notifications))
	}
	if resp.Meta.HasMore {
		t.Error("has_more = true, want false")
	}
}

func TestHandleListTotalCount(t *testing.T) {
	notifications := makeNotifications(25)
	store := newListStubStore(notifications...)
	handler := HandleList(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?limit=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Meta.TotalCount != 25 {
		t.Errorf("total_count = %d, want 25", resp.Meta.TotalCount)
	}
	if resp.Meta.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", resp.Meta.TotalPages)
	}
	if !resp.Meta.HasMore {
		t.Error("has_more = false, want true")
	}
}

func TestHandleListTotalCountEmpty(t *testing.T) {
	store := newListStubStore()
	handler := HandleList(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Meta.TotalCount != 0 {
		t.Errorf("total_count = %d, want 0", resp.Meta.TotalCount)
	}
	if resp.Meta.TotalPages != 0 {
		t.Errorf("total_pages = %d, want 0", resp.Meta.TotalPages)
	}
}

func TestHandleListTotalPagesRoundsUp(t *testing.T) {
	notifications := makeNotifications(21)
	store := newListStubStore(notifications...)
	handler := HandleList(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?limit=20", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Meta.TotalCount != 21 {
		t.Errorf("total_count = %d, want 21", resp.Meta.TotalCount)
	}
	if resp.Meta.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2", resp.Meta.TotalPages)
	}
}

// Silence unused import warning for base64.
var _ = base64.StdEncoding
