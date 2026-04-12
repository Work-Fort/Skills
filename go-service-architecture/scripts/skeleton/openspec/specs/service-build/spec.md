# Service Build

## Purpose

Defines the three build variants (dev, qa, production), mise task automation, Dockerfile configuration, and QA seed data infrastructure. Build tags control which assets and seed data are compiled into the binary.

## Requirements

### Build Variants

- REQ-001: The service SHALL support three build variants: dev, qa, and production.
- REQ-002: The **dev** build SHALL be compiled with no build tags. It SHALL NOT embed the SPA and SHALL NOT include seed data. It uses a reverse proxy to the Vite dev server.
- REQ-003: The **qa** build SHALL be compiled with `-tags spa,qa`. It SHALL embed the SPA and SHALL include seed data.
- REQ-004: The **production** build SHALL be compiled with `-tags spa`. It SHALL embed the SPA and SHALL NOT include seed data.

### QA Seed Data

- REQ-005: The QA seed infrastructure SHALL be established in the project skeleton (implementation step 1). Each subsequent step SHALL add seed data for the states it introduces.
- REQ-006: QA seed data SHALL be guarded by `//go:build qa` so it cannot execute in production builds.
- REQ-007: The QA seed SQL script SHALL be embedded in the binary via `//go:embed` and executed on startup.
- REQ-008: The QA seed data SHALL populate notifications in the following states: delivered, failed, and not_sent with retries remaining.
- REQ-009: The `not_sent` seed records SHALL trigger automatic retries when the service starts, creating visible dashboard activity immediately.

### Mise Tasks

- REQ-010: Build tasks SHALL be implemented as executable bash scripts in `.mise/tasks/`, with subdirectories creating colon-separated namespaces.
- REQ-011: The `mise run build:go` task SHALL build the Go binary without SPA embedding.
- REQ-012: The `mise run build:web` task SHALL build the frontend by running `npm run build` in the `web/` directory.
- REQ-013: The `mise run release:dev` task SHALL build a debug binary with the `-race` flag enabled.
- REQ-014: The `mise run release:production` task SHALL depend on `build:web`, compile with `-tags spa`, set `CGO_ENABLED=0`, apply `-ldflags="-s -w -X main.Version=${VERSION}"`, and use `-trimpath`.
- REQ-015: The `mise run test:unit` task SHALL run `go test ./...`.
- REQ-016: The `mise run test:e2e` task SHALL depend on `build:go` and run `go test -v -count=1 ./...` in the `tests/e2e/` directory.
- REQ-017: The `mise run dev:web` task SHALL start the Vite dev server by running `npm run dev` in the `web/` directory.
- REQ-018: The `mise run dev:storybook` task SHALL start the Storybook dev server on port 6006.
- REQ-019: All mise task scripts SHALL include `set -euo pipefail` and a `#MISE description="..."` directive.
- REQ-020: The `mise.toml` at the repo root SHALL pin tool versions (e.g., `go = "1.26.0"`) and SHALL NOT contain inline task definitions.

### Dockerfile

- REQ-021: The Dockerfile SHALL use a multi-stage build with `golang:1.26-alpine` as the build stage and `gcr.io/distroless/static-debian12` as the runtime stage.
- REQ-022: The build stage SHALL compile with `CGO_ENABLED=0`, `-ldflags="-s -w"`, and `-trimpath`.
- REQ-023: The runtime stage SHALL run as `USER nonroot:nonroot`.
- REQ-024: The default entrypoint SHALL be the service binary and the default CMD SHALL be `["daemon"]`.

## Scenarios

### Scenario: QA build includes seed data

- **Given** the binary is built with `-tags spa,qa`
- **When** the service starts with a fresh database
- **Then** the database SHALL contain seed notifications in delivered, failed, and not_sent states
- **And** not_sent records SHALL begin automatic retry processing

### Scenario: Production build excludes seed data

- **Given** the binary is built with `-tags spa`
- **When** the service starts with a fresh database
- **Then** the database SHALL contain no seed data
- **And** only migration-created schema SHALL exist

### Scenario: Dev build proxies to Vite

- **Given** the binary is built with no build tags
- **When** the service starts with `--dev --dev-url http://localhost:5173`
- **Then** requests to `/` SHALL be proxied to the Vite dev server at `http://localhost:5173`

### Scenario: Release production task builds full binary

- **Given** the frontend has been built in `web/dist/`
- **When** `mise run release:production` is executed with `VERSION=1.0.0`
- **Then** the output binary SHALL embed the SPA assets
- **And** the binary version SHALL report `1.0.0`
- **And** `CGO_ENABLED` SHALL be `0`
