# Notification Delivery

## Purpose

Accepts notification requests via a REST endpoint, validates input, enforces one-notification-per-email uniqueness, enqueues email jobs for background delivery via SMTP, and renders branded HTML email templates. This is the primary write path of the notification service.

## Requirements

### Notify Endpoint

- REQ-001: The service SHALL expose `POST /v1/notify` accepting a JSON body with an `email` field.
- REQ-002: On success, the endpoint SHALL return HTTP 202 with the notification ID in the response body.
- REQ-003: The notification ID SHALL be a prefixed UUID in the format `ntf_<uuid>`, generated at the infra layer using `google/uuid`.
- REQ-004: The endpoint SHALL create a notification record in the database with state `pending`.
- REQ-005: The endpoint SHALL enqueue an email delivery job via the goqite background queue. Email sending SHALL NOT happen synchronously in the HTTP handler.

### Input Validation

- REQ-006: The endpoint SHALL validate the email address format using `net/mail.ParseAddress` from the Go standard library before accepting the request.
- REQ-007: An invalid email address SHALL return HTTP 422 Unprocessable Entity.

### Duplicate Prevention

- REQ-008: Each email address SHALL be notified at most once. A second POST for the same email address SHALL return HTTP 409 Conflict.
- REQ-009: The 409 response body SHALL contain the text `"already notified"` (mapped from the domain error `ErrAlreadyNotified`).

### Background Queue

- REQ-010: The service SHALL use `maragu.dev/goqite` for the persistent background job queue. The queue name SHALL be `"notifications"`.
- REQ-011: The goqite queue SHALL share the same `*sql.DB` instance as the notification store.
- REQ-012: The goqite schema SHALL be installed via a goose migration (contents of goqite's `schema_sqlite.sql` or `schema_postgres.sql`).
- REQ-013: The job runner SHALL be configured with `maragu.dev/goqite/jobs` using `jobs.NewRunner` and `jobs.Create` for enqueuing. The runner SHALL use `Limit: 5` (max concurrent jobs) and `PollInterval: 500 * time.Millisecond`.
- REQ-013a: Retry limits SHALL be enforced at the application level, not by goqite's `MaxReceive`. The goqite `MaxReceive` SHALL be set higher than the application retry limit (e.g., 5-10) as a safety net. The job handler SHALL check `retry_count >= retry_limit` and, when the limit is reached, transition the notification to `failed` and return `nil` to acknowledge the message.

### Email Sending

- REQ-014: The SMTP adapter SHALL use `wneessen/go-mail` for email delivery.
- REQ-015: The SMTP adapter SHALL implement the `domain.EmailSender` port interface.
- REQ-016: Email sending SHALL include a 6-second artificial delay to simulate real delivery latency and make state transitions visible in the dashboard.
- REQ-017: Emails to addresses ending in `@example.com` SHALL automatically fail, simulating an undeliverable address without needing a real mail server.
- REQ-018: Failed sends due to transient errors SHALL be automatically retried by goqite's visibility timeout mechanism.

### Email Templates

- REQ-019: Email templates SHALL use `html/template` from the Go standard library with embedded template files via `//go:embed`.
- REQ-020: Email templates SHALL be compiled at build time using Maizzle (MIT, npm), which processes Tailwind CSS and inlines all styles into static HTML. No runtime CSS processing SHALL occur in the Go service.
- REQ-021: Email templates SHALL share brand colors with the dashboard via a `brand.json` file at the project root. The Maizzle build SHALL import `brand.json` to resolve brand color tokens into inlined CSS during compilation. At runtime, the Go service SHALL only inject dynamic values (e.g., recipient name, notification ID) into the pre-compiled HTML via `html/template`. This ensures consistency between the dashboard and transactional emails without runtime CSS processing.
- REQ-022: Every email SHALL include both an HTML body and a plaintext body (via `AddAlternativeString` with `gomail.TypeTextPlain`).

### Request ID in Email

- REQ-023: Every outbound email SHALL include an `X-Request-ID` header containing the request ID from the originating API call.

### Error Mapping

- REQ-024: The domain error `ErrAlreadyNotified` SHALL map to HTTP 409 Conflict at the handler layer.
- REQ-025: The domain error `ErrNotFound` SHALL map to HTTP 404 Not Found at the handler layer.
- REQ-026: Unhandled domain errors SHALL be logged via `slog.Error` and SHALL return HTTP 500 Internal Server Error.

### Port Interfaces

- REQ-027: The `domain.EmailSender` interface SHALL define a `Send(ctx context.Context, msg *EmailMessage) error` method.
- REQ-028: The `EmailMessage` struct SHALL contain fields: `To []string`, `Subject string`, `HTML string`, `Text string`.

## Scenarios

### Scenario: Successful notification

- **Given** the email address `user@company.com` has not been previously notified
- **When** a POST request is sent to `/v1/notify` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 202
- **And** the response body SHALL contain an `id` field matching the pattern `ntf_<uuid>`
- **And** a notification record SHALL exist in the database with state `pending`
- **And** an email delivery job SHALL be enqueued in the goqite queue

### Scenario: Duplicate notification rejected

- **Given** a notification has already been sent to `user@company.com`
- **When** a POST request is sent to `/v1/notify` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 409
- **And** the response body SHALL contain `"already notified"`

### Scenario: Invalid email rejected

- **Given** the email value `"not-an-email"` is not a valid email format
- **When** a POST request is sent to `/v1/notify` with `{"email": "not-an-email"}`
- **Then** the system SHALL return HTTP 422

### Scenario: Email to example.com auto-fails

- **Given** a POST request is sent to `/v1/notify` with `{"email": "test@example.com"}`
- **When** the background worker picks up the email job
- **Then** the email delivery SHALL fail permanently
- **And** the notification state SHALL transition to `failed`

### Scenario: Email includes plaintext alternative

- **Given** a notification is being sent to `user@company.com`
- **When** the email is composed
- **Then** the email SHALL contain an HTML body rendered from a Maizzle-compiled template with pre-inlined CSS, with dynamic values injected via `html/template`
- **And** the email SHALL contain a plaintext alternative body

### Scenario: Request ID propagated to email

- **Given** a POST request is sent to `/v1/notify` generating request ID `abc-123`
- **When** the background worker sends the email
- **Then** the email SHALL contain the header `X-Request-ID: abc-123`
