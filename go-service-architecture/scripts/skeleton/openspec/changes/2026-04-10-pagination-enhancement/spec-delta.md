# Pagination Enhancement -- Spec Delta

## notification-management/spec.md

### Requirements Changed

- REQ-009: The list endpoint SHALL support cursor-based pagination. Query parameters SHALL be `after` (base64-encoded cursor from a previous response) and `limit` (page size). The response body SHALL include a `meta` object with `has_more` (boolean) and `next_cursor` (base64-encoded string, present when `has_more` is true).
+ REQ-009: The list endpoint SHALL support cursor-based pagination. Query parameters SHALL be `after` (base64-encoded cursor from a previous response) and `limit` (page size, default 20, maximum 100). The response body SHALL include a `meta` object with `has_more` (boolean), `next_cursor` (base64-encoded string, present when `has_more` is true), `total_count` (integer, total number of notifications across all pages), and `total_pages` (integer, computed as `ceil(total_count / limit)`).

### Requirements Added

+ REQ-019: The `NotificationStore` interface SHALL include a `CountNotifications(ctx context.Context) (int, error)` method that returns the total number of notification records.
+ REQ-020: Both the SQLite and PostgreSQL store implementations SHALL implement `CountNotifications` using `SELECT COUNT(*) FROM notifications`.
+ REQ-021: The list handler SHALL call `CountNotifications` in addition to `ListNotifications` and include the result as `meta.total_count` in the response. `meta.total_pages` SHALL be computed as `ceil(total_count / limit)`.
+ REQ-022: When `total_count` is 0, `total_pages` SHALL be 0.

### Scenarios Changed

- **Scenario: List notifications with pagination**
-   Given 25 notifications exist in the database
-   When a GET request is sent to `/v1/notifications`
-   Then the system SHALL return HTTP 200
-   And the response SHALL contain a page of notifications
-   And each notification SHALL include `id`, `email`, `state`, `retry_count`, and `retry_limit`
+ **Scenario: List notifications with pagination**
+   Given 25 notifications exist in the database
+   When a GET request is sent to `/v1/notifications` with default limit (20)
+   Then the system SHALL return HTTP 200
+   And the response SHALL contain 20 notifications
+   And each notification SHALL include `id`, `email`, `state`, `retry_count`, and `retry_limit`
+   And `meta.has_more` SHALL be `true`
+   And `meta.total_count` SHALL be `25`
+   And `meta.total_pages` SHALL be `2`

### Scenarios Added

+ **Scenario: Total count reflects all notifications**
+   Given 50 notifications exist in the database
+   When a GET request is sent to `/v1/notifications?limit=10`
+   Then `meta.total_count` SHALL be `50`
+   And `meta.total_pages` SHALL be `5`
+   And `meta.has_more` SHALL be `true`

+ **Scenario: Total pages is zero when no notifications exist**
+   Given 0 notifications exist in the database
+   When a GET request is sent to `/v1/notifications`
+   Then `meta.total_count` SHALL be `0`
+   And `meta.total_pages` SHALL be `0`
+   And `meta.has_more` SHALL be `false`
+   And the `notifications` array SHALL be empty

+ **Scenario: Total pages rounds up for partial last page**
+   Given 21 notifications exist in the database
+   When a GET request is sent to `/v1/notifications?limit=20`
+   Then `meta.total_count` SHALL be `21`
+   And `meta.total_pages` SHALL be `2`

## frontend-dashboard/spec.md

### Requirements Changed

- REQ-012: The dashboard SHALL display a table of all notifications with their current state, email, retry count, and retry limit.
+ REQ-012: The dashboard SHALL display a table of all notifications with their current state, email, retry count, and retry limit. Below the table, the pagination component SHALL display the current page number and total page count.

### Requirements Added

+ REQ-046: The `Pagination` component SHALL accept `currentPage` (number), `totalPages` (number), and `onPageChange` (callback accepting a page number) props in addition to the existing navigation props.
+ REQ-047: The `Pagination` component SHALL display numbered page buttons for direct page access. When the total number of pages exceeds 7, the component SHALL show the first page, last page, current page with its immediate neighbors, and ellipsis indicators for skipped ranges.
+ REQ-048: The `Pagination` component SHALL highlight the current page button with a visually distinct style (e.g., filled background using the `primary` Button variant) while non-current pages use the `secondary` variant.
+ REQ-049: The `Pagination` component SHALL display a "Page X of Y" text label indicating the current page and total pages.
+ REQ-050: When `totalPages` is 0 or 1, the `Pagination` component SHALL NOT render any page buttons or navigation controls.
+ REQ-051: The `Pagination.stories.tsx` file SHALL include stories for: `ManyPages` (e.g., 10+ pages, current page in the middle showing ellipsis), `SinglePage` (1 page, controls hidden), `FirstPage` (current page is 1), `LastPage` (current page is the last), and `FewPages` (e.g., 5 pages, all page numbers visible without ellipsis).
+ REQ-052: The `useNotifications` hook SHALL expose `currentPage` (number, 1-indexed), `totalPages` (number), and `totalCount` (number) derived from the API response `meta.total_count` and `meta.total_pages`. It SHALL also expose a `goToPage(page: number)` callback.

### Scenarios Added

+ **Scenario: Pagination displays page numbers**
+   Given 50 notifications exist and the page size is 20
+   When the dashboard loads
+   Then the pagination component SHALL display page buttons "1", "2", "3"
+   And page 1 SHALL be highlighted as the current page
+   And the text "Page 1 of 3" SHALL be visible

+ **Scenario: Pagination shows ellipsis for many pages**
+   Given 200 notifications exist and the page size is 20
+   When the user navigates to page 5
+   Then the pagination component SHALL display "1 ... 4 5 6 ... 10"
+   And page 5 SHALL be highlighted as the current page

+ **Scenario: Pagination hidden for single page**
+   Given 10 notifications exist and the page size is 20
+   When the dashboard loads
+   Then the pagination component SHALL NOT render page buttons or Previous/Next controls

+ **Scenario: Storybook pagination stories exist**
+   Given `Pagination.stories.tsx` is loaded in Storybook
+   When the stories list is displayed
+   Then stories SHALL exist for: `ManyPages`, `SinglePage`, `FirstPage`, `LastPage`, and `FewPages`
