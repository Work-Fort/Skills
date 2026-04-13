# Notification Management

## Purpose

Provides the reset endpoint for re-sending notifications, the paginated list endpoint for viewing all notifications, and the health check endpoint. These are the read and administrative paths of the notification service.

## Requirements

### Reset Endpoint

- REQ-001: The service SHALL expose `POST /v1/notify/reset` accepting a JSON body with an `email` field.
- REQ-002: The reset endpoint SHALL transition the notification for the given email address back to `pending` state via the state machine.
- REQ-003: If no notification exists for the given email address, the endpoint SHALL return HTTP 404 Not Found (mapped from `ErrNotFound`).
- REQ-004: On successful reset, the notification record SHALL have its `retry_count` cleared to 0.
- REQ-005: On successful reset, the notification record SHALL have its delivery results cleared.
- REQ-006: On successful reset, the notification record SHALL have its timestamp fields (other than `created_at`) reset.
- REQ-007: After a successful reset, the endpoint SHALL enqueue a new delivery job via the background queue so the worker picks up the notification and re-attempts delivery. The notification SHALL NOT remain in `pending` state without a corresponding queued job.
- REQ-007a: On successful reset, the endpoint SHALL return HTTP 204 No Content with an empty response body.

### List Endpoint

- REQ-008: The service SHALL expose `GET /v1/notifications` returning all notifications with their current state.
- REQ-009: The list endpoint SHALL support cursor-based pagination. Query parameters SHALL be `after` (base64-encoded cursor from a previous response) and `limit` (page size, default 20, maximum 100). The response body SHALL include a `meta` object with `has_more` (boolean), `next_cursor` (base64-encoded string, present when `has_more` is true), `total_count` (integer, total number of notifications across all pages), and `total_pages` (integer, computed as `ceil(total_count / limit)`).
- REQ-010: Each notification in the response SHALL include: `id`, `email`, `state`, `retry_count`, `retry_limit`, `created_at`, and `updated_at`.

### Health Endpoint

- REQ-011: The service SHALL expose `GET /v1/health` as a health check.
- REQ-012: The health endpoint SHALL ping the database via the `HealthChecker` port interface.
- REQ-013: When the database is reachable, the endpoint SHALL return HTTP 200 with JSON body `{"status": "healthy"}`.
- REQ-014: When the database is not reachable, the endpoint SHALL return HTTP 503 with JSON body `{"status": "unhealthy"}`.
- REQ-015: The response SHALL set the `Content-Type` header to `application/json`.

### Count Query

- REQ-019: The `NotificationStore` interface SHALL include a `CountNotifications(ctx context.Context) (int, error)` method that returns the total number of notification records.
- REQ-020: Both the SQLite and PostgreSQL store implementations SHALL implement `CountNotifications` using `SELECT COUNT(*) FROM notifications`.
- REQ-021: The list handler SHALL call `CountNotifications` in addition to `ListNotifications` and include the result as `meta.total_count` in the response. `meta.total_pages` SHALL be computed as `ceil(total_count / limit)`.
- REQ-022: When `total_count` is 0, `total_pages` SHALL be 0.

### Reset Guard for In-Progress Retries

- REQ-023: When the notification is in `not_sent` state and `retry_count < retry_limit` (auto-retry is still in progress), the reset endpoint SHALL return HTTP 409 Conflict with a JSON error body containing the message `"notification has retries remaining"`. The notification state SHALL NOT change.
- REQ-024: When the notification is in `not_sent` state and `retry_count >= retry_limit` (retries exhausted), the reset endpoint SHALL allow the reset (same behavior as `failed` or `delivered`).
- REQ-025: For notifications in `failed` or `delivered` state, the reset endpoint SHALL allow the reset regardless of `retry_count`.

### Test Coverage

- REQ-026: The health endpoint SHALL have a dedicated E2E test that starts the service binary, sends `GET /v1/health`, and verifies HTTP 200 with `{"status": "healthy"}` and `Content-Type: application/json`. This test SHALL NOT rely on the health check being a side-effect assertion inside another test.
- REQ-027: The reset guard (REQ-023) SHALL have an E2E test that: (1) sends a notification to a transient-failure address (e.g., `test@fail.com` in QA builds) so it reaches `not_sent` with `retry_count < retry_limit`, (2) attempts `POST /v1/notify/reset` for that address, and (3) verifies HTTP 409 with `"notification has retries remaining"`.
- REQ-028: The pagination endpoint SHALL have E2E tests covering edge cases: (a) `limit` values above 100 SHALL be clamped to 100, (b) `total_pages` SHALL be correctly calculated as `ceil(total_count / limit)`, (c) an empty notification list SHALL return `total_count: 0`, `total_pages: 0`, `has_more: false`, and an empty `notifications` array.
- REQ-029: The reset guard SHALL have an integration test using real SQLite storage (not mocks) that: (1) creates a notification with state `not_sent` and `retry_count < retry_limit`, (2) invokes the reset guard check, and (3) verifies the guard returns `ErrRetriesRemaining`. The test SHALL also verify the converse: when `retry_count >= retry_limit`, the guard SHALL allow the reset.

### REST Framework

- REQ-016: REST endpoints (except health) SHALL be registered using the `huma` framework via `humago.New` and `huma.Register`.
- REQ-017: The health endpoint SHALL be registered directly on the `http.ServeMux` via `mux.HandleFunc("GET /v1/health", ...)` (not via huma).
- REQ-018: The `POST /v1/notify/reset` endpoint SHALL limit the request body size to 1 MB via `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` applied before reading the body. Requests exceeding this limit SHALL result in a `400 Bad Request` response.

## Scenarios

### Scenario: Reset a delivered notification

- **Given** a notification for `user@company.com` exists with state `delivered`
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 204 No Content with an empty body
- **And** the notification state SHALL be `pending`
- **And** the `retry_count` SHALL be 0
- **And** a delivery job SHALL be enqueued via the background queue

### Scenario: Reset rejected for not_sent notification with retries remaining

- **Given** a notification for `user@company.com` exists with state `not_sent`, `retry_count` of 1, and `retry_limit` of 3
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 409 Conflict
- **And** the response body SHALL contain `"notification has retries remaining"`
- **And** the notification state SHALL remain `not_sent`
- **And** the `retry_count` SHALL remain 1

### Scenario: Reset allowed for not_sent notification with retries exhausted

- **Given** a notification for `user@company.com` exists with state `not_sent`, `retry_count` of 3, and `retry_limit` of 3
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 204 No Content with an empty body
- **And** the notification state SHALL be `pending`
- **And** the `retry_count` SHALL be 0
- **And** a delivery job SHALL be enqueued via the background queue

### Scenario: Reset a non-existent notification

- **Given** no notification exists for `nobody@company.com`
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "nobody@company.com"}`
- **Then** the system SHALL return HTTP 404

### Scenario: Re-notify after reset

- **Given** a notification for `user@company.com` was delivered and then reset (which enqueued a delivery job)
- **When** the background worker processes the enqueued job
- **Then** the notification SHALL transition through the delivery state machine (pending -> sending -> delivered/not_sent)
- **And** a separate `POST /v1/notify` for the same email SHALL return HTTP 409 because the notification already exists

### Scenario: List notifications with pagination

- **Given** 25 notifications exist in the database
- **When** a GET request is sent to `/v1/notifications` with default limit (20)
- **Then** the system SHALL return HTTP 200
- **And** the response SHALL contain 20 notifications
- **And** each notification SHALL include `id`, `email`, `state`, `retry_count`, and `retry_limit`
- **And** `meta.has_more` SHALL be `true`
- **And** `meta.total_count` SHALL be `25`
- **And** `meta.total_pages` SHALL be `2`

### Scenario: Total count reflects all notifications

- **Given** 50 notifications exist in the database
- **When** a GET request is sent to `/v1/notifications?limit=10`
- **Then** `meta.total_count` SHALL be `50`
- **And** `meta.total_pages` SHALL be `5`
- **And** `meta.has_more` SHALL be `true`

### Scenario: Total pages is zero when no notifications exist

- **Given** 0 notifications exist in the database
- **When** a GET request is sent to `/v1/notifications`
- **Then** `meta.total_count` SHALL be `0`
- **And** `meta.total_pages` SHALL be `0`
- **And** `meta.has_more` SHALL be `false`
- **And** the `notifications` array SHALL be empty

### Scenario: Total pages rounds up for partial last page

- **Given** 21 notifications exist in the database
- **When** a GET request is sent to `/v1/notifications?limit=20`
- **Then** `meta.total_count` SHALL be `21`
- **And** `meta.total_pages` SHALL be `2`

### Scenario: Health check with healthy database

- **Given** the database is reachable
- **When** a GET request is sent to `/v1/health`
- **Then** the system SHALL return HTTP 200
- **And** the response body SHALL be `{"status": "healthy"}`
- **And** the `Content-Type` header SHALL be `application/json`

### Scenario: Health check with unreachable database

- **Given** the database connection has failed
- **When** a GET request is sent to `/v1/health`
- **Then** the system SHALL return HTTP 503
- **And** the response body SHALL be `{"status": "unhealthy"}`

### Scenario: Oversized reset request body rejected

- **Given** a POST request body larger than 1 MB is prepared
- **When** the request is sent to `POST /v1/notify/reset`
- **Then** the system SHALL return HTTP 400 Bad Request
- **And** no notification state change SHALL occur

### Scenario: E2E health endpoint returns healthy

- **Given** the service is running with a reachable database
- **When** a GET request is sent to `/v1/health`
- **Then** the system SHALL return HTTP 200
- **And** the response body SHALL be `{"status": "healthy"}`
- **And** the `Content-Type` header SHALL be `application/json`

### Scenario: E2E reset guard rejects not_sent with retries remaining

- **Given** the service is running
- **And** a notification exists with state `not_sent` and `retry_count` of 1 and `retry_limit` of 3 (retries still in progress)
- **When** a POST request is sent to `/v1/notify/reset` for that notification's email
- **Then** the system SHALL return HTTP 409 Conflict
- **And** the response body SHALL contain `"notification has retries remaining"`

### Scenario: E2E pagination with limit clamping

- **Given** the service is running with at least 1 notification in the database
- **When** a GET request is sent to `/v1/notifications?limit=200`
- **Then** the system SHALL clamp the limit to 100
- **And** the response SHALL contain at most 100 notifications

### Scenario: E2E pagination with empty list

- **Given** the service is running with 0 notifications in the database
- **When** a GET request is sent to `/v1/notifications`
- **Then** `meta.total_count` SHALL be `0`
- **And** `meta.total_pages` SHALL be `0`
- **And** `meta.has_more` SHALL be `false`
- **And** the `notifications` array SHALL be empty

### Scenario: Integration test — retry guard with real database

- **Given** a real SQLite database (in-memory) with the notification schema
- **And** a notification record exists with state `not_sent`, `retry_count` of 1, and `retry_limit` of 3
- **When** the reset guard function (`CheckResetAllowed` or equivalent) is called with this notification
- **Then** the guard SHALL return an error indicating retries are remaining
- **And** when the same check is performed on a notification with `retry_count` of 3 and `retry_limit` of 3
- **Then** the guard SHALL return nil (reset allowed)
