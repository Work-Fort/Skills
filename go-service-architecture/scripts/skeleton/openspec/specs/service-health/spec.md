# Health and Infrastructure

## Purpose

The health and infrastructure capability provides the operational foundation for the notification service. It includes a health check endpoint, MCP tool integration for programmatic access, CLI scaffolding with cobra and koanf, dual database support (SQLite and PostgreSQL), and graceful shutdown. This capability ensures the service is observable, configurable, and production-ready.

## Requirements

### Health Check

- REQ-1: The system SHALL expose a GET `/v1/health` endpoint.
- REQ-2: The health endpoint SHALL check database connectivity by calling the `Ping` method on the store.
- REQ-3: The health endpoint SHALL return HTTP 200 with `{"status": "healthy"}` when the database is reachable.
- REQ-4: The health endpoint SHALL return HTTP 503 with `{"status": "unhealthy"}` when the database is unreachable.

### MCP Integration

- REQ-5: The system SHALL expose an MCP server endpoint at `/mcp` using `mark3labs/mcp-go` with `StreamableHTTPServer`.
- REQ-6: The system SHALL provide an MCP tool `list_notifications` that returns all notification records.
- REQ-7: The system SHALL provide an MCP tool `send_notification` that accepts an email address and triggers the notification flow.
- REQ-7a: The system SHALL provide an MCP tool `reset_notification` that accepts an email address and triggers the reset flow.
- REQ-7b: The system SHALL provide an MCP tool `check_health` that returns the service health status.
- REQ-8: The system SHALL include a `cmd/mcp-bridge` subcommand that bridges stdio JSON-RPC to the HTTP MCP endpoint.

### CLI and Configuration

- REQ-9: The system SHALL use `spf13/cobra` for CLI command structure with subcommands: `daemon`, `mcp-bridge`.
- REQ-10: The system SHALL use `knadh/koanf` for configuration, loading from a YAML config file and environment variables (env vars override file values).
- REQ-11: The system SHALL use XDG-compliant paths for configuration (`$XDG_CONFIG_HOME/<service>/config.yaml`) and state (`$XDG_STATE_HOME/<service>/`).
- REQ-12: The system SHALL support configuration of: bind address, port, database DSN, and SMTP settings (host, port, username, password, from address).
- REQ-13: The system SHALL set the version string via `-ldflags` at build time, with a default of "dev".

### Database

- REQ-14: The system SHALL default to SQLite when no DSN is configured, storing the database file in the XDG state directory.
- REQ-15: The system SHALL use PostgreSQL when the DSN starts with `postgres`.
- REQ-16: The system SHALL use `modernc.org/sqlite` (CGO-free) for SQLite, `jackc/pgx/v5` for PostgreSQL, and `coder/websocket` (ISC-licensed) for WebSocket support.
- REQ-17: The system SHALL run Goose migrations automatically on startup for both database backends.
- REQ-18: The system SHALL configure SQLite with WAL mode, foreign keys enabled, and a busy timeout of 5000ms.
- REQ-19: The system SHALL share the same `*sql.DB` instance between the store and the goqite background queue.

### Graceful Shutdown

- REQ-20: The system SHALL listen for SIGINT and SIGTERM signals to initiate graceful shutdown.
- REQ-21: The system SHALL stop accepting new HTTP connections on shutdown signal.
- REQ-22: The system SHALL allow in-flight requests up to 15 seconds to complete before forcing shutdown.
- REQ-23: The system SHALL stop the goqite job runner before closing the database connection.
- REQ-24: The system SHALL close the database connection as the final shutdown step.

### Project Structure

- REQ-25: The system SHALL follow hexagonal architecture with `internal/domain/` for core types and port interfaces, and `internal/infra/` for all infrastructure implementations.
- REQ-26: The domain package SHALL have zero imports from the infra package.
- REQ-27: The system SHALL use `log/slog` for structured logging throughout, with JSON-formatted output in all environments.

### Request ID Middleware

- REQ-28: The system SHALL include a request ID middleware that generates a UUID for each incoming HTTP request and stores it in the request context.
- REQ-29: The request ID SHALL be included in all structured log entries for the duration of the request.
- REQ-30: The request ID SHALL be returned in the HTTP response via a header.

### Build Variants

- REQ-31: The system SHALL support three build types controlled by build tags: dev (no tags), qa (`-tags spa,qa`), and production (`-tags spa`).
- REQ-32: The QA build SHALL embed a SQL seed script via `//go:build qa` that populates the database with notifications in various states on first boot.
- REQ-33: The QA seed data SHALL include notifications in `delivered`, `failed`, and `not_sent` states, with `not_sent` records triggering automatic retries on startup.
- REQ-34: The seed data mechanism SHALL be compiled in via build tags so it cannot accidentally execute in production builds.

### WebSocket

- REQ-35: The system SHALL use `coder/websocket` (ISC-licensed) as the WebSocket library for real-time browser communication.

### Dependencies

- REQ-36: The system SHALL use Mailpit (`aqua:axllent/mailpit`) as a dev/test SMTP dependency, managed via mise.
- REQ-37: The system SHALL configure PostgreSQL connection pooling with appropriate pool size, max idle connections, and connection lifetime settings.

### MCP Tool Parity

- REQ-38: The MCP tools SHALL maintain 1:1 parity with REST endpoints: `send_notification` (POST `/v1/notify`), `reset_notification` (POST `/v1/notify/reset`), `list_notifications` (GET `/v1/notifications`), and `check_health` (GET `/v1/health`).

## Scenarios

#### Scenario: Healthy database
- GIVEN the service is running and the database is accessible
- WHEN a GET request is made to `/v1/health`
- THEN the system SHALL return HTTP 200
- AND the response body SHALL be `{"status": "healthy"}`

#### Scenario: Unhealthy database
- GIVEN the service is running but the database is unreachable
- WHEN a GET request is made to `/v1/health`
- THEN the system SHALL return HTTP 503
- AND the response body SHALL be `{"status": "unhealthy"}`

#### Scenario: MCP list notifications
- GIVEN the database contains notification records
- WHEN the `list_notifications` MCP tool is invoked
- THEN it SHALL return all notification records with email, result, and state

#### Scenario: MCP send notification
- GIVEN the system is running
- WHEN the `send_notification` MCP tool is invoked with email "user@real.com"
- THEN it SHALL trigger the same notification flow as POST `/v1/notify`

#### Scenario: SQLite default startup
- GIVEN no DSN is configured
- WHEN the daemon subcommand starts
- THEN the system SHALL create a SQLite database in the XDG state directory
- AND Goose migrations SHALL run automatically
- AND the database SHALL be configured with WAL mode

#### Scenario: PostgreSQL startup
- GIVEN the DSN is configured as "postgres://user:pass@localhost/notifications"
- WHEN the daemon subcommand starts
- THEN the system SHALL connect to PostgreSQL
- AND Goose migrations SHALL run automatically

#### Scenario: Environment variable override
- GIVEN a config file sets the port to 8080
- AND the environment variable for port is set to 9090
- WHEN the daemon subcommand starts
- THEN the service SHALL listen on port 9090

#### Scenario: Graceful shutdown
- GIVEN the service is running with active connections and a background job runner
- WHEN a SIGTERM signal is received
- THEN the system SHALL stop accepting new connections
- AND the system SHALL wait up to 15 seconds for in-flight requests
- AND the system SHALL stop the job runner
- AND the system SHALL close the database connection last

#### Scenario: MCP bridge stdio
- GIVEN the daemon is running on port 8080
- WHEN the `mcp-bridge` subcommand is started
- THEN it SHALL read JSON-RPC messages from stdin
- AND forward them to `http://127.0.0.1:8080/mcp`
- AND write responses to stdout

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

#### Scenario: QA build boots with seed data
- GIVEN the binary is built with `-tags spa,qa`
- WHEN the daemon subcommand starts for the first time
- THEN the embedded seed SQL SHALL populate the database with sample notifications
- AND the seed data SHALL include notifications in `delivered`, `failed`, and `not_sent` states
- AND `not_sent` notifications SHALL trigger automatic retry activity on the dashboard

#### Scenario: Production build excludes seed data
- GIVEN the binary is built with `-tags spa` only (no `qa` tag)
- WHEN the daemon subcommand starts
- THEN no seed data SHALL be loaded
- AND the seed SQL SHALL not be compiled into the binary

#### Scenario: MCP tools have 1:1 REST parity
- GIVEN the service is running with all endpoints active
- WHEN the MCP tool list is queried
- THEN it SHALL contain exactly four tools: `send_notification`, `reset_notification`, `list_notifications`, and `check_health`
- AND each tool SHALL produce the same domain result as its corresponding REST endpoint

#### Scenario: MCP reset notification
- GIVEN a notification for "user@real.com" exists in `delivered` state
- WHEN the `reset_notification` MCP tool is invoked with email "user@real.com"
- THEN it SHALL trigger the same reset flow as POST `/v1/notify/reset`
- AND the notification state SHALL be `pending`

#### Scenario: Mailpit captures test emails
- GIVEN the service is running in dev or QA mode with Mailpit as the SMTP server
- WHEN an email is sent via the notification flow
- THEN Mailpit SHALL capture the email for inspection
- AND E2E tests SHALL verify delivery via the Mailpit API
