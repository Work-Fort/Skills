# Notifier

A minimal but fully-featured Go microservice that demonstrates every layer of
the architecture stack through a simple notification workflow: accept an email
address, queue a message for delivery, track it through a state machine, and
show the results on a React dashboard in real time via WebSocket. See
[docs/overview.md](docs/overview.md) for the full architecture walkthrough.

## Prerequisites

Install [mise](https://mise.jdx.dev/) and run `mise install` from the project
root. This provisions:

| Tool | Version |
|------|---------|
| Go | 1.26 |
| Node | 22 |
| Tailwind CSS | latest |
| Mailpit | latest |
| golangci-lint | latest |

Do not invoke `go`, `node`, or `npm` directly -- always use `mise run`.

## Quick Start

### Dev (three terminals)

```sh
mise run dev:mailpit          # start Mailpit (SMTP :1025, UI :8025)
```

```sh
mise run dev:web              # start Vite dev server on :5173
```

```sh
mise run release:dev          # build debug binary with race detector
./build/notifier daemon --dev # start server, proxies SPA to Vite
```

Open <http://localhost:8080> for the dashboard, <http://localhost:8025>
for the Mailpit email UI. Mailpit captures all emails sent by the dev
build — no real SMTP server needed.

### QA (single binary, no external deps)

```sh
mise run release:qa
./build/notifier daemon
```

The QA build embeds the SPA, seeds the database with sample notifications in
every state, uses in-memory SQLite, and replaces the SMTP sender with a
`ConsoleSender` that logs to stdout. No Mailpit, no database file, no
configuration needed.

### Production

```sh
VERSION=1.0.0 mise run release:production
./build/notifier daemon
```

The production build embeds the SPA but requires real SMTP configuration and a
database. Set environment variables or provide a config file at
`$XDG_CONFIG_HOME/notifier/config.yaml`.

## Environment Variables

All variables use the `NOTIFIER_` prefix. A matching YAML key in the config
file works too (e.g. `bind: 0.0.0.0`).

| Variable | Default | Description |
|----------|---------|-------------|
| `NOTIFIER_PORT` | `8080` | Listen port |
| `NOTIFIER_BIND` | `127.0.0.1` | Bind address |
| `NOTIFIER_DB` | *(empty)* | Database DSN. Empty = SQLite in `$XDG_STATE_HOME/notifier/`. `postgres://...` for PostgreSQL. QA builds force in-memory SQLite regardless of this value. |
| `NOTIFIER_SMTP_HOST` | `127.0.0.1` | SMTP server host |
| `NOTIFIER_SMTP_PORT` | `1025` | SMTP server port |
| `NOTIFIER_SMTP_FROM` | `notifier@localhost` | Email sender address |
| `NOTIFIER_SHUTDOWN_TIMEOUT` | `60` | Graceful shutdown timeout in seconds |
| `NOTIFIER_WS_ALLOWED_ORIGINS` | `localhost:*,127.0.0.1:*` | Comma-separated WebSocket origin patterns |

## CLI Flags

The `daemon` subcommand accepts the same settings as flags. Flags are
overridden by config file values and environment variables (highest priority).

```
--port              Listen port (default 8080)
--bind              Bind address (default 127.0.0.1)
--db                Database DSN (default: SQLite in XDG state dir)
--smtp-host         SMTP server host (default 127.0.0.1)
--smtp-port         SMTP server port (default 1025)
--smtp-from         Email sender address (default notifier@localhost)
--shutdown-timeout  Shutdown timeout in seconds (default 60)
--dev               Enable dev mode (proxy SPA to Vite dev server)
--dev-url           Vite dev server URL (default http://localhost:5173)
```

## Build Types

| Build | Tags | Embedded SPA | Seed Data | Console Sender | In-Memory DB | Use Case |
|-------|------|:------------:|:---------:|:--------------:|:------------:|----------|
| **dev** | *(none)* | No (proxy to Vite) | No | No | No | Local development with hot reload |
| **qa** | `spa,qa` | Yes | Yes | Yes | Yes | Demo, QA testing, E2E tests |
| **production** | `spa` | Yes | No | No | No | Production deployment |

## QA Simulated Email Domains

In QA builds, the `ConsoleSender` intercepts emails and applies simulated
behaviors based on the recipient domain:

| Domain | Behavior | Error Type |
|--------|----------|------------|
| `@example.com` | Permanent failure | `ErrExampleDomain` -- triggers `failed` state |
| `@fail.com` | Simulated timeout | Transient (wraps `context.DeadlineExceeded`) -- triggers `not_sent` with automatic retry |
| `@slow.com` | 30-second delay then success | None -- replaces the normal 6s delay |
| *all others* | 6-second delay then success | None -- logged to stdout via `ConsoleSender` |

## Mise Tasks

| Task | Description |
|------|-------------|
| `release:dev` | Build debug binary with race detector |
| `release:qa` | Build QA binary with embedded SPA and seed data |
| `release:production` | Build release binary with embedded SPA |
| `build:go` | Build the Go binary (debug, no SPA embed) |
| `build:web` | Build the frontend (`npm run build` in `web/`) |
| `build:email` | Build email templates (Maizzle + Tailwind CSS inlining) |
| `dev:web` | Start the Vite dev server |
| `dev:storybook` | Start the Storybook dev server on port 6006 |
| `test:unit` | Run unit tests |
| `test:e2e` | Run end-to-end tests |
| `test:a11y` | Run Storybook accessibility tests (WCAG 2.1 AA) |
| `lint:go` | Run Go linter (golangci-lint) |
| `lint:web` | Type-check the frontend (tsc) |
| `clean:go` | Remove Go build artifacts |
| `clean:web` | Remove frontend build artifacts |

## Testing

```sh
mise run test:unit       # Go unit tests
mise run test:e2e        # end-to-end tests (builds Go binary, starts server + Mailpit)
mise run test:a11y       # Storybook accessibility tests (WCAG 2.1 AA)
mise run lint:go         # golangci-lint
mise run lint:web        # TypeScript type checking
```

The E2E suite builds a QA binary, starts the server and Mailpit, exercises the
full notify/reset/list workflow over HTTP, and verifies emails arrive in Mailpit.

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/v1/notify` | Send a notification to an email address |
| `POST` | `/v1/notify/reset` | Reset a notification for re-sending |
| `GET` | `/v1/notifications` | List notifications (cursor-paginated) |
| `GET` | `/v1/health` | Health check (database ping) |
| `GET` | `/v1/ws` | WebSocket -- real-time notification state updates |
| `*` | `/mcp/` | MCP streamable HTTP endpoint |
| `*` | `/` | React SPA dashboard (when built with `-tags spa`) |
