# Frontend Dashboard

## Purpose

Provides a React SPA dashboard embedded in the Go binary for monitoring notification state in real time. The frontend uses Tailwind CSS with dark mode support, communicates via WebSocket for live updates, and ships with Storybook for isolated component development.

## Requirements

### SPA Embed

- REQ-001: The frontend SHALL be a React + TypeScript application built with Vite, with source in `web/` and build output in `web/dist/`.
- REQ-002: The SPA SHALL be embedded into the Go binary using build tags. Two embed files SHALL exist in `cmd/web/` (or equivalent): `embed.go` (no tag, empty `embed.FS`) and `embed_spa.go` (tag `spa`, embeds `all:dist`).
- REQ-003: When built with `-tags spa`, the binary SHALL serve the SPA from the embedded filesystem via an SPA handler.
- REQ-004: When built without the `spa` tag, the binary SHALL support a `--dev` flag that proxies requests to the Vite dev server.
- REQ-005: The dev proxy SHALL be implemented as a `NewSPADevProxy(devURL string)` function returning an `http.Handler` backed by `httputil.NewSingleHostReverseProxy`.
- REQ-006: The SPA handler SHALL use `fs.Sub(webFS, "dist")` to strip the embed directory prefix before serving.

### SPA Handler Behavior

- REQ-007: The SPA handler SHALL serve static files from the embedded filesystem. If a requested path matches a real file, it SHALL serve that file.
- REQ-008: If a requested path does not match a real file, the SPA handler SHALL fall back to serving `index.html` for client-side routing.
- REQ-009: Files under the `assets/` path SHALL receive the HTTP header `Cache-Control: public, max-age=31536000, immutable` (Vite content-hashes these filenames).
- REQ-010: `index.html` SHALL NOT receive immutable cache headers.

### Route Priority

- REQ-011: API routes SHALL be registered on the mux before the SPA catch-all handler. API routes SHALL always take priority over the SPA.

### React Application

- REQ-012: The dashboard SHALL display a table of all notifications with their current state, email, retry count, and retry limit.
- REQ-013: The dashboard SHALL connect to `/v1/ws` via WebSocket to receive real-time state updates without polling.
- REQ-014: The dashboard SHALL provide a resend button that calls `POST /v1/notify/reset` for a given notification.
- REQ-015: The dashboard SHALL include an API client module for communicating with the REST endpoints.

### Tailwind CSS and Dark Mode

- REQ-016: The frontend SHALL use Tailwind CSS for styling.
- REQ-017: The frontend SHALL support light and dark mode via Tailwind's `dark` class strategy.
- REQ-018: A dark mode switcher component SHALL detect system preference and allow manual toggle.

### Storybook

- REQ-019: The project SHALL include Storybook using `@storybook/react-vite`, sharing the Vite config so Tailwind and aliases work automatically.
- REQ-020: Stories SHALL use CSF 3.0 format with `tags: ['autodocs']` for automatic documentation.
- REQ-021: A dark mode decorator SHALL be configured in `.storybook/preview.tsx` that toggles the `dark` class on `<html>` based on a global `theme` toolbar control.
- REQ-022: The `@storybook/addon-a11y` accessibility addon SHALL be installed and added to `.storybook/main.ts` addons. It SHALL run axe-core checks on every story.
- REQ-023: Reusable components (button, pagination, dark mode switcher) SHALL have individual story files.

### Shared Branding

- REQ-024: The dashboard and email templates SHALL share the same brand color palette via a shared `brand.json` file at the project root. The Tailwind config SHALL import `brand.json` to define custom color tokens. The Maizzle email build SHALL also import `brand.json` to resolve brand colors into inlined CSS at compile time. At runtime, the Go service only injects dynamic values into pre-compiled email HTML via `html/template` — no runtime CSS processing occurs.

## Scenarios

### Scenario: SPA served from embedded filesystem

- **Given** the binary is built with `-tags spa`
- **When** a GET request is sent to `/`
- **Then** the system SHALL serve `index.html` from the embedded filesystem

### Scenario: Client-side routing fallback

- **Given** the binary is built with `-tags spa`
- **When** a GET request is sent to `/notifications/ntf_abc123`
- **And** no file exists at that path in the embedded filesystem
- **Then** the system SHALL serve `index.html` for client-side routing

### Scenario: Hashed assets receive immutable cache headers

- **Given** the binary is built with `-tags spa`
- **When** a GET request is sent to `/assets/index-abc123.js`
- **Then** the response SHALL include `Cache-Control: public, max-age=31536000, immutable`

### Scenario: Dev proxy to Vite

- **Given** the binary is built without the `spa` tag
- **And** the Vite dev server is running on `http://localhost:5173`
- **When** the service starts with `--dev --dev-url http://localhost:5173`
- **And** a GET request is sent to `/`
- **Then** the request SHALL be proxied to `http://localhost:5173`

### Scenario: API routes take priority over SPA

- **Given** the binary is built with `-tags spa`
- **When** a GET request is sent to `/v1/health`
- **Then** the system SHALL return the health check JSON response
- **And** the request SHALL NOT be handled by the SPA handler

### Scenario: Real-time dashboard update via WebSocket

- **Given** the dashboard is loaded in a browser and connected to `/v1/ws`
- **When** a notification transitions from `sending` to `delivered`
- **Then** the dashboard SHALL update the notification's state in the table without a page refresh

### Scenario: Storybook dark mode decorator

- **Given** Storybook is running on port 6006
- **When** the user selects "dark" from the theme toolbar
- **Then** the `<html>` element SHALL have the class `dark`
- **And** all components SHALL render in dark mode
