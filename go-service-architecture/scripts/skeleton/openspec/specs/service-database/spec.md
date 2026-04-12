# Service Database

## Purpose

Provides dual-database support for the notification service. SQLite is used for development, edge, and single-node deployments. PostgreSQL is used for production and multi-node deployments. Both backends implement the same domain port interfaces and use Goose for embedded SQL migrations.

## Requirements

### Backend Selection

- REQ-001: The service SHALL select the database backend based on the DSN. A DSN starting with `"postgres"` SHALL select PostgreSQL. All other values (including empty string) SHALL select SQLite.
- REQ-002: When no DSN is configured (empty string), the service SHALL default to SQLite and store the database file in the XDG state directory at `$XDG_STATE_HOME/<service>/`.

### SQLite Configuration

- REQ-003: The SQLite store SHALL use `modernc.org/sqlite` (CGO-free, pure Go).
- REQ-004: The SQLite store SHALL set `PRAGMA journal_mode=WAL` on connection open.
- REQ-005: The SQLite store SHALL set `PRAGMA foreign_keys=ON` on connection open.
- REQ-006: The SQLite store SHALL set `PRAGMA busy_timeout=5000` on connection open.
- REQ-007: The SQLite store SHALL set `db.SetMaxOpenConns(1)` for single-writer serialization.
- REQ-008: In-memory SQLite SHALL be used for unit tests by passing an empty string DSN to `sqlite.Open("")`.

### PostgreSQL Configuration

- REQ-009: The PostgreSQL store SHALL use `pgx/v5` as the connection driver.
- REQ-010: The PostgreSQL store SHALL set `db.SetMaxOpenConns(25)`.
- REQ-011: The PostgreSQL store SHALL set `db.SetMaxIdleConns(5)`.
- REQ-012: The PostgreSQL store SHALL set `db.SetConnMaxLifetime(5 * time.Minute)`.

### Migrations

- REQ-013: Both database backends SHALL use `pressly/goose` for schema migrations.
- REQ-014: Migration SQL files SHALL be embedded in the binary via `//go:embed migrations/*.sql`.
- REQ-015: Migrations SHALL be numbered sequentially: `001_init.sql`, `002_feature.sql`, etc.
- REQ-016: Each migration file SHALL contain `-- +goose Up` and `-- +goose Down` markers.
- REQ-017: Migrations SHALL run automatically on database open (via `goose.Up`).
- REQ-018: SQL queries SHALL use parameterized placeholders (`?` for SQLite, `$1` for PostgreSQL). String concatenation for query parameters SHALL NOT be used.

### Shared Connection

- REQ-019: The `*sql.DB` instance SHALL be shared between the store and the goqite background queue. They SHALL NOT open separate database connections.

### Port Interfaces

- REQ-020: Both SQLite and PostgreSQL stores SHALL implement the same `domain.Store` composite interface, which combines `NotificationStore`, `HealthChecker`, and `io.Closer`.

## Scenarios

### Scenario: SQLite selected by default

- **Given** no DSN is configured
- **When** the store is opened
- **Then** the system SHALL create a SQLite database in the XDG state directory
- **And** WAL mode SHALL be enabled

### Scenario: PostgreSQL selected by DSN prefix

- **Given** the DSN is `"postgres://user:pass@localhost/notifydb"`
- **When** the store is opened
- **Then** the system SHALL connect using the PostgreSQL driver

### Scenario: Migrations run on startup

- **Given** a fresh database with no tables
- **When** the store is opened
- **Then** the system SHALL run all embedded goose migrations
- **And** all required tables SHALL exist after open returns

### Scenario: In-memory SQLite for tests

- **Given** an empty string DSN
- **When** `sqlite.Open("")` is called
- **Then** the system SHALL create an in-memory SQLite database
- **And** migrations SHALL run successfully against it
