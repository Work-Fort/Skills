# Notification Delivery

## Purpose

The notification delivery capability accepts an email address via a REST endpoint and sends an email notification to that address. Email sending is asynchronous -- the HTTP handler enqueues a job via goqite, and a background worker performs the actual SMTP delivery using go-mail. Each email address can only be notified once; duplicate requests are rejected.

## Requirements

- REQ-1: The system SHALL expose a POST `/v1/notify` endpoint that accepts a JSON body containing an `email` field.
- REQ-2: The system SHALL validate that the `email` field is present, non-empty, and a valid email format, returning HTTP 422 if validation fails.
- REQ-3: The system SHALL reject duplicate notification requests for the same email address, returning HTTP 409 with an "already notified" error message.
- REQ-4: The system SHALL enqueue email delivery as a background job via goqite rather than sending synchronously in the HTTP handler.
- REQ-5: The system SHALL return HTTP 202 (Accepted) immediately after successfully enqueuing the notification job.
- REQ-6: The background worker SHALL send the email via SMTP using the `wneessen/go-mail` library.
- REQ-7: The system SHALL render an HTML email template with inline CSS using `vanng822/go-premailer`.
- REQ-8: The system SHALL include a plaintext alternative body alongside the HTML body in every sent email.
- REQ-9: The email templates SHALL be embedded in the Go binary via `//go:embed`.
- REQ-10: The system SHALL automatically fail delivery for email addresses ending in `@example.com` without attempting SMTP delivery.
- REQ-11: The system SHALL persist notification records in the database, storing at minimum the email address, delivery status, and timestamps.
- REQ-12: The system SHALL use the domain `EmailSender` port interface for SMTP sending, keeping infrastructure concerns out of the domain layer.
- REQ-13: The system SHALL generate a prefixed UUID for each notification ID in the format `ntf_<uuid>`, generated at the infra layer.
- REQ-14: The system SHALL generate a request ID (UUID) for each incoming HTTP request, propagate it through context, and embed it in the outgoing email as an `X-Request-ID` header.
- REQ-15: The background worker SHALL introduce a 6-second artificial delay before sending each email to simulate real delivery latency and make state transitions visible in the dashboard.
- REQ-16: The system SHALL use `log/slog` for structured logging throughout the delivery pipeline, including the request ID in all log entries.

## Scenarios

#### Scenario: Successful notification request
- GIVEN the system is running and the email "user@real.com" has not been previously notified
- WHEN a POST request is made to `/v1/notify` with body `{"email": "user@real.com"}`
- THEN the system SHALL return HTTP 202
- AND a notification record SHALL be persisted in the database
- AND a background job SHALL be enqueued for email delivery

#### Scenario: Background worker delivers email
- GIVEN a notification job for "user@real.com" is in the goqite queue
- WHEN the background worker picks up the job
- THEN the worker SHALL send an email to "user@real.com" via SMTP
- AND the email SHALL contain both HTML and plaintext bodies
- AND the HTML body SHALL have inline CSS (no `<style>` blocks)

#### Scenario: Duplicate notification rejected
- GIVEN the email "user@real.com" has already been notified
- WHEN a POST request is made to `/v1/notify` with body `{"email": "user@real.com"}`
- THEN the system SHALL return HTTP 409
- AND the response body SHALL contain an "already notified" error message

#### Scenario: Missing email field
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify` with an empty body or missing `email` field
- THEN the system SHALL return HTTP 422

#### Scenario: Invalid email format rejected
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify` with body `{"email": "not-an-email"}`
- THEN the system SHALL return HTTP 422
- AND the response body SHALL indicate an invalid email format

#### Scenario: Example.com email auto-fails
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify` with body `{"email": "test@example.com"}`
- THEN the system SHALL return HTTP 202
- AND the background worker SHALL mark the delivery as failed without attempting SMTP
- AND the notification record SHALL reflect the failed status

#### Scenario: Notification ID has prefixed UUID format
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify` with body `{"email": "user@real.com"}`
- THEN the created notification record SHALL have an ID matching the pattern `ntf_<uuid>`

#### Scenario: Request ID propagated to email headers
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify` with body `{"email": "user@real.com"}`
- AND the background worker delivers the email
- THEN the sent email SHALL contain an `X-Request-ID` header matching the request's UUID

#### Scenario: Delivery includes artificial delay
- GIVEN a notification job for "user@real.com" is in the goqite queue
- WHEN the background worker picks up the job
- THEN the worker SHALL wait 6 seconds before attempting SMTP delivery
- AND the notification SHALL remain in `sending` state during the delay
