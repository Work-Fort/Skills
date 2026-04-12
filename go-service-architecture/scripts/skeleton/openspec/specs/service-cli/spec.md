# Service CLI

## Purpose

The service CLI capability defines the command-line interface and configuration loading for the notification service. It uses cobra for subcommand routing and koanf for hierarchical configuration from YAML files and environment variables, following XDG conventions for file placement.

## Requirements

- REQ-1: The system SHALL use `spf13/cobra` for CLI command structure with subcommands: `daemon`, `mcp-bridge`, and `admin`.
- REQ-2: The system SHALL use `knadh/koanf` for configuration, loading from a YAML config file and environment variables (env vars override file values).
- REQ-3: The system SHALL use XDG-compliant paths for configuration (`$XDG_CONFIG_HOME/<service>/config.yaml`) and state (`$XDG_STATE_HOME/<service>/`).
- REQ-4: The system SHALL support configuration of: bind address, port, database DSN, and SMTP settings (host, port, username, password, from address).
- REQ-5: The system SHALL set the version string via `-ldflags` at build time, with a default of "dev".

## Scenarios

#### Scenario: Environment variable override
- GIVEN a config file sets the port to 8080
- AND the environment variable for port is set to 9090
- WHEN the daemon subcommand starts
- THEN the service SHALL listen on port 9090
