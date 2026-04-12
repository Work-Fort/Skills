# Pagination Enhancement -- Design

## Approach

Add a `CountNotifications(ctx context.Context) (int, error)` method to `domain.NotificationStore`. Both SQLite and PostgreSQL stores implement it with `SELECT COUNT(*) FROM notifications`. The HTTP list handler and MCP list handler call this method alongside the existing `ListNotifications` and include `total_count` and `total_pages` (computed as `ceil(total_count / limit)`) in the response metadata. The frontend `Pagination` component is extended with numbered page buttons and a "Page X of Y" label, consuming the new metadata fields from the API response.

## Components Affected

- `internal/domain/store.go` -- Add `CountNotifications(ctx context.Context) (int, error)` to the `NotificationStore` interface.
- `internal/infra/sqlite/store.go` -- Implement `CountNotifications` with `SELECT COUNT(*) FROM notifications`.
- `internal/infra/postgres/store.go` -- Implement `CountNotifications` with `SELECT COUNT(*) FROM notifications`.
- `internal/infra/db/open.go` -- Delegate `CountNotifications` to the inner store.
- `internal/infra/httpapi/list.go` -- Add `total_count` and `total_pages` to `listMeta` struct; call `CountNotifications` in handler; compute `total_pages`.
- `internal/infra/mcp/tools.go` -- Add `total_count` and `total_pages` to `HandleListNotifications` response JSON.
- `web/src/api.ts` -- Add `total_count` and `total_pages` to `ListMeta` interface.
- `web/src/hooks/useNotifications.ts` -- Expose `currentPage`, `totalPages`, `totalCount`, and `goToPage` callback.
- `web/src/components/Pagination.tsx` -- Add numbered page buttons, current page highlight, "Page X of Y" text.
- `web/src/components/Pagination.stories.tsx` -- Add story variants for many pages, single page, and current page highlighting.

## Risks

- **COUNT(*) performance on large tables.** For SQLite with WAL mode and PostgreSQL with small-to-medium tables (< 100k rows), `COUNT(*)` is fast. For very large PostgreSQL tables, an approximate count or caching strategy may be needed in the future. Acceptable for the skeleton app's scale.
- **Race between count and list.** The count query and the list query are not in the same transaction, so the count could be stale by the time the list returns. This is acceptable for a dashboard -- the count is informational, not transactional.
- **Cursor-based pagination with page numbers.** The API still uses cursors for navigation. Page numbers displayed in the frontend are derived from `total_pages`, but clicking page 5 does not directly jump to page 5 by offset -- the frontend must still navigate sequentially via cursors. This limits direct page access to the cursor stack (pages already visited). The frontend can display the numbers for positional context even if not all pages are directly clickable.

## Alternatives Considered

- **Offset-based pagination.** Would make direct page jumps trivial (`?page=5&limit=20` maps to `OFFSET 80`), but offset pagination has known issues: skipped or duplicated rows when data changes between pages, and poor database performance at high offsets. Cursor-based pagination avoids these issues.
- **Approximate count via `pg_class.reltuples`.** Faster for large PostgreSQL tables but inaccurate after bulk inserts/deletes. Premature optimization for the skeleton app.
- **Separate `/v1/notifications/count` endpoint.** Adds an extra round trip. Including the count in the existing response is simpler for the frontend.
