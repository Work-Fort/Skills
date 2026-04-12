# Service Observability

## Purpose

Provides structured logging, request ID propagation, and HTTP middleware for the notification service. Every request is traceable from the API response through logs and into email headers via a unique request ID.

## Requirements

### Structured Logging

- REQ-001: The service SHALL use `log/slog` from the Go standard library for all logging.
- REQ-002: Log output SHALL use JSON format via `slog.NewJSONHandler`.
- REQ-003: Log entries for HTTP requests SHALL include the fields: `method`, `path`, `status`, and `duration`.

### Request ID

- REQ-004: The service SHALL generate a UUID for each incoming HTTP request using `google/uuid`.
- REQ-005: The request ID SHALL be stored in the request context and accessible to all downstream handlers and services.
- REQ-006: The request ID SHALL be included in the `X-Request-ID` response header. It SHALL NOT be included in the response body.
- REQ-007: The request ID SHALL be propagated into outbound emails via the `X-Request-ID` email header.

### Middleware Stack

- REQ-008: The HTTP server SHALL apply middleware in this order (outermost first): request logging, then panic recovery.
- REQ-009: The request logging middleware SHALL log every HTTP request with method, path, response status code, and duration.
- REQ-010: The request logging middleware SHALL use a `statusRecorder` wrapper to capture the response status code, and SHALL provide an `Unwrap()` method returning the underlying `http.ResponseWriter`.
- REQ-011: The panic recovery middleware SHALL catch panics in downstream handlers, log the error via `slog.Error`, and return HTTP 500 if no response has been written yet.

### Server Timeouts

- REQ-012: The HTTP server SHALL set `ReadTimeout` to 15 seconds.
- REQ-013: The HTTP server SHALL set `WriteTimeout` to 15 seconds.
- REQ-014: The HTTP server SHALL set `IdleTimeout` to 60 seconds.
- REQ-015: The HTTP server SHALL set `ReadHeaderTimeout` to 5 seconds.

## Scenarios

### Scenario: Request ID flows from API to email header

- **Given** a POST request is sent to `/v1/notify` with a valid email address
- **When** the notification email is sent via SMTP
- **Then** the email SHALL contain an `X-Request-ID` header matching the request ID from the API response

### Scenario: Panic in handler does not crash the server

- **Given** a handler raises a panic
- **When** the panic recovery middleware catches it
- **Then** the system SHALL log the panic via `slog.Error`
- **And** the system SHALL return HTTP 500 to the client
- **And** the server SHALL continue accepting new requests

### Scenario: Request logging records status and duration

- **Given** a GET request is sent to `/v1/health`
- **When** the response is returned
- **Then** the system SHALL emit a structured log entry with `"method":"GET"`, `"path":"/v1/health"`, `"status":200`, and a `"duration"` value
