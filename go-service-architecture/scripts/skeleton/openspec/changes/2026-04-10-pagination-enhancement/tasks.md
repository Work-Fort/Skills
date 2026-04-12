# Pagination Enhancement -- Tasks

## Task Breakdown

1. Add `CountNotifications` to domain interface
   - Files: `internal/domain/store.go`
   - Verification: Code compiles. All existing store implementations fail to compile until updated (confirms interface change propagated).

2. Implement `CountNotifications` in SQLite store
   - Files: `internal/infra/sqlite/store.go`, `internal/infra/sqlite/store_test.go`
   - Verification: `go test -run "TestCountNotifications" ./internal/infra/sqlite/...`

3. Implement `CountNotifications` in PostgreSQL store
   - Files: `internal/infra/postgres/store.go`, `internal/infra/postgres/store_test.go`
   - Verification: `go test -run "TestPostgresCountNotifications" ./internal/infra/postgres/...`

4. Delegate `CountNotifications` in db wrapper
   - Files: `internal/infra/db/open.go`
   - Verification: Code compiles. `go vet ./internal/infra/db/...`

5. Update HTTP list handler with `total_count` and `total_pages`
   - Files: `internal/infra/httpapi/list.go`, `internal/infra/httpapi/list_test.go`
   - Verification: `go test -run "TestHandleList" ./internal/infra/httpapi/...` -- response JSON contains `meta.total_count` and `meta.total_pages`.

6. Update MCP `list_notifications` handler with `total_count` and `total_pages`
   - Files: `internal/infra/mcp/tools.go`, `internal/infra/mcp/tools_test.go`
   - Verification: `go test -run "TestListNotificationsTool" ./internal/infra/mcp/...` -- response JSON contains `total_count` and `total_pages`.

7. Update stub stores in test files
   - Files: `internal/infra/httpapi/notify_test.go`, `internal/infra/httpapi/reset_test.go`, `internal/infra/mcp/tools_test.go`, `internal/infra/queue/worker_test.go`
   - Verification: `go test ./internal/...` -- all tests pass with updated stub stores implementing `CountNotifications`.

8. Update frontend API types and client
   - Files: `web/src/api.ts`
   - Verification: TypeScript compiles. `ListMeta` interface includes `total_count` and `total_pages`.

9. Update `useNotifications` hook with page totals
   - Files: `web/src/hooks/useNotifications.ts`
   - Verification: Hook exposes `currentPage`, `totalPages`, `totalCount`. `npm run typecheck` passes.

10. Update `Pagination` component with numbered page buttons
    - Files: `web/src/components/Pagination.tsx`, `web/src/components/Pagination.stories.tsx`
    - Verification: `npm run storybook` -- Pagination stories show numbered buttons, "Page X of Y", current page highlight. `npm run test:a11y` passes.

11. Wire updated pagination props in App.tsx
    - Files: `web/src/App.tsx`
    - Verification: Dashboard shows numbered pagination with page context. Manual QA in browser.
