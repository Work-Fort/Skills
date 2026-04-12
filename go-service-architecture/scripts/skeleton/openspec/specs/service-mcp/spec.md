# Service MCP

## Purpose

The service MCP capability exposes the notification service's functionality through the Model Context Protocol. It provides MCP tools that maintain 1:1 parity with REST endpoints, a StreamableHTTPServer mount for in-process access, and a stdio bridge subcommand for external MCP client integration.

## Requirements

### MCP Server

- REQ-1: The system SHALL expose an MCP server endpoint at `/mcp` using `mark3labs/mcp-go` with `StreamableHTTPServer`.

### MCP Tools

- REQ-2: The system SHALL provide an MCP tool `list_notifications` that returns all notification records.
- REQ-3: The system SHALL provide an MCP tool `send_notification` that accepts an email address and triggers the notification flow.
- REQ-4: The system SHALL provide an MCP tool `reset_notification` that accepts an email address and triggers the reset flow.
- REQ-5: The system SHALL provide an MCP tool `check_health` that returns the service health status.
- REQ-6: The MCP tools SHALL maintain 1:1 parity with REST endpoints: `send_notification` (POST `/v1/notify`), `reset_notification` (POST `/v1/notify/reset`), `list_notifications` (GET `/v1/notifications`), and `check_health` (GET `/v1/health`).

### MCP Bridge

- REQ-7: The system SHALL include a `cmd/mcp-bridge` subcommand that bridges stdio JSON-RPC to the HTTP MCP endpoint.

## Scenarios

#### Scenario: MCP list notifications
- GIVEN the database contains notification records
- WHEN the `list_notifications` MCP tool is invoked
- THEN it SHALL return all notification records with email, result, and state

#### Scenario: MCP send notification
- GIVEN the system is running
- WHEN the `send_notification` MCP tool is invoked with email "user@real.com"
- THEN it SHALL trigger the same notification flow as POST `/v1/notify`

#### Scenario: MCP reset notification
- GIVEN a notification for "user@real.com" exists in `delivered` state
- WHEN the `reset_notification` MCP tool is invoked with email "user@real.com"
- THEN it SHALL trigger the same reset flow as POST `/v1/notify/reset`
- AND the notification state SHALL be `pending`

#### Scenario: MCP tools have 1:1 REST parity
- GIVEN the service is running with all endpoints active
- WHEN the MCP tool list is queried
- THEN it SHALL contain exactly four tools: `send_notification`, `reset_notification`, `list_notifications`, and `check_health`
- AND each tool SHALL produce the same domain result as its corresponding REST endpoint

#### Scenario: MCP bridge stdio
- GIVEN the daemon is running on port 8080
- WHEN the `mcp-bridge` subcommand is started
- THEN it SHALL read JSON-RPC messages from stdin
- AND forward them to `http://127.0.0.1:8080/mcp`
- AND write responses to stdout
