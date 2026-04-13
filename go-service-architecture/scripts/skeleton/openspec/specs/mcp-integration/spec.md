# MCP Integration

## Purpose

Exposes the notification service's domain logic through the Model Context Protocol (MCP) for AI agent integration. Every REST endpoint has a corresponding MCP tool, both backed by the same service/store layer. A bridge subcommand enables stdio-based MCP access to the HTTP endpoint.

## Requirements

### MCP Server

- REQ-001: The service SHALL expose an MCP endpoint at `/mcp` using `mcp-go`'s `StreamableHTTPServer`.
- REQ-002: The MCP server SHALL be created via `server.NewMCPServer("<service-name>", Version)`.
- REQ-003: When mounting on the main HTTP mux, the handler SHALL use `http.StripPrefix("/mcp", mcpHandler)` if the mux pattern includes a trailing slash.

### MCP Tools (1:1 with REST)

- REQ-004: The MCP tool `send_notification` SHALL be registered and call the same service/store logic as `POST /v1/notify`.
- REQ-005: The MCP tool `reset_notification` SHALL be registered and call the same service/store logic as `POST /v1/notify/reset`.
- REQ-006: The MCP tool `list_notifications` SHALL be registered and call the same service/store logic as `GET /v1/notifications`.
- REQ-007: The MCP tool `check_health` SHALL be registered and call the same service/store logic as `GET /v1/health`.
- REQ-008: Each MCP tool SHALL include a description via `mcp.WithDescription(...)`.
- REQ-009: MCP tool handlers SHALL accept domain port interfaces (e.g., `domain.NotificationStore`), not concrete store implementations.

### MCP Error Handling

- REQ-014: MCP tool handlers SHALL NOT expose raw `err.Error()` strings to clients. When an internal error occurs (database failure, enqueue failure, state machine error, update failure), the tool SHALL return a generic error message (e.g., `"internal error"`) via `gomcp.NewToolResultError`.
- REQ-015: MCP tool handlers SHALL log the real error server-side via `slog.Error` with structured context (operation name, relevant IDs) before returning the generic error to the client.
- REQ-016: Domain-specific errors that are safe for clients (e.g., `"email is required"`, `"already notified"`, `"not found"`, validation errors from `domain.ValidateEmail`) SHALL continue to be returned to the client as-is.

### MCP Reset Guard

- REQ-017: The `reset_notification` MCP tool SHALL reject reset requests for notifications in `not_sent` state when `retry_count < retry_limit` (auto-retry still in progress). The tool SHALL return a `gomcp.NewToolResultError` with the message `"notification has retries remaining"`. This mirrors the HTTP 409 Conflict behavior of `POST /v1/notify/reset`.
- REQ-018: The `reset_notification` MCP tool SHALL allow resets for `not_sent` notifications when `retry_count >= retry_limit`, and SHALL always allow resets for `failed` and `delivered` notifications.
- REQ-019: After a successful reset, the `reset_notification` MCP tool SHALL enqueue a new delivery job via the background queue so the worker picks up the notification and re-attempts delivery. This mirrors the REST `POST /v1/notify/reset` behavior (notification-management REQ-007).

### MCP Bridge

- REQ-010: The `mcp-bridge` subcommand SHALL read JSON-RPC messages from stdin and forward them to the HTTP MCP endpoint at `http://<host>:<port>/mcp`.
- REQ-011: The bridge SHALL relay responses from the HTTP endpoint back to stdout.
- REQ-012: The bridge SHALL pass an auth token on every forwarded request.

### Graceful Shutdown

- REQ-013: When the server shuts down, the MCP `StreamableHTTPServer` SHALL be shut down separately via `mcpHandler.Shutdown(shutdownCtx)` to signal SSE clients to reconnect.

## Scenarios

### Scenario: Send notification via MCP

- **Given** no notification exists for `user@company.com`
- **When** the `send_notification` MCP tool is called with `{"email": "user@company.com"}`
- **Then** a notification record SHALL be created with state `pending`
- **And** an email delivery job SHALL be enqueued
- **And** the tool response SHALL contain the notification ID

### Scenario: MCP and REST share domain logic

- **Given** a notification was sent via `POST /v1/notify` for `user@company.com`
- **When** the `list_notifications` MCP tool is called
- **Then** the response SHALL include the notification created via REST

### Scenario: MCP bridge forwards requests

- **Given** the service is running on `http://127.0.0.1:8080`
- **And** the `mcp-bridge` subcommand is started
- **When** a JSON-RPC request for `send_notification` is written to stdin
- **Then** the bridge SHALL forward the request to `http://127.0.0.1:8080/mcp`
- **And** the bridge SHALL write the JSON-RPC response to stdout

### Scenario: MCP tool returns generic error on internal failure

- **Given** the database is unreachable
- **When** the `send_notification` MCP tool is called with `{"email": "user@company.com"}`
- **Then** the tool response SHALL be an error containing `"internal error"`
- **And** the tool response SHALL NOT contain database driver details, connection strings, or file paths
- **And** the real error SHALL be logged server-side via `slog.Error`

### Scenario: MCP tool returns safe domain errors as-is

- **Given** a notification has already been sent to `user@company.com`
- **When** the `send_notification` MCP tool is called with `{"email": "user@company.com"}`
- **Then** the tool response SHALL be an error containing `"already notified"`

### Scenario: MCP reset rejected for not_sent with retries remaining

- **Given** a notification for `user@company.com` exists with state `not_sent`, `retry_count` of 1, and `retry_limit` of 3
- **When** the `reset_notification` MCP tool is called with `{"email": "user@company.com"}`
- **Then** the tool response SHALL be an error containing `"notification has retries remaining"`
- **And** the notification state SHALL remain `not_sent`

### Scenario: MCP reset allowed for not_sent with retries exhausted

- **Given** a notification for `user@company.com` exists with state `not_sent`, `retry_count` of 3, and `retry_limit` of 3
- **When** the `reset_notification` MCP tool is called with `{"email": "user@company.com"}`
- **Then** the notification state SHALL be `pending`
- **And** the `retry_count` SHALL be 0
- **And** a delivery job SHALL be enqueued via the background queue

### Scenario: MCP duplicate prevention matches REST

- **Given** a notification was sent via the `send_notification` MCP tool for `user@company.com`
- **When** a POST request is sent to `/v1/notify` with `{"email": "user@company.com"}`
- **Then** the system SHALL return HTTP 409 Conflict
