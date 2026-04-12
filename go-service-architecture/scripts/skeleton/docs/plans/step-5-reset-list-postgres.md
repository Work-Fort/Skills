---
type: plan
step: "5"
title: "Reset, List, and PostgreSQL"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "5"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-2-cli-and-database
  - step-3-notification-delivery
  - step-4-state-machine
---

# Step 5: Reset, List, and PostgreSQL

## Overview

Deliver three capabilities: the reset endpoint (`POST /v1/notify/reset`)
that clears a notification and makes the email address eligible for
re-notification, the paginated list endpoint
(`GET /v1/notifications`) with cursor-based pagination, and the
PostgreSQL store implementation that makes the service deployable to
production multi-node environments.

After this step:

- A previously notified email address can be reset and re-notified.
- All notifications can be listed with cursor-based pagination.
- The service runs on either SQLite or PostgreSQL, selected by DSN
  prefix, with the daemon wiring updated for dual-backend support.

## Prerequisites

- Step 4 completed: state machine with `stateless`, audit log,
  retry count/limit guards, worker using state machine for all
  transitions, QA seed for all terminal states
- Go 1.26.0 (pinned in `mise.toml`)
- `mise` CLI available on PATH
- PostgreSQL available for integration testing (local or container)

## New Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/jackc/pgx/v5` | v5.7.5 | PostgreSQL driver (via `pgx/v5/stdlib` for `database/sql` compatibility) |

Existing: `modernc.org/sqlite`, `github.com/google/uuid`,
`github.com/pressly/goose/v3`, `maragu.dev/goqite`,
`github.com/qmuntal/stateless`, `github.com/wneessen/go-mail`,
`github.com/spf13/cobra`, `github.com/knadh/koanf/v2`
(all from Steps 1-4).

The PostgreSQL store uses `pgx/v5/stdlib` to obtain a `*sql.DB`,
which is required for sharing the database connection with goqite
(service-database REQ-019). The `pgx/v5/stdlib` adapter registers
the `"pgx"` driver with `database/sql`, and `sql.Open("pgx", dsn)`
returns a standard `*sql.DB`. This keeps the same `database/sql`
interface used by the SQLite store and goqite.

Note: `go mod tidy` resolves exact versions. The version above is the
target -- `go mod tidy` may select a compatible patch version.

## Spec Traceability

Tasks trace to two specs:

- `openspec/specs/notification-management/spec.md` -- REQ-001 through
  REQ-010 (reset and list endpoints)
- `openspec/specs/service-database/spec.md` -- REQ-009 through REQ-020
  (PostgreSQL store, migrations, shared connection, port interfaces)

The health endpoint (notification-management REQ-011 through REQ-017)
was delivered in Step 2. This plan does not modify it.

**Deferred spec requirements:**

- **REQ-016** (huma framework): The spec requires REST endpoints
  (except health) to be registered using the `huma` framework via
  `humago.New` and `huma.Register`. The current codebase registers
  endpoints directly on `http.ServeMux`. Huma integration will be
  addressed in a future step.
- **REQ-018** (1 MB request body limit): The spec requires request
  body size to be limited to 1 MB via `http.MaxBytesReader`. Neither
  the existing notify handler nor the new reset handler applies this
  limit. Body size limiting will be addressed alongside huma
  integration in a future step.

## Tasks

### Task 1: Reset Handler

Satisfies: notification-management REQ-001 (POST /v1/notify/reset with
email body), REQ-002 (transition to pending via state machine), REQ-003
(404 if not found), REQ-004 (clear retry_count), REQ-005 (clear
delivery results), REQ-006 (reset timestamps), REQ-007 (eligible for
re-notification), REQ-007a (204 No Content with empty body).

Note on REQ-005: The spec says "clear delivery results." The current
domain model has no separate delivery results field -- the delivery
outcome is represented by the combination of `Status` and
`RetryCount`. Resetting the status to `pending` and clearing
`RetryCount` to 0 satisfies REQ-005 under the current schema. If the
spec intended a future `delivery_result` field, a spec update should
be filed.

**Files:**
- Create: `internal/infra/httpapi/reset.go`
- Test: `internal/infra/httpapi/reset_test.go`

**Step 1: Write the failing test**

Create `internal/infra/httpapi/reset_test.go`:

```go
package httpapi

import (
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

	handler := HandleReset(store)

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
}

func TestHandleResetNotFound(t *testing.T) {
	store := newResetStubStore()
	handler := HandleReset(store)

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
	handler := HandleReset(store)

	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetFromNotSent(t *testing.T) {
	store := newResetStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_reset-2",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleReset(store)

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
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestHandleReset" ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: HandleReset"

**Step 3: Write the implementation**

Create `internal/infra/httpapi/reset.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/workfort/notifier/internal/domain"
)

// ResetStore defines the storage interface needed by HandleReset. It
// combines notification CRUD, state machine accessor/mutator, and
// transition logging -- the same shape as the worker's store but
// consumed from the HTTP handler side.
type ResetStore interface {
	domain.NotificationStore
	domain.TransitionLogger
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// resetRequest is the JSON body for POST /v1/notify/reset.
type resetRequest struct {
	Email string `json:"email"`
}

// HandleReset returns an http.HandlerFunc for POST /v1/notify/reset.
// It looks up the notification by email, transitions it back to pending
// via the state machine (TriggerReset, satisfying REQ-002), clears the
// retry count and timestamps, logs the transition, and returns 204 No
// Content.
func HandleReset(store ResetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		// Look up the notification by email.
		n, err := store.GetNotificationByEmail(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "not found",
				})
				return
			}
			slog.Error("get notification for reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-002: Transition to pending via the state machine. This
		// validates that TriggerReset is permitted from the current state
		// and updates the status through the mutator.
		prevStatus := n.Status
		sm := domain.ConfigureStateMachine(
			store.NotificationStateAccessor(n.ID),
			store.NotificationStateMutator(n.ID),
			n.RetryLimit,
			n.RetryCount,
		)
		if err := sm.FireCtx(r.Context(), domain.TriggerReset); err != nil {
			slog.Error("state machine reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// Log the transition for the audit trail.
		if err := store.LogTransition(r.Context(), "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset); err != nil {
			slog.Error("log reset transition failed", "error", err)
			// Non-fatal: the reset itself succeeded.
		}

		// Clear retry count and timestamps (except created_at).
		// REQ-004: clear retry_count. REQ-005: clear delivery results
		// (status already reset by state machine; retry_count is the
		// remaining delivery result field). REQ-006: reset timestamps.
		n.RetryCount = 0
		n.UpdatedAt = time.Time{}

		if err := store.UpdateNotification(r.Context(), n); err != nil {
			slog.Error("update notification for reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-007a: 204 No Content with empty body.
		w.WriteHeader(http.StatusNoContent)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestHandleReset" ./internal/infra/httpapi/...`
Expected: PASS (4 tests)

**Step 5: Commit**

`feat(httpapi): add reset endpoint (POST /v1/notify/reset, 204)`

---

### Task 2: List Handler with Cursor-Based Pagination

Satisfies: notification-management REQ-008 (GET /v1/notifications),
REQ-009 (cursor-based pagination with `after`, `limit`, `meta`),
REQ-010 (response includes id, email, state, retry_count, retry_limit,
created_at, updated_at).

**Depends on:** Task 1 (shared `writeJSON` helper, stub store pattern)

**Files:**
- Create: `internal/infra/httpapi/list.go`
- Test: `internal/infra/httpapi/list_test.go`

**Step 1: Write the failing test**

Create `internal/infra/httpapi/list_test.go`:

```go
package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	// Collect all notifications sorted by creation order (simplified).
	var all []*domain.Notification
	for _, n := range s.notifications {
		all = append(all, n)
	}
	// Sort by ID for deterministic order in tests.
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	// Apply cursor.
	start := 0
	if after != "" {
		for i, n := range all {
			if n.ID == after {
				start = i + 1
				break
			}
		}
	}

	// Apply limit.
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], nil
}
```

Note: the test file needs `context` and `sort` imports. The full
test file is:

```go
package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
```

Note: the test file also needs a `fmt` import for `makeNotifications`.
The full import block is: `context`, `encoding/base64`, `encoding/json`,
`fmt`, `net/http`, `net/http/httptest`, `sort`, `testing`, `time`.

**Step 2: Run test to verify it fails**

Run: `go test -run "TestHandleList" ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: HandleList"

**Step 3: Write the implementation**

Create `internal/infra/httpapi/list.go`:

```go
package httpapi

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/workfort/notifier/internal/domain"
)

const defaultPageLimit = 20
const maxPageLimit = 100

// listNotificationItem is the JSON representation of a notification
// in the list response.
type listNotificationItem struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	State      string `json:"state"`
	RetryCount int    `json:"retry_count"`
	RetryLimit int    `json:"retry_limit"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// listMeta is the pagination metadata in the list response.
type listMeta struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// listResponse is the JSON response for GET /v1/notifications.
type listResponse struct {
	Notifications []listNotificationItem `json:"notifications"`
	Meta          listMeta               `json:"meta"`
}

// HandleList returns an http.HandlerFunc for GET /v1/notifications.
// It supports cursor-based pagination via `after` (base64-encoded ID)
// and `limit` query parameters.
func HandleList(store domain.NotificationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse pagination parameters.
		limit := defaultPageLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > maxPageLimit {
			limit = maxPageLimit
		}

		// Decode cursor: the `after` param is a base64-encoded
		// notification ID.
		var afterID string
		if v := r.URL.Query().Get("after"); v != "" {
			decoded, err := base64.StdEncoding.DecodeString(v)
			if err == nil {
				afterID = string(decoded)
			}
		}

		// Fetch limit+1 to determine if there are more results.
		notifications, err := store.ListNotifications(r.Context(), afterID, limit+1)
		if err != nil {
			slog.Error("list notifications failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		hasMore := len(notifications) > limit
		if hasMore {
			notifications = notifications[:limit]
		}

		// Build response items.
		items := make([]listNotificationItem, 0, len(notifications))
		for _, n := range notifications {
			items = append(items, listNotificationItem{
				ID:         n.ID,
				Email:      n.Email,
				State:      n.Status.String(),
				RetryCount: n.RetryCount,
				RetryLimit: n.RetryLimit,
				CreatedAt:  n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt:  n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		// Build pagination metadata.
		meta := listMeta{HasMore: hasMore}
		if hasMore && len(notifications) > 0 {
			lastID := notifications[len(notifications)-1].ID
			meta.NextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
		}

		writeJSON(w, http.StatusOK, listResponse{
			Notifications: items,
			Meta:          meta,
		})
	}
}
```

Pagination strategy: the handler requests `limit+1` rows from the
store. If `limit+1` rows come back, `has_more` is true and the extra
row is trimmed. The cursor is the base64-encoded ID of the last
returned notification. The store's `ListNotifications` uses
`(created_at, id)` ordering to produce a stable, deterministic sort.

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestHandleList" ./internal/infra/httpapi/...`
Expected: PASS (3 tests)

**Step 5: Commit**

`feat(httpapi): add paginated list endpoint (GET /v1/notifications)`

---

### Task 3: Wire Reset and List into Daemon

Satisfies: notification-management REQ-001 (reset endpoint route),
REQ-008 (list endpoint route).

**Depends on:** Task 1 (HandleReset), Task 2 (HandleList)

**Files:**
- Modify: `cmd/daemon/daemon.go:120-123`

**Step 1: Register the new routes**

Add the reset and list routes to the mux in `cmd/daemon/daemon.go`,
after the existing `POST /v1/notify` registration:

```go
	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
	mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
	mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))
```

**Step 2: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 3: Commit**

`feat(daemon): wire reset and list endpoints`

---

### Task 4: SQLite Store ListNotifications Limit+1 Adjustment

Satisfies: notification-management REQ-009 (cursor-based pagination).

The existing `ListNotifications` in `internal/infra/sqlite/store.go`
already accepts `after` and `limit` parameters and returns correct
results. The handler passes `limit+1` to detect `has_more`. No store
changes are needed -- the existing implementation handles this
correctly. This task verifies the existing store behaviour matches the
handler's expectations through the existing `TestListNotifications`
test.

**Files:**
- Test: `internal/infra/sqlite/store_test.go` (existing tests suffice)

**Step 1: Verify existing pagination tests pass**

Run: `go test -run "TestListNotifications" ./internal/infra/sqlite/...`
Expected: PASS

**Step 2: Commit** (no commit needed -- no changes)

---

### Task 5: PostgreSQL Migrations

Satisfies: service-database REQ-009 (pgx/v5 driver), REQ-013 (goose
migrations), REQ-014 (embedded SQL), REQ-015 (sequential numbering),
REQ-016 (goose Up/Down markers), REQ-018 (parameterized `$1`
placeholders).

**Files:**
- Create: `internal/infra/postgres/migrations/001_init.sql`
- Create: `internal/infra/postgres/migrations/002_goqite.sql`
- Create: `internal/infra/postgres/migrations/003_state_transitions.sql`

**Step 1: Create the init migration**

Create `internal/infra/postgres/migrations/001_init.sql`:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    status      INTEGER NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    retry_limit INTEGER NOT NULL DEFAULT 3,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_email ON notifications(email);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_status;
DROP INDEX IF EXISTS idx_notifications_email;
DROP TABLE IF EXISTS notifications;
```

Note: PostgreSQL uses `TIMESTAMPTZ` with `NOW()` instead of SQLite's
`TEXT` with `strftime()`. The `status` column remains `INTEGER` for
compatibility with the iota-based `domain.Status` type.

**Step 2: Create the goqite migration**

Create `internal/infra/postgres/migrations/002_goqite.sql`:

```sql
-- +goose Up
CREATE FUNCTION goqite_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated = NOW();
   RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE goqite (
    id       TEXT PRIMARY KEY DEFAULT ('m_' || encode(gen_random_bytes(16), 'hex')),
    created  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    queue    TEXT NOT NULL,
    body     BYTEA NOT NULL,
    timeout  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received INTEGER NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 0
);

CREATE TRIGGER goqite_updated_timestamp
BEFORE UPDATE ON goqite
FOR EACH ROW EXECUTE FUNCTION goqite_update_timestamp();

CREATE INDEX goqite_queue_priority_created_idx ON goqite (queue, priority DESC, created);

-- +goose Down
DROP INDEX IF EXISTS goqite_queue_priority_created_idx;
DROP TRIGGER IF EXISTS goqite_updated_timestamp ON goqite;
DROP FUNCTION IF EXISTS goqite_update_timestamp();
DROP TABLE IF EXISTS goqite;
```

This is the goqite PostgreSQL schema (`schema_postgres.sql`) wrapped in
goose markers. The `gen_random_bytes` function is built-in since
PostgreSQL 13, so no `pgcrypto` extension is needed. The trigger uses
`EXECUTE FUNCTION` (modern PostgreSQL 11+ syntax) instead of the
deprecated `EXECUTE PROCEDURE`. The trigger function is named
`goqite_update_timestamp` to avoid collisions with any
application-level `update_timestamp` functions.

**Step 3: Create the state_transitions migration**

Create `internal/infra/postgres/migrations/003_state_transitions.sql`:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS state_transitions (
    id          SERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_state_transitions_entity
    ON state_transitions(entity_type, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_state_transitions_entity;
DROP TABLE IF EXISTS state_transitions;
```

Note: `SERIAL` replaces SQLite's `INTEGER PRIMARY KEY AUTOINCREMENT`.
`TIMESTAMPTZ` with `NOW()` replaces `TEXT` with `strftime()`.

**Step 4: Commit**

`feat(db): add PostgreSQL migrations for notifications, goqite, and audit log`

---

### Task 6: PostgreSQL Store Implementation

Satisfies: service-database REQ-009 (pgx/v5 driver), REQ-010
(MaxOpenConns 25), REQ-011 (MaxIdleConns 5), REQ-012 (ConnMaxLifetime
5 min), REQ-013 (goose migrations), REQ-014 (embedded SQL), REQ-017
(goose.Up on open), REQ-018 ($1 placeholders), REQ-019 (shared *sql.DB),
REQ-020 (implements domain.Store).

**Depends on:** Task 5 (PostgreSQL migrations)

**Files:**
- Create: `internal/infra/postgres/store.go`
- Test: `internal/infra/postgres/store_test.go`

**Step 1: Add the pgx/v5 dependency**

Run: `cd /path/to/skeleton && go get github.com/jackc/pgx/v5@v5.7.5 && go mod tidy`
Expected: `go.mod` and `go.sum` updated with the pgx dependency

**Step 2: Write the failing test**

Create `internal/infra/postgres/store_test.go`:

```go
package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// testDSN reads the PostgreSQL DSN from the NOTIFIER_TEST_POSTGRES_DSN
// environment variable. Tests are skipped if not set.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("NOTIFIER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOTIFIER_TEST_POSTGRES_DSN not set; skipping PostgreSQL tests")
	}
	return dsn
}

func TestPostgresOpenAndPing(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestPostgresCreateAndGetNotification(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Clean up from previous test runs.
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications WHERE email = $1", "pgtest@test.com")

	n := &domain.Notification{
		ID:         "ntf_pg-001",
		Email:      "pgtest@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatalf("CreateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "pgtest@test.com")
	if err != nil {
		t.Fatalf("GetNotificationByEmail() error: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID = %q, want %q", got.ID, n.ID)
	}
	if got.Email != n.Email {
		t.Errorf("Email = %q, want %q", got.Email, n.Email)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("Status = %v, want %v", got.Status, domain.StatusPending)
	}
}

func TestPostgresCreateDuplicateReturnsAlreadyNotified(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications WHERE email = $1", "pgdup@test.com")

	n := &domain.Notification{
		ID:         "ntf_pgdup-1",
		Email:      "pgdup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n2 := &domain.Notification{
		ID:         "ntf_pgdup-2",
		Email:      "pgdup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	err = store.CreateNotification(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyNotified) {
		t.Errorf("error = %v, want ErrAlreadyNotified", err)
	}
}

func TestPostgresGetNotificationNotFound(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	_, err = store.GetNotificationByEmail(context.Background(), "pgnobody@test.com")
	if err == nil {
		t.Fatal("expected error for missing notification, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestPostgresListNotifications(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	// Clean up.
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications")

	for _, email := range []string{"pga@test.com", "pgb@test.com", "pgc@test.com"} {
		n := &domain.Notification{
			ID:         domain.NewID("ntf"),
			Email:      email,
			Status:     domain.StatusPending,
			RetryLimit: domain.DefaultRetryLimit,
		}
		if err := store.CreateNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.ListNotifications(ctx, "", 2)
	if err != nil {
		t.Fatalf("ListNotifications() error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}

	// Second page.
	list2, err := store.ListNotifications(ctx, list[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(list2))
	}
}

func TestPostgresLogTransition(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	if err := store.LogTransition(ctx, "notification", "ntf_pglog-1",
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition() error: %v", err)
	}

	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = $1",
		"ntf_pglog-1",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("transition count = %d, want 1", count)
	}
}

// Compile-time check: Store implements domain.Store.
var _ domain.Store = (*Store)(nil)
```

**Step 3: Run test to verify it fails**

Run: `go test -run "TestPostgres" ./internal/infra/postgres/...`
Expected: FAIL with "undefined: Open" (or skip if no DSN set)

**Step 4: Write the implementation**

Create `internal/infra/postgres/store.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/qmuntal/stateless"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver

	"github.com/workfort/notifier/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store implements domain.Store backed by PostgreSQL.
type Store struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB for sharing with goqite
// (service-database REQ-019).
func (s *Store) DB() *sql.DB {
	return s.db
}

// Open creates a new PostgreSQL store. The DSN must be a valid
// PostgreSQL connection string (e.g.,
// "postgres://user:pass@localhost:5432/notifydb?sslmode=disable").
// Migrations are run automatically on open.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// REQ-010: connection pool settings.
	db.SetMaxOpenConns(25)
	// REQ-011: idle connections.
	db.SetMaxIdleConns(5)
	// REQ-012: connection lifetime.
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connectivity before running migrations.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Run embedded goose migrations.
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// CreateNotification inserts a new notification record. Returns
// domain.ErrAlreadyNotified if the email already exists.
func (s *Store) CreateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, email, status, retry_count, retry_limit, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		n.ID, n.Email, int(n.Status), n.RetryCount, n.RetryLimit, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create notification %s: %w", n.Email, domain.ErrAlreadyNotified)
		}
		return fmt.Errorf("create notification: %w", err)
	}
	n.CreatedAt = now
	n.UpdatedAt = now
	return nil
}

// GetNotificationByEmail retrieves a notification by email address.
// Returns domain.ErrNotFound if no record exists.
func (s *Store) GetNotificationByEmail(ctx context.Context, email string) (*domain.Notification, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
		 FROM notifications WHERE email = $1`, email,
	)

	n := &domain.Notification{}
	var status int
	err := row.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get notification %s: %w", email, domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}
	n.Status = domain.Status(status)
	return n, nil
}

// UpdateNotification updates an existing notification record.
func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET status = $1, retry_count = $2, retry_limit = $3, updated_at = $4
		 WHERE id = $5`,
		int(n.Status), n.RetryCount, n.RetryLimit, now, n.ID,
	)
	if err != nil {
		return fmt.Errorf("update notification: %w", err)
	}
	n.UpdatedAt = now
	return nil
}

// ListNotifications returns notifications with cursor-based pagination.
// If after is empty, returns from the beginning. Limit controls page size.
func (s *Store) ListNotifications(ctx context.Context, after string, limit int) ([]*domain.Notification, error) {
	var rows *sql.Rows
	var err error
	if after == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications ORDER BY created_at ASC, id ASC LIMIT $1`, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications
			 WHERE (created_at, id) > (
			     SELECT created_at, id FROM notifications WHERE id = $1
			 )
			 ORDER BY created_at ASC, id ASC LIMIT $2`, after, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		var status int
		if err := rows.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Status = domain.Status(status)
		result = append(result, n)
	}
	return result, rows.Err()
}

// NotificationStateAccessor returns an accessor function for the
// stateless state machine. It reads the current status of a
// notification by ID from the database.
func (s *Store) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(ctx context.Context) (stateless.State, error) {
		var state int
		err := s.db.QueryRowContext(ctx,
			"SELECT status FROM notifications WHERE id = $1", notificationID,
		).Scan(&state)
		if err != nil {
			return nil, fmt.Errorf("read notification state: %w", err)
		}
		return domain.Status(state), nil
	}
}

// NotificationStateMutator returns a mutator function for the
// stateless state machine. It writes the new status to the database
// and updates the updated_at timestamp.
func (s *Store) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(ctx context.Context, state stateless.State) error {
		now := time.Now().UTC()
		_, err := s.db.ExecContext(ctx,
			"UPDATE notifications SET status = $1, updated_at = $2 WHERE id = $3",
			int(state.(domain.Status)), now, notificationID,
		)
		if err != nil {
			return fmt.Errorf("write notification state: %w", err)
		}
		return nil
	}
}

// LogTransition records a state transition in the audit log table.
// Stores human-readable string values.
func (s *Store) LogTransition(ctx context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entityType, entityID, from.String(), to.String(), trigger.String(),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("log transition: %w", err)
	}
	return nil
}

// isUniqueViolation checks if a PostgreSQL error is a UNIQUE constraint
// violation. pgx returns errors containing "duplicate key value
// violates unique constraint" or SQLSTATE code 23505.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
```

Key differences from the SQLite store:

- Uses `$1`, `$2`, ... placeholders instead of `?`.
- Scans `created_at`/`updated_at` directly into `time.Time` (PostgreSQL
  `TIMESTAMPTZ` maps natively to Go `time.Time` via pgx) instead of
  scanning into strings and parsing.
- Uses `"pgx"` as the `sql.Open` driver name (registered by the
  `pgx/v5/stdlib` blank import).
- Connection pool settings: `MaxOpenConns(25)`, `MaxIdleConns(5)`,
  `ConnMaxLifetime(5 * time.Minute)`.
- `isUniqueViolation` checks for PostgreSQL error code 23505 instead
  of SQLite's "UNIQUE constraint failed" string.

**Step 5: Run tests to verify they pass**

Run: `NOTIFIER_TEST_POSTGRES_DSN="postgres://localhost:5432/notifier_test?sslmode=disable" go test -run "TestPostgres" ./internal/infra/postgres/...`
Expected: PASS (6 tests) or SKIP if no DSN set

Run without DSN to verify skip:
`go test -run "TestPostgres" ./internal/infra/postgres/...`
Expected: SKIP (all tests skipped, no failures)

**Step 6: Commit**

`feat(postgres): add PostgreSQL store with pgx/v5 and embedded migrations`

---

### Task 7: Dual Backend Dispatcher

Satisfies: service-database REQ-001 (DSN-based backend selection).

**Files:**
- Create: `internal/infra/db/open.go`
- Test: `internal/infra/db/open_test.go`

**Step 1: Write the failing test**

Create `internal/infra/db/open_test.go`:

```go
package db

import (
	"strings"
	"testing"
)

func TestOpenSelectsSQLiteForEmptyDSN(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Verify it opened successfully (SQLite in-memory).
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestOpenSelectsSQLiteForFilePath(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(\":memory:\") error: %v", err)
	}
	defer func() { _ = store.Close() }()
}

func TestOpenSelectsPostgresForPostgresDSN(t *testing.T) {
	// This test verifies the dispatch logic. It will fail to connect
	// (no PostgreSQL running), but it should attempt the PostgreSQL
	// path, not the SQLite path. We test the branch by checking the
	// error message.
	_, err := Open("postgres://localhost:5432/nonexistent_db?sslmode=disable&connect_timeout=1")
	if err == nil {
		// If it connects, that is also fine.
		return
	}
	// The error should come from the postgres package, not sqlite.
	if !containsAny(err.Error(), "postgres", "pgx", "connect") {
		t.Errorf("expected postgres-related error, got: %v", err)
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestOpen" ./internal/infra/db/...`
Expected: FAIL with "undefined: Open" (package does not exist yet)

**Step 3: Write the implementation**

Create `internal/infra/db/open.go`:

```go
package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/qmuntal/stateless"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/postgres"
	"github.com/workfort/notifier/internal/infra/sqlite"
)

// backendStore extends domain.Store with the state machine
// accessor/mutator and DB() method that both sqlite.Store and
// postgres.Store implement.
type backendStore interface {
	domain.Store
	DB() *sql.DB
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// Store wraps either a SQLite or PostgreSQL backend behind the
// backendStore interface. All domain.Store methods delegate through
// the interface field, so adding new methods to domain.Store only
// requires updating the concrete stores -- the dispatcher stays
// unchanged.
type Store struct {
	store backendStore
	db    *sql.DB
}

// Open selects the database backend based on DSN prefix and returns
// a Store that satisfies domain.Store and exposes DB().
//
// - DSN starting with "postgres" selects PostgreSQL (REQ-001).
// - All other values (including empty) select SQLite (REQ-001).
func Open(dsn string) (*Store, error) {
	if strings.HasPrefix(dsn, "postgres") {
		pg, err := postgres.Open(dsn)
		if err != nil {
			return nil, err
		}
		return &Store{store: pg, db: pg.DB()}, nil
	}
	sq, err := sqlite.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{store: sq, db: sq.DB()}, nil
}

// DB returns the underlying *sql.DB for sharing with goqite
// (service-database REQ-019).
func (s *Store) DB() *sql.DB {
	return s.db
}

// --- domain.Store delegation (all through s.store) ---

func (s *Store) Ping(ctx context.Context) error {
	return s.store.Ping(ctx)
}

func (s *Store) CreateNotification(ctx context.Context, n *domain.Notification) error {
	return s.store.CreateNotification(ctx, n)
}

func (s *Store) GetNotificationByEmail(ctx context.Context, email string) (*domain.Notification, error) {
	return s.store.GetNotificationByEmail(ctx, email)
}

func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	return s.store.UpdateNotification(ctx, n)
}

func (s *Store) ListNotifications(ctx context.Context, after string, limit int) ([]*domain.Notification, error) {
	return s.store.ListNotifications(ctx, after, limit)
}

func (s *Store) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return s.store.NotificationStateAccessor(notificationID)
}

func (s *Store) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return s.store.NotificationStateMutator(notificationID)
}

func (s *Store) LogTransition(ctx context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	return s.store.LogTransition(ctx, entityType, entityID, from, to, trigger)
}

func (s *Store) Close() error {
	return s.store.Close()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestOpen" ./internal/infra/db/...`
Expected: PASS (3 tests -- the PostgreSQL test may show a connection
error but the dispatch logic is correct)

**Step 5: Commit**

`feat(db): add dual-backend dispatcher selecting SQLite or PostgreSQL by DSN`

---

### Task 8: Update Daemon to Use Dual Backend Dispatcher

Satisfies: service-database REQ-001 (DSN-based selection in daemon),
REQ-019 (shared *sql.DB with goqite).

**Depends on:** Task 7 (db.Open dispatcher)

**Files:**
- Modify: `cmd/daemon/daemon.go:81-91`

**Step 1: Replace sqlite.Open with db.Open**

In `cmd/daemon/daemon.go`, change the import and store opening. Replace:

```go
	"github.com/workfort/notifier/internal/infra/sqlite"
```

with:

```go
	"github.com/workfort/notifier/internal/infra/db"
```

And replace the store opening block:

```go
	// Open the store.
	store, err := sqlite.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
```

with:

```go
	// Open the store (SQLite or PostgreSQL based on DSN).
	store, err := db.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
```

The rest of the daemon code remains unchanged: `store.DB()` returns
the `*sql.DB` for goqite, and `store` satisfies `domain.NotificationStore`
and `domain.HealthChecker` through the dispatcher's forwarding methods.

**Step 2: Update the goqite queue creation for PostgreSQL**

The goqite queue needs to know which SQL flavor to use when the
backend is PostgreSQL. Modify the `queue.NewNotificationQueue`
function or pass a flavor parameter. The simplest approach is to
detect the backend from the DSN in the daemon and pass the flavor
through.

In `cmd/daemon/daemon.go`, after opening the store, determine the
flavor:

```go
	// Determine goqite SQL flavor from DSN.
	var queueFlavor queue.Flavor
	if strings.HasPrefix(cfg.DSN, "postgres") {
		queueFlavor = queue.FlavorPostgres
	}
```

And change the queue creation call from:

```go
	nq, err := queue.NewNotificationQueue(store.DB())
```

to:

```go
	nq, err := queue.NewNotificationQueue(store.DB(), queueFlavor)
```

**Step 3: Update queue package to accept flavor**

In `internal/infra/queue/queue.go`, add the flavor type and update
`NewNotificationQueue`:

```go
// Flavor selects the SQL dialect for goqite.
type Flavor int

const (
	FlavorSQLite   Flavor = iota // default
	FlavorPostgres
)

// NewNotificationQueue creates a goqite queue named "notifications"
// sharing the given *sql.DB (service-database REQ-019). The flavor
// parameter selects the SQL dialect for goqite.
func NewNotificationQueue(db *sql.DB, flavor Flavor) (*NotificationQueue, error) {
	opts := goqite.NewOpts{
		DB:         db,
		Name:       queueName,
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	}
	if flavor == FlavorPostgres {
		opts.SQLFlavor = goqite.SQLFlavorPostgreSQL
	}
	q := goqite.New(opts)
	return &NotificationQueue{q: q}, nil
}
```

**Step 4: Update queue tests**

In `internal/infra/queue/queue_test.go`, update any calls to
`NewNotificationQueue` to pass the default `FlavorSQLite`:

Replace any occurrence of `NewNotificationQueue(db)` with
`NewNotificationQueue(db, FlavorSQLite)`.

**Step 5: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 6: Verify all existing tests pass**

Run: `go test ./...`
Expected: PASS (all packages)

**Step 7: Commit**

`feat(daemon): use dual-backend dispatcher and pass goqite SQL flavor`

---

### Task 9: PostgreSQL Seed Support

Satisfies: service-database REQ-001 (PostgreSQL in QA builds).

The QA seed currently inserts rows using SQLite-compatible SQL. Since
PostgreSQL uses the same table schema (same column names, same integer
status values), the existing seed SQL works for both backends. The
goqite job enqueueing is done programmatically via `jobs.Create()`,
which also works for both backends.

However, the seed's goqite queue must use the correct SQL flavor. Update
the seed to accept a flavor parameter.

**Files:**
- Modify: `internal/infra/seed/seed_qa.go`
- Modify: `internal/infra/seed/seed_default.go`
- Modify: `cmd/daemon/daemon.go`

**Step 1: Update seed_qa.go to accept flavor**

Change the `RunSeed` function signature to accept a `goqite.SQLFlavor`:

```go
//go:build qa

package seed

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

//go:embed testdata/seed.sql
var seedSQL []byte

// SeedSQL returns the raw embedded seed SQL. Exposed for testing.
func SeedSQL() []byte {
	return seedSQL
}

// seedJobs defines the goqite jobs to enqueue for seed notifications.
var seedJobs = []struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}{
	{"ntf_seed-001", "alice@company.com", "req_seed-001"},
	{"ntf_seed-002", "bob@company.com", "req_seed-002"},
	{"ntf_seed-003", "charlie@example.com", "req_seed-003"},
	{"ntf_seed-006", "retry@company.com", "req_seed-006"},
}

// RunSeed executes the embedded seed SQL and enqueues delivery jobs.
// The sqlFlavor parameter selects the goqite SQL dialect (0 for SQLite,
// use goqite.SQLFlavorPostgreSQL for PostgreSQL).
func RunSeed(db *sql.DB, sqlFlavor ...goqite.SQLFlavor) error {
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed sql: %w", err)
	}

	opts := goqite.NewOpts{
		DB:         db,
		Name:       "notifications",
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	}
	if len(sqlFlavor) > 0 {
		opts.SQLFlavor = sqlFlavor[0]
	}
	q := goqite.New(opts)

	ctx := context.Background()
	for _, sj := range seedJobs {
		payload, err := json.Marshal(sj)
		if err != nil {
			return fmt.Errorf("marshal seed job %s: %w", sj.NotificationID, err)
		}
		if _, err := jobs.Create(ctx, q, "send_notification", goqite.Message{Body: payload}); err != nil {
			return fmt.Errorf("enqueue seed job %s: %w", sj.NotificationID, err)
		}
	}

	return nil
}
```

**Step 2: Update seed_default.go**

```go
//go:build !qa

package seed

import "database/sql"

// RunSeed is a no-op in non-QA builds.
func RunSeed(_ *sql.DB, _ ...any) error {
	return nil
}
```

The variadic `...any` accepts the optional flavor parameter without
importing goqite in the default build.

**Step 3: Update daemon to pass flavor to seed**

In `cmd/daemon/daemon.go`, change the seed call from:

```go
	if err := seed.RunSeed(store.DB()); err != nil {
```

to:

```go
	if err := seed.RunSeed(store.DB()); err != nil {
```

For PostgreSQL, pass the flavor. Since the seed uses variadic params,
only pass it when PostgreSQL is active:

```go
	// Run QA seed data (no-op in non-QA builds).
	if strings.HasPrefix(cfg.DSN, "postgres") {
		if err := seed.RunSeed(store.DB(), goqite.SQLFlavorPostgreSQL); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	} else {
		if err := seed.RunSeed(store.DB()); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	}
```

Add `"maragu.dev/goqite"` to the daemon imports.

**Step 4: Verify the project compiles for both builds**

Run: `go build ./...`
Expected: exits 0

Run: `go build -tags qa ./...`
Expected: exits 0

**Step 5: Verify the existing seed test still passes**

The signature change to `RunSeed(db *sql.DB, sqlFlavor ...goqite.SQLFlavor)`
is backward-compatible: the existing `seed_test.go` calls `RunSeed(db)`
which is valid with the variadic parameter. Confirm this explicitly.

Run: `go test -tags qa ./internal/infra/seed/...`
Expected: PASS

**Step 6: Commit**

`feat(seed): support PostgreSQL flavor in QA seed`

---

### Task 10: Integration Smoke Test

Satisfies: verification that all three capabilities work end-to-end.

**Files:**
- No new files -- uses existing test infrastructure

**Step 1: Run the full SQLite test suite**

Run: `go test ./...`
Expected: PASS (all packages)

**Step 2: Run the QA build test suite**

Run: `go build -tags qa ./...`
Expected: exits 0 (compiles with no errors)

Run: `go test -tags qa ./...`
Expected: PASS (all packages)

**Step 3: Run linter**

Run: `mise run lint:go`
Expected: exits 0 (no warnings)

**Step 4: Commit** (no commit needed -- verification only)

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings
- [ ] `go build -tags qa ./...` succeeds with no warnings
- [ ] `go test ./...` passes (all packages)
- [ ] `go test -tags qa ./...` passes (all packages)
- [ ] `mise run lint:go` produces no warnings
- [ ] `POST /v1/notify/reset` with existing email returns 204 No Content with empty body (REQ-007a)
- [ ] `POST /v1/notify/reset` with non-existent email returns 404 (REQ-003)
- [ ] After reset, `POST /v1/notify` with same email returns 202 (REQ-007)
- [ ] Reset clears `retry_count` to 0 (REQ-004)
- [ ] Reset transitions notification to `pending` state via `TriggerReset` through the state machine (REQ-002)
- [ ] Reset logs the transition in the audit trail via `LogTransition`
- [ ] `GET /v1/notifications` returns paginated results with `meta` object (REQ-009)
- [ ] `meta.has_more` is true when more results exist, false otherwise
- [ ] `meta.next_cursor` is base64-encoded and usable as `after` param
- [ ] Each notification in list includes id, email, state, retry_count, retry_limit, created_at, updated_at (REQ-010)
- [ ] `GET /v1/notifications?limit=2` returns at most 2 items
- [ ] PostgreSQL store opens with `postgres://` DSN and runs migrations
- [ ] PostgreSQL store implements all `domain.Store` methods
- [ ] PostgreSQL uses `$1` placeholders, not `?` (REQ-018)
- [ ] PostgreSQL connection pool: MaxOpenConns=25, MaxIdleConns=5, ConnMaxLifetime=5min (REQ-010/011/012)
- [ ] `db.Open("")` selects SQLite, `db.Open("postgres://...")` selects PostgreSQL (REQ-001)
- [ ] goqite uses `SQLFlavorPostgreSQL` when running on PostgreSQL
- [ ] PostgreSQL tests skip cleanly when `NOTIFIER_TEST_POSTGRES_DSN` is not set
- [ ] Compile-time check `var _ domain.Store = (*postgres.Store)(nil)` passes
