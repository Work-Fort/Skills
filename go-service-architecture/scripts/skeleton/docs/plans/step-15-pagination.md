---
type: plan
step: "15"
title: "Pagination Enhancement"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "15"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
  - step-8-dashboard
---

# Step 15: Pagination Enhancement

## Overview

Adds `total_count` and `total_pages` to the paginated list endpoint
response metadata, and updates the frontend `Pagination` component
from simple Previous/Next buttons to numbered page buttons with a
"Page X of Y" indicator. The MCP `list_notifications` tool response
gains the same totals.

After this step:

- The `NotificationStore` interface has a `CountNotifications` method.
- Both SQLite and PostgreSQL stores implement it with
  `SELECT COUNT(*)`.
- The HTTP list handler and MCP list handler include `total_count` and
  `total_pages` in the response `meta` object.
- The frontend `Pagination` component renders numbered page buttons
  with ellipsis for large page counts, highlights the current page,
  and shows "Page X of Y".
- The `useNotifications` hook exposes `currentPage`, `totalPages`,
  `totalCount`, and `goToPage`.
- Pagination controls are hidden when there are 0 or 1 pages.
- Storybook stories cover all pagination variants.

Satisfies notification-management REQ-009 (updated), REQ-019, REQ-020,
REQ-021, REQ-022 and frontend-dashboard REQ-012 (updated), REQ-054,
REQ-055, REQ-056, REQ-057, REQ-058, REQ-059, REQ-060.

## Prerequisites

- Step 5 (Reset, List, Postgres) completed: `ListNotifications` exists
  in both stores, the HTTP list handler works with cursor pagination.
- Step 6 (MCP and WebSocket) completed: `HandleListNotifications` MCP
  tool exists.
- Step 8 (Dashboard) completed: `Pagination` component, `useNotifications`
  hook, `api.ts` client, and `App.tsx` wiring are in place.

## Resolved Ambiguities

1. The COUNT query and list query are not in the same transaction.
   Eventual consistency is acceptable -- the count is informational.
2. Page numbers displayed in the frontend are positional context only,
   not jump-to-offset targets. The cursor stack limits direct access
   to previously visited pages.
3. Ellipsis threshold is 7 total pages. Below 7, all page numbers are
   shown. At 7 or above, ellipsis logic applies.
4. The MCP `list_notifications` tool uses the same meta wrapper as
   the REST endpoint (includes `total_count` and `total_pages`).
5. When `totalPages` is 0 or 1, the `Pagination` component renders
   nothing (no buttons, no nav, no text).

## Tasks

### Task 1: Add `CountNotifications` to domain interface

**Files:**
- Modify: `internal/domain/store.go:11-16`

**Step 1: Add the method to `NotificationStore`**

Add `CountNotifications` to the `NotificationStore` interface:

```go
// NotificationStore persists and retrieves notification records.
type NotificationStore interface {
	CreateNotification(ctx context.Context, n *Notification) error
	GetNotificationByEmail(ctx context.Context, email string) (*Notification, error)
	UpdateNotification(ctx context.Context, n *Notification) error
	ListNotifications(ctx context.Context, after string, limit int) ([]*Notification, error)
	CountNotifications(ctx context.Context) (int, error)
}
```

Satisfies notification-management REQ-019.

**Step 2: Verify the build fails**

Run: `go build ./internal/...`
Expected: FAIL -- `sqlite.Store`, `postgres.Store`, and `db.Store` do
not implement `CountNotifications`. Every stub store in test files will
also fail. This confirms the interface change propagated.

### Task 2: Implement `CountNotifications` in SQLite store

**Files:**
- Modify: `internal/infra/sqlite/store.go:183`
- Test: `internal/infra/sqlite/store_test.go`

**Step 1: Write the failing test**

Append to `internal/infra/sqlite/store_test.go`:

```go
func TestCountNotifications(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Empty database: count should be 0.
	count, err := store.CountNotifications(ctx)
	if err != nil {
		t.Fatalf("CountNotifications() error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	// Insert 3 notifications.
	for i, email := range []string{"cnt1@test.com", "cnt2@test.com", "cnt3@test.com"} {
		n := &domain.Notification{
			ID:         fmt.Sprintf("ntf_cnt-%d", i+1),
			Email:      email,
			Status:     domain.StatusPending,
			RetryLimit: domain.DefaultRetryLimit,
		}
		if err := store.CreateNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	count, err = store.CountNotifications(ctx)
	if err != nil {
		t.Fatalf("CountNotifications() error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}
```

Add `"fmt"` to the import block in `store_test.go` if not already
present.

**Step 2: Run test to verify it fails**

Run: `go test -run TestCountNotifications ./internal/infra/sqlite/...`
Expected: FAIL with "store.CountNotifications undefined"

**Step 3: Write the implementation**

Append to `internal/infra/sqlite/store.go` after the `ListNotifications`
method (after line 183):

```go
// CountNotifications returns the total number of notification records.
// Satisfies notification-management REQ-020.
func (s *Store) CountNotifications(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return count, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestCountNotifications ./internal/infra/sqlite/...`
Expected: PASS

### Task 3: Implement `CountNotifications` in PostgreSQL store

**Files:**
- Modify: `internal/infra/postgres/store.go:171`
- Test: `internal/infra/postgres/store_test.go`

**Step 1: Write the failing test**

Append to `internal/infra/postgres/store_test.go` before the
compile-time check at the bottom:

```go
func TestPostgresCountNotifications(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	// Clean up.
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications")

	// Empty database: count should be 0.
	count, err := store.CountNotifications(ctx)
	if err != nil {
		t.Fatalf("CountNotifications() error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	// Insert 2 notifications.
	for _, email := range []string{"pgcnt1@test.com", "pgcnt2@test.com"} {
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

	count, err = store.CountNotifications(ctx)
	if err != nil {
		t.Fatalf("CountNotifications() error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestPostgresCountNotifications ./internal/infra/postgres/...`
Expected: FAIL with "store.CountNotifications undefined" (or skip if
`NOTIFIER_TEST_POSTGRES_DSN` is not set).

**Step 3: Write the implementation**

Append to `internal/infra/postgres/store.go` after the
`ListNotifications` method (after line 171):

```go
// CountNotifications returns the total number of notification records.
// Satisfies notification-management REQ-020.
func (s *Store) CountNotifications(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return count, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestPostgresCountNotifications ./internal/infra/postgres/...`
Expected: PASS (or skip if DSN not set).

### Task 4: Delegate `CountNotifications` in db wrapper

**Files:**
- Modify: `internal/infra/db/open.go:79-81`

**Step 1: Add the delegation method**

Insert after the `ListNotifications` delegation (after line 81) in
`internal/infra/db/open.go`:

```go
func (s *Store) CountNotifications(ctx context.Context) (int, error) {
	return s.store.CountNotifications(ctx)
}
```

**Step 2: Verify the build compiles**

Run: `go build ./internal/infra/db/...`
Expected: PASS -- the `backendStore` interface embeds `domain.Store`
which embeds `NotificationStore`, so both concrete stores already
satisfy it after Tasks 2 and 3. The `db.Store` wrapper now delegates
`CountNotifications` through.

### Task 5: Update stub stores in test files

**Depends on:** Task 1 (interface change)

**Files:**
- Modify: `internal/infra/httpapi/notify_test.go:47-49`
- Modify: `internal/infra/mcp/tools_test.go:51-57`
- Modify: `internal/infra/mcp/tools_test.go:116-118` (failStore)
- Modify: `internal/infra/queue/worker_test.go:67-69`

Every stub and spy store that implements `NotificationStore` needs a
`CountNotifications` method to compile.

**Step 1: Add `CountNotifications` to httpapi `stubNotificationStore`**

Add after the `ListNotifications` method in
`internal/infra/httpapi/notify_test.go` (after line 49):

```go
func (s *stubNotificationStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
}
```

**Step 2: Add `CountNotifications` to MCP `stubNotificationStore`**

Add after the `ListNotifications` method in
`internal/infra/mcp/tools_test.go` (after line 57):

```go
func (s *stubNotificationStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
}
```

**Step 3: Add `CountNotifications` override to MCP `failStore`**

Add after the `ListNotifications` override in
`internal/infra/mcp/tools_test.go` (after line 118):

```go
func (s *failStore) CountNotifications(_ context.Context) (int, error) {
	return 0, fmt.Errorf("pq: relation \"notifications\" does not exist")
}
```

**Step 4: Add `CountNotifications` to queue `spyStore`**

Add after the `ListNotifications` method in
`internal/infra/queue/worker_test.go` (after line 69):

```go
func (s *spyStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
}
```

**Step 5: Verify all tests pass**

Run: `mise run test:unit`
Expected: PASS -- all packages compile and existing tests pass.

**Step 6: Commit**

`feat(domain): add CountNotifications to NotificationStore interface`

### Task 6: Update HTTP list handler with `total_count` and `total_pages`

**Depends on:** Task 5 (all stubs compile)

**Files:**
- Modify: `internal/infra/httpapi/list.go:28-37`
- Modify: `internal/infra/httpapi/list.go:42`
- Modify: `internal/infra/httpapi/list.go:95-105`
- Modify: `internal/infra/httpapi/list_test.go:35-58`
- Test: `internal/infra/httpapi/list_test.go`

**Step 1: Update `listMeta` struct**

Replace the `listMeta` struct in `internal/infra/httpapi/list.go`
(lines 28-31):

```go
// listMeta is the pagination metadata in the list response.
type listMeta struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	TotalCount int    `json:"total_count"`
	TotalPages int    `json:"total_pages"`
}
```

Satisfies notification-management REQ-009 (updated).

**Step 2: Update `HandleList` to call `CountNotifications`**

In `internal/infra/httpapi/list.go`, the handler currently accepts
`domain.NotificationStore` which now includes `CountNotifications`.
After the `ListNotifications` call (around line 66) and before
building the response metadata (around line 95), add the count query
and compute `total_pages`:

Replace lines 95-105 with:

```go
		// Query total count (outside the list transaction -- eventual
		// consistency accepted per resolved ambiguity #1).
		totalCount, err := store.CountNotifications(r.Context())
		if err != nil {
			slog.Error("count notifications failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// Compute total pages: ceil(totalCount / limit).
		// REQ-022: when totalCount is 0, totalPages is 0.
		totalPages := 0
		if totalCount > 0 {
			totalPages = (totalCount + limit - 1) / limit
		}

		// Build pagination metadata.
		meta := listMeta{
			HasMore:    hasMore,
			TotalCount: totalCount,
			TotalPages: totalPages,
		}
		if hasMore && len(notifications) > 0 {
			lastID := notifications[len(notifications)-1].ID
			meta.NextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
		}

		writeJSON(w, http.StatusOK, listResponse{
			Notifications: items,
			Meta:          meta,
		})
```

Satisfies notification-management REQ-021, REQ-022.

**Step 3: Write updated list handler tests**

Update the `listStubStore` in `internal/infra/httpapi/list_test.go`
to add a `CountNotifications` method (after the `ListNotifications`
override, around line 58):

```go
func (s *listStubStore) CountNotifications(_ context.Context) (int, error) {
	return len(s.notifications), nil
}
```

Add a new test for total count and total pages:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestHandleList ./internal/infra/httpapi/...`
Expected: PASS -- all list handler tests pass including the new
total count tests.

**Step 5: Commit**

`feat(httpapi): add total_count and total_pages to list endpoint meta`

### Task 7: Update MCP `list_notifications` handler

**Depends on:** Task 5 (all stubs compile)

**Files:**
- Modify: `internal/infra/mcp/tools.go:115-156`
- Test: `internal/infra/mcp/tools_test.go`

**Step 1: Update `HandleListNotifications`**

Replace the `HandleListNotifications` function body in
`internal/infra/mcp/tools.go` (lines 115-156):

```go
// HandleListNotifications returns an MCP tool handler for
// list_notifications. It calls the same domain logic as
// GET /v1/notifications (REQ-006).
func HandleListNotifications(store domain.NotificationStore) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		after := req.GetString("after", "")
		limit := req.GetInt("limit", 20)
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		notifications, err := store.ListNotifications(ctx, after, limit)
		if err != nil {
			slog.Error("list notifications failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
		}

		totalCount, err := store.CountNotifications(ctx)
		if err != nil {
			slog.Error("count notifications failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
		}

		// Compute total pages: ceil(totalCount / limit).
		totalPages := 0
		if totalCount > 0 {
			totalPages = (totalCount + limit - 1) / limit
		}

		type item struct {
			ID         string `json:"id"`
			Email      string `json:"email"`
			State      string `json:"state"`
			RetryCount int    `json:"retry_count"`
			RetryLimit int    `json:"retry_limit"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
		}
		items := make([]item, 0, len(notifications))
		for _, n := range notifications {
			items = append(items, item{
				ID:         n.ID,
				Email:      n.Email,
				State:      n.Status.String(),
				RetryCount: n.RetryCount,
				RetryLimit: n.RetryLimit,
				CreatedAt:  n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt:  n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		result, _ := json.Marshal(map[string]any{
			"notifications": items,
			"meta": map[string]any{
				"total_count": totalCount,
				"total_pages": totalPages,
			},
		})
		return gomcp.NewToolResultText(string(result)), nil
	}
}
```

Satisfies notification-management REQ-021 (MCP mirrors REST meta).

**Step 2: Write the test for MCP total count**

Add to `internal/infra/mcp/tools_test.go`:

```go
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
```

Add `"fmt"` to the import block in `tools_test.go` if not already
present.

**Step 3: Run tests to verify they pass**

Run: `go test -run TestListNotificationsTool ./internal/infra/mcp/...`
Expected: PASS

**Step 4: Commit**

`feat(mcp): add total_count and total_pages to list_notifications response`

### Task 8: Update frontend API types

**Files:**
- Modify: `web/src/api.ts:15-18`

**Step 1: Add `total_count` and `total_pages` to `ListMeta`**

Replace the `ListMeta` interface in `web/src/api.ts` (lines 15-18):

```typescript
export interface ListMeta {
  has_more: boolean
  next_cursor?: string
  total_count: number
  total_pages: number
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: PASS -- no type errors.

### Task 9: Update `useNotifications` hook

**Depends on:** Task 8 (API types)

**Files:**
- Modify: `web/src/hooks/useNotifications.ts`

**Step 1: Update the hook interface and implementation**

Replace `UseNotificationsResult` interface (lines 7-25):

```typescript
interface UseNotificationsResult {
  /** The current page of notifications. */
  notifications: Notification[]
  /** True while the initial fetch is in progress. */
  loading: boolean
  /** Error message from the last failed operation, or null. */
  error: string | null
  /** True if there is a next page. */
  hasNext: boolean
  /** True if there is a previous page (not on the first page). */
  hasPrevious: boolean
  /** Load the next page. */
  goNext: () => void
  /** Load the previous page. */
  goPrevious: () => void
  /** Navigate to a specific page (1-indexed). Only pages in the cursor stack are reachable. */
  goToPage: (page: number) => void
  /** Current page number (1-indexed). */
  currentPage: number
  /** Total number of pages. */
  totalPages: number
  /** Total number of notifications across all pages. */
  totalCount: number
  /** Reset a notification to pending so the queue worker re-enqueues it. */
  resend: (id: string) => Promise<void>
  /** Set of notification IDs currently being resent (for loading spinners). */
  resending: Set<string>
}
```

Add state variables after the existing state declarations (after
line 41):

```typescript
  const [totalPages, setTotalPages] = useState(0)
  const [totalCount, setTotalCount] = useState(0)
```

Update `fetchPage` to capture the new meta fields. Replace the
`fetchPage` callback (lines 49-69):

```typescript
  // Fetch a page by cursor.
  const fetchPage = useCallback(async (cursor?: string) => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchNotifications(PAGE_SIZE, cursor)
      const items: Notification[] = data.notifications.map((n) => ({
        id: n.id,
        email: n.email,
        status: n.state,
        retry_count: n.retry_count,
        retry_limit: n.retry_limit,
      }))
      setNotifications(items)
      setHasMore(data.meta.has_more)
      setNextCursor(data.meta.next_cursor)
      setTotalPages(data.meta.total_pages)
      setTotalCount(data.meta.total_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [])
```

Add the `goToPage` callback after `goPrevious` (after line 103):

```typescript
  const goToPage = useCallback(
    (page: number) => {
      if (page < 1 || page > totalPages) return
      // Pages are 1-indexed; cursor stack is 0-indexed.
      const targetIndex = page - 1
      if (targetIndex === pageIndex) return
      if (targetIndex < cursorStack.length) {
        setPageIndex(targetIndex)
        fetchPage(cursorStack[targetIndex])
      }
      // Pages beyond the cursor stack are not directly reachable
      // (positional context only -- resolved ambiguity #2).
    },
    [totalPages, pageIndex, cursorStack, fetchPage],
  )
```

Update the return value (lines 131-141) to include the new fields:

```typescript
  return {
    notifications,
    loading,
    error,
    hasNext: hasMore,
    hasPrevious: pageIndex > 0,
    goNext,
    goPrevious,
    goToPage,
    currentPage: pageIndex + 1,
    totalPages,
    totalCount,
    resend,
    resending,
  }
```

Satisfies frontend-dashboard REQ-060.

**Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

### Task 10: Update `Pagination` component

**Depends on:** Task 9 (hook exposes page data)

**Files:**
- Modify: `web/src/components/Pagination.tsx`
- Modify: `web/src/components/index.ts:7-8`

**Step 1: Rewrite the `Pagination` component**

Replace the entire contents of `web/src/components/Pagination.tsx`:

```tsx
import { Button } from './Button'

export interface PaginationProps {
  /** Called when the user clicks "Previous". */
  onPrevious: () => void
  /** Called when the user clicks "Next". */
  onNext: () => void
  /** Disable the "Previous" button (e.g., on the first page). */
  hasPrevious: boolean
  /** Disable the "Next" button (e.g., on the last page). */
  hasNext: boolean
  /** Current page number (1-indexed). */
  currentPage: number
  /** Total number of pages. */
  totalPages: number
  /** Called when the user clicks a numbered page button. */
  onPageChange: (page: number) => void
}

/**
 * Compute visible page numbers with ellipsis placeholders.
 * Returns an array of page numbers and null for ellipsis gaps.
 * Ellipsis logic applies when totalPages >= 7.
 */
function getPageNumbers(
  currentPage: number,
  totalPages: number,
): (number | null)[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }

  const pages: (number | null)[] = []

  // Always show first page.
  pages.push(1)

  // Left ellipsis: if current page is far enough from the start.
  if (currentPage > 3) {
    pages.push(null)
  }

  // Pages around the current page.
  const start = Math.max(2, currentPage - 1)
  const end = Math.min(totalPages - 1, currentPage + 1)
  for (let i = start; i <= end; i++) {
    pages.push(i)
  }

  // Right ellipsis: if current page is far enough from the end.
  if (currentPage < totalPages - 2) {
    pages.push(null)
  }

  // Always show last page.
  pages.push(totalPages)

  return pages
}

export function Pagination({
  onPrevious,
  onNext,
  hasPrevious,
  hasNext,
  currentPage,
  totalPages,
  onPageChange,
}: PaginationProps) {
  // REQ-058: Hide pagination entirely when 0 or 1 pages.
  if (totalPages <= 1) {
    return null
  }

  const pageNumbers = getPageNumbers(currentPage, totalPages)

  return (
    <nav
      className="flex items-center justify-between border-t border-gray-200 px-4 py-3 dark:border-gray-700"
      aria-label="Pagination"
    >
      <Button
        variant="secondary"
        onClick={onPrevious}
        disabled={!hasPrevious}
      >
        Previous
      </Button>

      <div className="flex items-center gap-1">
        {pageNumbers.map((page, index) =>
          page === null ? (
            <span
              key={`ellipsis-${index}`}
              className="px-2 text-gray-400 dark:text-gray-500"
              aria-hidden="true"
            >
              ...
            </span>
          ) : (
            <Button
              key={page}
              variant={page === currentPage ? 'primary' : 'secondary'}
              onClick={() => onPageChange(page)}
              aria-current={page === currentPage ? 'page' : undefined}
              aria-label={`Page ${page}`}
            >
              {page}
            </Button>
          ),
        )}
      </div>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Page {currentPage} of {totalPages}
      </div>

      <Button
        variant="secondary"
        onClick={onNext}
        disabled={!hasNext}
      >
        Next
      </Button>
    </nav>
  )
}
```

Satisfies frontend-dashboard REQ-054, REQ-055, REQ-056, REQ-057,
REQ-058.

**Step 2: Update component index exports**

The `PaginationProps` export in `web/src/components/index.ts`
(lines 7-8) already exports the type. No change needed -- the
interface name is unchanged.

**Step 3: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

### Task 11: Update Storybook stories for Pagination

**Depends on:** Task 10 (new component props)

**Files:**
- Modify: `web/src/components/Pagination.stories.tsx`

**Step 1: Replace stories with new variants**

Replace the entire contents of
`web/src/components/Pagination.stories.tsx`:

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { Pagination } from './Pagination'

const meta = {
  component: Pagination,
  tags: ['autodocs'],
  args: {
    onPrevious: fn(),
    onNext: fn(),
    onPageChange: fn(),
  },
} satisfies Meta<typeof Pagination>

export default meta
type Story = StoryObj<typeof meta>

export const FewPages: Story = {
  args: {
    currentPage: 3,
    totalPages: 5,
    hasPrevious: true,
    hasNext: true,
  },
}

export const ManyPages: Story = {
  args: {
    currentPage: 5,
    totalPages: 10,
    hasPrevious: true,
    hasNext: true,
  },
}

export const SinglePage: Story = {
  args: {
    currentPage: 1,
    totalPages: 1,
    hasPrevious: false,
    hasNext: false,
  },
}

export const FirstPage: Story = {
  args: {
    currentPage: 1,
    totalPages: 10,
    hasPrevious: false,
    hasNext: true,
  },
}

export const LastPage: Story = {
  args: {
    currentPage: 10,
    totalPages: 10,
    hasPrevious: true,
    hasNext: false,
  },
}
```

Satisfies frontend-dashboard REQ-059.

**Step 2: Verify stories render**

Run: `cd web && npx storybook dev -p 6006` (manual verification)
Expected: Five stories visible -- `FewPages` shows all 5 page buttons,
`ManyPages` shows "1 ... 4 5 6 ... 10", `SinglePage` renders nothing,
`FirstPage` and `LastPage` show correct highlighting.

**Step 3: Commit**

`feat(web): add numbered page buttons and page indicator to Pagination`

### Task 12: Wire updated pagination props in App.tsx

**Depends on:** Task 10 (component), Task 9 (hook)

**Files:**
- Modify: `web/src/App.tsx:9-19`
- Modify: `web/src/App.tsx:96-101`

**Step 1: Destructure new hook values**

Update the `useNotifications()` destructuring in `web/src/App.tsx`
(lines 9-19):

```tsx
  const {
    notifications,
    loading,
    error,
    hasNext,
    hasPrevious,
    goNext,
    goPrevious,
    goToPage,
    currentPage,
    totalPages,
    resend,
    resending,
  } = useNotifications()
```

**Step 2: Update Pagination component usage**

Replace the Pagination usage in `web/src/App.tsx` (lines 96-101):

```tsx
            <Pagination
              hasPrevious={hasPrevious}
              hasNext={hasNext}
              onPrevious={goPrevious}
              onNext={goNext}
              currentPage={currentPage}
              totalPages={totalPages}
              onPageChange={goToPage}
            />
```

Satisfies frontend-dashboard REQ-012 (updated).

**Step 3: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

**Step 4: Verify the build**

Run: `mise run build:web`
Expected: PASS -- Vite build succeeds.

**Step 5: Commit**

`feat(web): wire pagination page numbers into dashboard`

## Verification Checklist

- [ ] `mise run build:go` succeeds with no warnings
- [ ] `mise run test:unit` -- all Go tests pass
- [ ] `mise run lint:go` -- no linter warnings
- [ ] `cd web && npx tsc --noEmit` -- no TypeScript errors
- [ ] `mise run build:web` -- Vite build succeeds
- [ ] HTTP response from `GET /v1/notifications` includes
  `meta.total_count` and `meta.total_pages`
- [ ] MCP `list_notifications` result JSON includes
  `meta.total_count` and `meta.total_pages`
- [ ] Dashboard shows numbered page buttons when there are 2+ pages
- [ ] Dashboard shows "Page X of Y" text
- [ ] Current page button is highlighted with the primary variant
- [ ] Pagination is hidden when there is only 1 page
- [ ] Ellipsis appears when navigating through 7+ pages
- [ ] Storybook stories render correctly: `ManyPages`, `SinglePage`,
  `FirstPage`, `LastPage`, `FewPages`
- [ ] `mise run test:a11y` -- accessibility checks pass for
  pagination stories
