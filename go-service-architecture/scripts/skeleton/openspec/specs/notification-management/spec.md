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
- REQ-007: After a successful reset, the email address SHALL be eligible for re-notification via `POST /v1/notify`.
- REQ-007a: On successful reset, the endpoint SHALL return HTTP 204 No Content with an empty response body.

### List Endpoint

- REQ-008: The service SHALL expose `GET /v1/notifications` returning all notifications with their current state.
- REQ-009: The list endpoint SHALL support cursor-based pagination. Query parameters SHALL be `after` (base64-encoded cursor from a previous response) and `limit` (page size). The response body SHALL include a `meta` object with `has_more` (boolean) and `next_cursor` (base64-encoded string, present when `has_more` is true).
- REQ-010: Each notification in the response SHALL include: `id`, `email`, `state`, `retry_count`, `retry_limit`, `created_at`, and `updated_at`.

### Health Endpoint

- REQ-011: The service SHALL expose `GET /v1/health` as a health check.
- REQ-012: The health endpoint SHALL ping the database via the `HealthChecker` port interface.
- REQ-013: When the database is reachable, the endpoint SHALL return HTTP 200 with JSON body `{"status": "healthy"}`.
- REQ-014: When the database is not reachable, the endpoint SHALL return HTTP 503 with JSON body `{"status": "unhealthy"}`.
- REQ-015: The response SHALL set the `Content-Type` header to `application/json`.

### REST Framework

- REQ-016: REST endpoints (except health) SHALL be registered using the `huma` framework via `humago.New` and `huma.Register`.
- REQ-017: The health endpoint SHALL be registered directly on the `http.ServeMux` via `mux.HandleFunc("GET /v1/health", ...)` (not via huma).
- REQ-018: Request body size SHALL be limited to 1 MB via `http.MaxBytesReader`.

## Scenarios

### Scenario: Reset a delivered notification

- **Given** a notification for `user@company.com` exists with state `delivered`
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 204 No Content with an empty body
- **And** the notification state SHALL be `pending`
- **And** the `retry_count` SHALL be 0

### Scenario: Reset a non-existent notification

- **Given** no notification exists for `nobody@company.com`
- **When** a POST request is sent to `/v1/notify/reset` with `{"email": "nobody@company.com"}`
- **Then** the system SHALL return HTTP 404

### Scenario: Re-notify after reset

- **Given** a notification for `user@company.com` was delivered and then reset
- **When** a POST request is sent to `/v1/notify` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 202
- **And** a new email delivery job SHALL be enqueued

### Scenario: List notifications with pagination

- **Given** 25 notifications exist in the database
- **When** a GET request is sent to `/v1/notifications`
- **Then** the system SHALL return HTTP 200
- **And** the response SHALL contain a page of notifications
- **And** each notification SHALL include `id`, `email`, `state`, `retry_count`, and `retry_limit`

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
