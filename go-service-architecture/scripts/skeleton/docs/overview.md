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
  → State machine transitions: pending → sending → delivered/failed
  → Dashboard shows result in real time
```

### Business Rules

- Each email address can only be notified once. A second attempt
  returns `409 Conflict`.
- Emails to `@example.com` automatically fail — simulating an
  undeliverable address without needing a real mail server.
- A reset endpoint clears the notification record, allowing the
  address to be notified again.
- Email sending has a 6-second artificial delay to simulate real
  delivery latency. This makes the `pending → sending → delivered`
  state transitions visible in the dashboard in real time.

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
| Background queue | goqite | Async processing — payment processing, image resizing, report generation, webhook delivery |
| State machine | stateless | Lifecycle tracking — order fulfillment, approval workflows, incident management, deployment pipelines |
| Duplicate prevention | Domain logic + store | Idempotency — payment deduplication, preventing double-submissions, at-most-once delivery |
| 6-second send delay | Latency simulation | Async UX — showing progress for slow operations, optimistic UI, background processing feedback |
| @example.com auto-fail | Error simulation | Graceful failure handling — unreachable services, invalid recipients, quota exhaustion |
| Reset endpoint | State machine reset | Administrative actions — retry failed operations, re-process records, clear locks |
| Audit log | Transition history table | Compliance — audit trails, change tracking, incident forensics |
| React dashboard | Embedded SPA | Admin panels — monitoring dashboards, back-office tools, internal consoles |
| WebSocket live updates | Real-time push to browser | Live dashboards — order tracking, deployment status, chat, monitoring alerts |
| Tailwind + dark mode | CSS framework with theme switching | Design systems — consistent UI across light/dark themes, accessibility, user preference |
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
| E2E tests | Test harness + Mailpit | Integration verification — testing the real binary with real HTTP and real SMTP |
| Mise tasks | Namespaced .mise/tasks/ | Build automation — reproducible builds, CI/CD task definitions, developer onboarding |
| Dockerfile | Multi-stage distroless build | Container deployment — minimal attack surface, reproducible production images |
| OpenSpec specs | Capability-organized specs | Requirements management — living documentation, spec-driven development, change tracking |

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

The app is built in five sequential plans, each delivering working,
testable functionality:

| Step | Plan | Delivers |
|------|------|----------|
| 1 | Foundation | Project layout, domain types, CLI, config, SQLite store, health endpoint, mise tasks, Dockerfile |
| 2 | Notification delivery | Notify endpoint, email templates, goqite queue, SMTP adapter, Mailpit E2E tests |
| 3 | State machine | Stateless integration, transitions, audit log, @example.com auto-fail |
| 4 | Reset and admin | Reset endpoint, notifications list, MCP tools, MCP bridge |
| 5 | Frontend | React SPA, Vite setup, embed, dev proxy, dashboard UI |
