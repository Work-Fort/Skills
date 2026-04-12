# Service Infrastructure

## Purpose

The service infrastructure capability provides the data persistence layer and lifecycle management for the notification service. It supports dual database backends (SQLite and PostgreSQL), automatic schema migrations via Goose, and a graceful shutdown sequence that drains connections and releases resources in the correct order.

## Requirements

### Database

- REQ-1: The system SHALL default to SQLite when no DSN is configured, storing the database file in the XDG state directory.
- REQ-2: The system SHALL use PostgreSQL when the DSN starts with `postgres`.
- REQ-3: The system SHALL use `modernc.org/sqlite` (CGO-free) for SQLite and `jackc/pgx/v5` for PostgreSQL.
- REQ-4: The system SHALL run Goose migrations automatically on startup for both database backends.
- REQ-5: The system SHALL configure SQLite with WAL mode, foreign keys enabled, and a busy timeout of 5000ms.
- REQ-6: The system SHALL share the same `*sql.DB` instance between the store and the goqite background queue.
- REQ-7: The system SHALL configure PostgreSQL connection pooling with appropriate pool size, max idle connections, and connection lifetime settings.

### Graceful Shutdown

- REQ-8: The system SHALL listen for SIGINT and SIGTERM signals to initiate graceful shutdown.
- REQ-9: The system SHALL stop accepting new HTTP connections on shutdown signal.
- REQ-10: The system SHALL allow in-flight requests up to 15 seconds to complete before forcing shutdown.
- REQ-11: The system SHALL stop the goqite job runner before closing the database connection.
- REQ-12: The system SHALL close the database connection as the final shutdown step.

### Project Structure

- REQ-13: The system SHALL follow hexagonal architecture with `internal/domain/` for core types and port interfaces, and `internal/infra/` for all infrastructure implementations.
- REQ-14: The domain package SHALL have zero imports from the infra package.

## Scenarios

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

#### Scenario: Graceful shutdown
- GIVEN the service is running with active connections and a background job runner
- WHEN a SIGTERM signal is received
- THEN the system SHALL stop accepting new connections
- AND the system SHALL wait up to 15 seconds for in-flight requests
- AND the system SHALL stop the job runner
- AND the system SHALL close the database connection last
