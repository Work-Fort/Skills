# Service CLI

## Purpose

Provides the command-line interface for the notification service binary. The service exposes multiple subcommands via Cobra, loads configuration from YAML files and environment variables via koanf, and stores all runtime state in XDG-compliant directories.

## Requirements

### Subcommands

- REQ-001: The binary SHALL expose a `daemon` subcommand that starts the HTTP server.
- REQ-002: The binary SHALL expose a `mcp-bridge` subcommand that bridges stdio JSON-RPC to the HTTP MCP endpoint.
- REQ-003: The binary SHALL expose an `admin` subcommand for CLI administration commands.
- REQ-004: The root command SHALL display the binary version, set at build time via `-ldflags "-X main.Version=${VERSION}"`. The default version SHALL be `"dev"`.

### Configuration Loading

- REQ-005: The service SHALL load configuration using `knadh/koanf/v2` with YAML file provider and environment variable provider.
- REQ-006: Configuration sources SHALL be applied in order: YAML config file first, then environment variables. Later sources SHALL override earlier sources.
- REQ-007: Environment variables SHALL use the prefix `NOTIFIER_`, be lowercased after prefix removal, and replace underscores with dots for koanf key mapping.
- REQ-008: A missing config file SHALL NOT cause a startup error. The service SHALL start with defaults and environment variable overrides.
- REQ-009: The koanf instance SHALL be loaded once during startup in the root command's `PersistentPreRunE`. It SHALL NOT be reloaded during request handling.

### XDG Paths

- REQ-010: The config file SHALL be located at `$XDG_CONFIG_HOME/<service>/config.yaml`.
- REQ-011: Runtime state (database files, logs) SHALL be stored under `$XDG_STATE_HOME/<service>/`.
- REQ-012: The service SHALL call an `InitDirs` function in `PersistentPreRunE` to create XDG directories if they do not exist.

### Daemon Flags

- REQ-013: The `daemon` subcommand SHALL accept a `--bind` flag with a default value of `"127.0.0.1"`.
- REQ-014: The `daemon` subcommand SHALL accept a `--port` flag with a default value of `8080`.
- REQ-015: The `daemon` subcommand SHALL accept a `--db` flag for the database DSN, with an empty string default (which selects SQLite).

### Shutdown

- REQ-016: The root `Execute()` function SHALL call `os.Exit(1)` if the root command returns an error.

## Scenarios

### Scenario: Default startup with no config file

- **Given** no config file exists at the XDG config path
- **When** the binary is started with `daemon`
- **Then** the service SHALL start successfully using default values
- **And** the service SHALL bind to `127.0.0.1:8080`

### Scenario: Environment variable overrides config file

- **Given** a config file sets `port: 9090`
- **And** the environment variable `NOTIFIER_PORT=3000` is set
- **When** the binary is started with `daemon`
- **Then** the service SHALL bind to port `3000`

### Scenario: Version flag

- **Given** the binary was built with `-ldflags "-X main.Version=1.2.3"`
- **When** the user runs the binary with `--version`
- **Then** the output SHALL contain `1.2.3`

### Scenario: Unknown subcommand

- **Given** no subcommand matching `foobar` is registered
- **When** the user runs the binary with `foobar`
- **Then** the binary SHALL exit with code 1
