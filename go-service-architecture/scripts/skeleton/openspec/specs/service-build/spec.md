# Service Build

## Purpose

The service build capability defines the build variants, packaging, and development tooling for the notification service. It uses Go build tags to produce dev, QA, and production binaries with different embedded assets and seed data, and relies on mise for task orchestration and dev dependencies.

## Requirements

### Build Variants

- REQ-1: The system SHALL support three build types controlled by build tags: dev (no tags), qa (`-tags spa,qa`), and production (`-tags spa`).
- REQ-2: The QA build SHALL embed a SQL seed script via `//go:build qa` that populates the database with notifications in various states on first boot.
- REQ-3: The QA seed data SHALL include notifications in `delivered`, `failed`, and `not_sent` states, with `not_sent` records triggering automatic retries on startup.
- REQ-4: The seed data mechanism SHALL be compiled in via build tags so it cannot accidentally execute in production builds.

### Development Dependencies

- REQ-5: The system SHALL use Mailpit (`aqua:axllent/mailpit`) as a dev/test SMTP dependency, managed via mise.
- REQ-6: The system SHALL use mise tasks (namespaced) for build orchestration.
- REQ-7: The system SHALL provide a Dockerfile for container builds.

## Scenarios

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

#### Scenario: Mailpit captures test emails
- GIVEN the service is running in dev or QA mode with Mailpit as the SMTP server
- WHEN an email is sent via the notification flow
- THEN Mailpit SHALL capture the email for inspection
- AND E2E tests SHALL verify delivery via the Mailpit API
