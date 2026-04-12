# Service Observability

## Purpose

The service observability capability provides structured logging and request tracing for the notification service. It generates a unique request ID for every incoming HTTP request, propagates it through the context for consistent log correlation, and enforces JSON-formatted structured output via `log/slog`.

## Requirements

### Structured Logging

- REQ-1: The system SHALL use `log/slog` for structured logging throughout, with JSON-formatted output in all environments.

### Request ID Middleware

- REQ-2: The system SHALL include a request ID middleware that generates a UUID for each incoming HTTP request and stores it in the request context.
- REQ-3: The request ID SHALL be included in all structured log entries for the duration of the request.
- REQ-4: The request ID SHALL be returned in the HTTP response via a header.

### WebSocket

- REQ-5: The system SHALL use `coder/websocket` (ISC-licensed) as the WebSocket library for real-time browser communication.

## Scenarios

#### Scenario: Request ID propagated through context
- GIVEN the service is running
- WHEN a POST request is made to `/v1/notify`
- THEN the middleware SHALL generate a UUID request ID
- AND the request ID SHALL appear in all slog entries for that request
- AND the request ID SHALL be returned in the HTTP response headers

#### Scenario: Structured logging in JSON format
- GIVEN the service is running
- WHEN any request is handled
- THEN all log output SHALL be structured JSON via `log/slog`
- AND each log entry SHALL include the request ID, timestamp, level, and message
