# Notification Service — Skeleton App Overview

A minimal but fully-featured Go microservice that demonstrates every
layer of the architecture stack through a simple notification workflow.

## What It Does

The service sends one-time email notifications. An API consumer calls
the notify endpoint with an email address, and the service queues an
email for delivery, tracks it through a state machine, and exposes the
results through a REST API and a React dashboard.

### Core Workflow

```
POST /v1/notify { email }
  → Create notification record (pending)
  → Enqueue email job via background queue
  → Worker picks up job, sends via SMTP
  → State machine transitions: pending → sending → delivered/failed/not_sent
  → Dashboard shows result in real time via WebSocket
```

### State Machine

```
pending → sending → delivered
                  → failed     (permanent — @example.com)
                  → not_sent   (soft fail — retryable)

not_sent → sending             (automatic retry via queue)
any terminal → pending         (manual reset via /v1/notify/reset)
```

- **delivered** — email accepted by SMTP server
- **failed** — permanent failure, will not retry (e.g., `@example.com`)
- **not_sent** — transient failure, goqite auto-retries via visibility
  timeout. Each attempt increments a retry count on the notification
  record. After reaching the configured retry limit (default 3), the
  notification transitions to `failed` permanently. The retry count
  and limit are visible in the dashboard and API responses.

### Business Rules

- Each email address can only be notified once. A second attempt
  returns `409 Conflict` (`ErrAlreadyNotified`).
- Emails to `@example.com` automatically fail — simulating an
  undeliverable address without needing a real mail server.
- A reset endpoint clears the notification record, allowing the
  address to be notified again. Returns `404` (`ErrNotFound`) if
  no notification exists for that email.
- Email sending has a 6-second artificial delay to simulate real
  delivery latency. This makes the `pending → sending → delivered`
  state transitions visible in the dashboard in real time.
- Email addresses are validated for format before accepting. Invalid
  addresses return `422 Unprocessable Entity`.
- Every notification gets a prefixed UUID (`ntf_<uuid>`) generated
  at the infra layer.

### Branding and Design

- The React frontend uses Tailwind CSS with light and dark mode support.
- Email templates share the same brand colors and styling as the
  dashboard, demonstrating how to maintain consistent branding across
  web UI and transactional email. The email templates extract the
  brand palette from a shared config and apply it via go-premailer
  CSS inlining.

## Architecture Stack Coverage

Every feature in this app maps to a real-world concern that production
services handle daily. The table below shows what each part of the
skeleton demonstrates and where you would encounter the same pattern
in a real system.

| App Feature | Stack Component | Real-World Application |
|-------------|----------------|----------------------|
| Notify endpoint | REST API (huma) | Any API that accepts requests — order placement, user registration, webhook receivers |
| Email templates | `html/template` + go-premailer | Transactional email — password resets, order confirmations, onboarding sequences |
| SMTP sending | go-mail adapter | Email delivery — notifications, alerts, reports, marketing automation |
| Request ID propagation | UUID in context + email headers | Distributed tracing — correlating logs, API responses, and emails back to the originating request |
| Structured logging | `log/slog` with JSON output | Observability — structured log queries, request correlation, production debugging |
| ID generation | `google/uuid` with prefix (`ntf_`) | Entity identification — prefixed UUIDs for type-safe, human-readable IDs |
| Input validation | Email format check | Data integrity — rejecting bad input at the boundary before it enters the domain |
| Error sentinels | `ErrAlreadyNotified`, `ErrNotFound` | Domain errors — typed errors mapped to HTTP status codes at the handler layer |
| Background queue | goqite | Async processing — payment processing, image resizing, report generation, webhook delivery |
| State machine | stateless | Lifecycle tracking — order fulfillment, approval workflows, incident management, deployment pipelines |
| Paginated list endpoint | Cursor-based pagination | Large datasets — paginating API results, infinite scroll, batch processing, export |
| Duplicate prevention | Domain logic + store | Idempotency — payment deduplication, preventing double-submissions, at-most-once delivery |
| 6-second send delay | Latency simulation | Async UX — showing progress for slow operations, optimistic UI, background processing feedback |
| @example.com auto-fail | Error simulation | Graceful failure handling — unreachable services, invalid recipients, quota exhaustion |
| Reset endpoint | State machine reset | Administrative actions — retry failed operations, re-process records, clear locks |
| Audit log | Transition history table | Compliance — audit trails, change tracking, incident forensics |
| React dashboard | Embedded SPA | Admin panels — monitoring dashboards, back-office tools, internal consoles |
| WebSocket live updates | Real-time push to browser | Live dashboards — order tracking, deployment status, chat, monitoring alerts |
| Tailwind + dark mode | CSS framework with theme switching | Design systems — consistent UI across light/dark themes, accessibility, user preference |
| Storybook | Component development + visual testing | Component libraries — isolated component development, visual regression testing, design system documentation |
| Shared brand styling | Same palette in frontend and email | Brand consistency — matching colors and typography across web, email, and print |
| SPA embed + build tags | go:embed with conditional compilation | Single-binary deployment — CLI tools with web UIs, edge services, appliance software |
| Dev proxy | Reverse proxy to Vite | Development experience — hot reload, fast iteration without rebuilding Go binary |
| Health endpoint | GET /v1/health | Load balancer checks — Kubernetes readiness probes, uptime monitoring, circuit breakers |
| MCP tools (1:1 with REST) | mcp-go | AI agent integration — same domain logic exposed through multiple interfaces (REST for humans/apps, MCP for agents) |
| MCP bridge | stdio-to-HTTP bridge | Tool distribution — making HTTP services available as local CLI tools for AI agents |
| Dual database | SQLite + PostgreSQL | Deployment flexibility — SQLite for dev/edge/single-node, PostgreSQL for production/multi-node |
| Goose migrations | Embedded SQL migrations | Schema management — version-controlled schema changes, zero-downtime deployments |
| XDG config | koanf + XDG paths | Configuration — environment-specific settings, secrets management, 12-factor app compliance |
| Cobra CLI | Subcommands (daemon, mcp-bridge, admin) | Multi-mode binaries — server mode, worker mode, CLI admin mode, migration mode |
| Graceful shutdown | Signal handling + drain | Zero-downtime deployments — connection draining, in-flight request completion, resource cleanup |
| Middleware | Request logging, panic recovery | Cross-cutting concerns — observability, error containment, auth, rate limiting |
| Port interfaces | Domain ports + infra adapters | Testability and flexibility — swap implementations without changing business logic |
| E2E tests | Test harness + Mailpit + QA build | Integration verification — testing the real binary with seeded data, real HTTP, and real SMTP |
| Mise tasks | Namespaced .mise/tasks/ | Build automation — reproducible builds, CI/CD task definitions, developer onboarding |
| Build variants (dev/qa/prod) | Build tags + embedded seed data | Environment-specific builds — demo modes, QA fixtures, feature flags, debug tooling |
| Dockerfile | Multi-stage distroless build | Container deployment — minimal attack surface, reproducible production images |
| OpenSpec specs | Capability-organized specs | Requirements management — living documentation, spec-driven development, change tracking |

## Build Types

Three build variants controlled by build tags:

| Build | Tag | SPA | Seed Data | Use case |
|-------|-----|-----|-----------|----------|
| **dev** | (none) | No (proxy to Vite) | No | Local development with hot reload |
| **qa** | `-tags spa,qa` | Yes (embedded) | Yes (embedded SQL) | Demo, QA, and E2E tests — dashboard has activity from first boot |
| **production** | `-tags spa` | Yes (embedded) | No | Production deployment |

The QA build embeds a SQL seed script that populates the database with
notifications in various states — delivered, failed, not_sent with
retries remaining. The `not_sent` records trigger automatic retries
when the service starts, creating visible activity on the dashboard
immediately. This demonstrates the full lifecycle without manual setup.

The seed data is compiled in via `//go:build qa` so it cannot
accidentally run in production.

## Interfaces

The same domain logic is exposed through two interfaces — REST for
humans and applications, MCP for AI agents. Every REST endpoint has a
corresponding MCP tool. Both call the same service/store layer,
demonstrating hexagonal architecture's interface independence.

### REST API

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/notify` | Send a notification to an email address |
| POST | `/v1/notify/reset` | Reset a notification record for re-sending |
| GET | `/v1/notifications` | List all notifications with state |
| GET | `/v1/health` | Health check (database ping) |
| GET | `/v1/ws` | WebSocket — real-time notification state updates |
| * | `/` | React SPA dashboard (when built with `-tags spa`) |
| * | `/mcp` | MCP streamable HTTP endpoint |

### MCP Tools (1:1 mapping)

| Tool | Equivalent REST | Purpose |
|------|----------------|---------|
| `send_notification` | POST `/v1/notify` | Send a notification |
| `reset_notification` | POST `/v1/notify/reset` | Reset for re-sending |
| `list_notifications` | GET `/v1/notifications` | List all with state |
| `check_health` | GET `/v1/health` | Health check |

## Project Layout

```
cmd/
  daemon/           -- HTTP server
  mcp-bridge/       -- stdio-to-HTTP MCP bridge
  admin/            -- CLI admin commands
internal/
  config/           -- koanf config, XDG paths
  domain/           -- types, ports, errors, state machine config
  infra/
    sqlite/         -- SQLite store + migrations
    postgres/       -- PostgreSQL store + migrations
    httpapi/        -- REST handlers, middleware, SPA handler
    mcp/            -- MCP tool handlers
    email/          -- SMTP adapter (go-mail)
web/                -- React SPA source (Vite + TypeScript)
tests/
  e2e/              -- E2E tests with test harness + Mailpit
openspec/
  specs/            -- capability specifications
main.go
go.mod
mise.toml
.mise/tasks/        -- namespaced build tasks
Dockerfile
```

## Implementation Plan Sequence

The app is built in eight sequential plans, each delivering working,
testable functionality:

| Step | Plan | Est. tasks | Delivers |
|------|------|-----------|----------|
| 1 | Project skeleton | ~10 | Project layout, go.mod, domain types, error sentinels, mise tasks, Dockerfile, QA build tag + seed infrastructure |
| 2 | CLI and database | ~12 | Cobra CLI, koanf config, XDG paths, SQLite store, Goose migrations, health endpoint, structured logging, request ID middleware |
| 3 | Notification delivery | ~14 | Notify endpoint, input validation, email templates with brand styling, goqite queue, SMTP adapter, 6-second delay, QA seed, Mailpit E2E tests |
| 4 | State machine | ~12 | Stateless integration, state transitions, retry count/limit, not_sent soft-fail, audit log, @example.com auto-fail, QA seed for all states |
| 5 | Reset, list, and PostgreSQL | ~10 | Reset endpoint, paginated notifications list, PostgreSQL store + migrations |
| 6 | MCP and WebSocket | ~10 | MCP tools (1:1 with REST), MCP bridge, WebSocket endpoint for live updates |
| 7 | Frontend foundation | ~12 | React + TypeScript + Vite scaffold, Tailwind + dark mode, Storybook setup, reusable components (button, pagination, dark mode switcher), component stories, embed files + dev proxy |
| 8 | Dashboard | ~12 | Notifications table, WebSocket integration, resend button, API client, dashboard Storybook story, SPA routing |

The QA build tag (`//go:build qa`) and seed infrastructure are
established in Step 1. Each subsequent step adds seed data for the
states it introduces, so the QA build always reflects the full
feature set completed so far.

After each step's code review passes, a **skill review checkpoint**
runs before the next step begins. This reviews all skills, agents,
and architecture docs for accuracy against what was actually built.
See [skill-review-checkpoint.md](skill-review-checkpoint.md).
