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

### Typography

- REQ-025: `brand.json` SHALL include `fontSans` and `fontMono` keys defining font family stacks as strings. `fontSans` SHALL be `"Inter, ui-sans-serif, system-ui, sans-serif"`. `fontMono` SHALL be `"JetBrains Mono, ui-monospace, monospace"`.
- REQ-026: The dashboard `index.css` `@theme` block SHALL declare `--font-sans` and `--font-mono` CSS custom properties whose values match the `fontSans` and `fontMono` values from `brand.json`.
- REQ-027: The Maizzle email `tailwind.config.js` SHALL import `fontSans` and `fontMono` from `brand.json` and configure them as `theme.extend.fontFamily.sans` and `theme.extend.fontFamily.mono` respectively.

### Semantic Color Tokens

- REQ-028: `brand.json` SHALL include a `semantic` object with keys `success`, `warning`, `danger`, `info`, and `neutral`. Each key SHALL map to an object with `light` and `dark` sub-keys. Each sub-key SHALL be an object containing `bg` and `text` color values.
  - `success`: green tones (`light.bg: "#dcfce7"`, `light.text: "#166534"`, `dark.bg: "#052e16"`, `dark.text: "#86efac"`)
  - `warning`: orange tones (`light.bg: "#ffedd5"`, `light.text: "#9a3412"`, `dark.bg: "#431407"`, `dark.text: "#fdba74"`)
  - `danger`: red tones (`light.bg: "#fee2e2"`, `light.text: "#991b1b"`, `dark.bg: "#450a0a"`, `dark.text: "#fca5a5"`)
  - `info`: purple tones (`light.bg: "#f3e8ff"`, `light.text: "#6b21a8"`, `dark.bg: "#3b0764"`, `dark.text: "#d8b4fe"`)
  - `neutral`: yellow tones (`light.bg: "#fef9c3"`, `light.text: "#854d0e"`, `dark.bg: "#422006"`, `dark.text: "#fde047"`)
- REQ-029: The dashboard `index.css` `@theme` block SHALL declare CSS custom properties for each semantic token following the pattern `--color-semantic-{name}-{property}` for light mode values (e.g., `--color-semantic-success-bg`, `--color-semantic-success-text`). Dark mode overrides SHALL be declared in a `.dark` selector using the same property names, pointing to the `dark` sub-key values from `brand.json`.

### StatusBadge Colors

- REQ-030: The `StatusBadge` component SHALL use semantic color tokens for its status-to-color mapping. The mapping SHALL be: `pending` = `neutral` (yellow), `sending` = `info` (purple), `delivered` = `success` (green), `failed` = `danger` (red), `not_sent` = `warning` (orange).
- REQ-031: The `StatusBadge` component SHALL reference Tailwind utility classes derived from the semantic CSS custom properties (e.g., `bg-semantic-success-bg text-semantic-success-text`), NOT hardcoded Tailwind color scale classes (e.g., NOT `bg-green-100`).

### Button Variants

> **Badge vs. Button token usage:** Badges (non-interactive status indicators) use the lighter `bg` token as background with the darker `text` token as foreground (`bg-semantic-{name}-bg text-semantic-{name}-text`). Buttons (interactive controls) use the darker `text` token as background with white text (`bg-semantic-{name}-text text-white`) to provide higher contrast and a stronger visual affordance for clickable elements.

- REQ-032: The `Button` component SHALL support the following `variant` values: `primary`, `secondary`, `success`, `warning`, `info`, `danger`. The TypeScript type for `variant` SHALL be a union of these string literals.
- REQ-033: The `success` variant SHALL use the semantic success `text` token as background with white text (`bg-semantic-success-text text-white`), auto-switching via CSS custom properties in dark mode. Hover state SHALL darken the background by 10% using Tailwind opacity modifier or a dedicated hover token.
- REQ-034: The `warning` variant SHALL use the semantic warning `text` token as background with white text (`bg-semantic-warning-text text-white`), following the same button pattern as `success`.
- REQ-035: The `info` variant SHALL use the semantic info `text` token as background with white text (`bg-semantic-info-text text-white`), following the same button pattern as `success`.
- REQ-036: The `danger` variant SHALL use the semantic danger `text` token as background with white text (`bg-semantic-danger-text text-white`), following the same button pattern as `success`.
- REQ-037: The `Button.stories.tsx` file SHALL include stories for each new variant (`Success`, `Warning`, `Info`, `Danger`) in addition to the existing `Primary`, `Secondary`, and `Disabled` stories. The `argTypes.variant` control SHALL list all six variants.

### Contrast and Accessibility

- REQ-038: The project SHALL include a Storybook test-runner configuration file at `web/.storybook/test-runner.ts` that configures `@storybook/test-runner` with `preVisit` and `postVisit` hooks for axe-playwright accessibility checks.
- REQ-039: The test-runner SHALL validate WCAG 2.1 Level AA compliance (axe-core tags `wcag2a`, `wcag2aa`, `wcag21aa`) on every story, covering WCAG 1.4.3 (text contrast, 4.5:1 minimum) and WCAG 1.4.11 (non-text contrast, 3:1 minimum for UI component boundaries).
- REQ-040: The test-runner SHALL execute accessibility checks in both light mode and dark mode for every story by toggling the `dark` class on the `<html>` element before running the axe scan, ensuring both themes pass WCAG AA contrast requirements.
- REQ-041: A mise task `test:a11y` SHALL exist at `.mise/tasks/test/a11y` that builds Storybook statically, serves it via `http-server`, and runs `npx test-storybook --url http://127.0.0.1:6006` from the `web/` directory. It SHALL use `concurrently` to manage the server and test-runner processes.
- REQ-042: The `mise run ci` task SHALL include `test:a11y` in its dependency list so that accessibility checks run as part of every CI pipeline execution.
- REQ-043: The `@storybook/test-runner` package and `axe-playwright` package SHALL be listed in `web/package.json` devDependencies.
- REQ-044: The test-runner config SHALL use `preVisit` to inject axe into the page and `postVisit` to run axe checks after each story renders, calling `checkA11y(page)` from `axe-playwright`.
- REQ-045: The `@storybook/addon-a11y` SHALL remain installed and configured in `.storybook/main.ts` for interactive axe-core checks during development. The test-runner provides the automated CI gate in addition to the interactive addon.

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

### Scenario: Brand fonts defined in brand.json

- **Given** `brand.json` exists at the project root
- **When** its contents are parsed
- **Then** `fontSans` SHALL equal `"Inter, ui-sans-serif, system-ui, sans-serif"`
- **And** `fontMono` SHALL equal `"JetBrains Mono, ui-monospace, monospace"`

### Scenario: Dashboard consumes brand fonts via Tailwind theme

- **Given** `index.css` contains a `@theme` block
- **When** the CSS is processed by Tailwind
- **Then** the `--font-sans` custom property SHALL resolve to the `fontSans` value from `brand.json`
- **And** the `--font-mono` custom property SHALL resolve to the `fontMono` value from `brand.json`

### Scenario: StatusBadge displays distinct colors per status

- **Given** a notification with status `sending`
- **When** the `StatusBadge` component renders
- **Then** the badge SHALL use the `info` semantic token (purple tones)
- **And** the badge SHALL NOT use yellow tones (which are reserved for `pending`)

### Scenario: StatusBadge not_sent uses warning color

- **Given** a notification with status `not_sent`
- **When** the `StatusBadge` component renders
- **Then** the badge SHALL use the `warning` semantic token (orange tones)
- **And** the badge SHALL NOT use gray tones

### Scenario: Semantic tokens switch in dark mode

- **Given** the `dark` class is present on `<html>`
- **When** a `StatusBadge` with status `delivered` renders
- **Then** the badge background SHALL use `--color-semantic-success-bg` which resolves to `#052e16` in dark mode
- **And** the badge text SHALL use `--color-semantic-success-text` which resolves to `#86efac` in dark mode

### Scenario: Button danger variant renders with red semantic tokens

- **Given** a `Button` with `variant="danger"`
- **When** the component renders in light mode
- **Then** the button background SHALL use `--color-semantic-danger-text` (`#991b1b`)
- **And** the button text SHALL be white (`#ffffff`)

### Scenario: Button stories include all variants

- **Given** `Button.stories.tsx` is loaded in Storybook
- **When** the stories list is displayed
- **Then** stories SHALL exist for: `Primary`, `Secondary`, `Success`, `Warning`, `Info`, `Danger`, and `Disabled`

### Scenario: Email templates consume brand fonts

- **Given** the Maizzle `tailwind.config.js` imports `brand.json`
- **When** the email build compiles
- **Then** the `fontFamily.sans` theme value SHALL match `brand.json` `fontSans`
- **And** the `fontFamily.mono` theme value SHALL match `brand.json` `fontMono`

### Scenario: Accessibility test-runner checks all stories in light mode

- **Given** Storybook is running on port 6006
- **And** the test-runner is configured with axe-playwright
- **When** `npx test-storybook` executes
- **Then** every story SHALL be scanned by axe-core with tags `wcag2a`, `wcag2aa`, and `wcag21aa`
- **And** any violation SHALL cause the test to fail with the axe violation report

### Scenario: Accessibility test-runner checks dark mode contrast

- **Given** Storybook is running on port 6006
- **And** the test-runner setup adds the `dark` class to `<html>` before scanning
- **When** a story renders a dark-background component
- **Then** axe-core SHALL verify that text-to-background contrast meets 4.5:1 (WCAG 1.4.3)
- **And** axe-core SHALL verify that non-text UI element contrast meets 3:1 (WCAG 1.4.11)

### Scenario: CI pipeline runs accessibility checks

- **Given** `mise run ci` is invoked
- **When** the task dependency graph is resolved
- **Then** the `test:a11y` task SHALL execute
- **And** a WCAG violation in any story SHALL cause the CI pipeline to fail

### Scenario: axe-playwright packages are installed

- **Given** `web/package.json` is read
- **When** devDependencies are inspected
- **Then** `@storybook/test-runner` SHALL be present
- **And** `axe-playwright` SHALL be present
