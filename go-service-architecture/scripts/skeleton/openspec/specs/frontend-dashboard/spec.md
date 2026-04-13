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

- REQ-012: The dashboard SHALL display a table of all notifications with their current state, email, retry count, and retry limit. Below the table, the pagination component SHALL display the current page number and total page count.
- REQ-013: The dashboard SHALL connect to `/v1/ws` via WebSocket to receive real-time state updates without polling.
- REQ-014: The dashboard SHALL provide a Resend button for `failed`/`not_sent` notifications and a Reset button for `delivered` notifications. Both buttons call `POST /v1/notify/reset` with the notification's email address.
- REQ-015: The dashboard SHALL include an API client module for communicating with the REST endpoints.

### Table Border Styling

- REQ-046: The notifications table SHALL be wrapped in a container `<div>` with `rounded-lg border` and `overflow-hidden` to clip child content to the rounded corners. The container border SHALL be the only visible border on the table's outer edges.
- REQ-047: Table rows SHALL NOT have explicit `border-b` classes. Inter-row borders SHALL be achieved exclusively via `divide-y` on the `<tbody>` element, which applies a top border to every row except the first.
- REQ-048: The `<table>` element SHALL NOT have its own border styles. The visual border SHALL come solely from the wrapping container `<div>`.
- REQ-049: No `<hr>` elements SHALL appear inside or immediately after the table container.

### Empty State

- REQ-050: When the notification list is empty (zero rows), the dashboard SHALL render the table with its header row and a single `<tbody>` row containing a `<td>` that spans all columns (`colSpan` equal to the number of header columns).
- REQ-051: The empty state `<td>` SHALL display the text "No notifications yet" centered within the cell, using muted text color (`text-gray-500 dark:text-gray-400`) and vertical padding (`py-12`) for visual emphasis.
- REQ-052: The empty state SHALL be rendered inside the same table structure (same `<div>` container, same `<table>`, same `<thead>`) as the populated state, so that the table headers remain visible.
- REQ-053: The `Dashboard.stories.tsx` Empty story SHALL render the empty state inside the table (headers visible with the "No notifications yet" row), NOT as a standalone message outside the table.

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

### Pagination Enhancement

- REQ-054: The `Pagination` component SHALL accept `currentPage` (number, 1-indexed), `totalPages` (number), and `onPageChange` (callback accepting a page number) props in addition to the existing navigation props.
- REQ-055: The `Pagination` component SHALL display numbered page buttons for direct page access. When the total number of pages exceeds 7, the component SHALL show the first page, last page, current page with its immediate neighbors, and ellipsis indicators for skipped ranges.
- REQ-056: The `Pagination` component SHALL highlight the current page button with a visually distinct style (e.g., filled background using the `primary` Button variant) while non-current pages use the `secondary` variant.
- REQ-057: The `Pagination` component SHALL display a "Page X of Y" text label indicating the current page and total pages.
- REQ-058: When `totalPages` is 0 or 1, the `Pagination` component SHALL NOT render any page buttons or navigation controls.
- REQ-059: The `Pagination.stories.tsx` file SHALL include stories for: `ManyPages` (e.g., 10+ pages, current page in the middle showing ellipsis), `SinglePage` (1 page, controls hidden), `FirstPage` (current page is 1), `LastPage` (current page is the last), and `FewPages` (e.g., 5 pages, all page numbers visible without ellipsis).
- REQ-060: The `useNotifications` hook SHALL expose `currentPage` (number, 1-indexed), `totalPages` (number), and `totalCount` (number) derived from the API response `meta.total_count` and `meta.total_pages`. It SHALL also expose a `goToPage(page: number)` callback.

### Layout Stability

- REQ-061: Interactive buttons whose visible text changes between states (e.g., "Resend" / "Resending...", "Reset" / "Resetting...") SHALL have a stable rendered width that does not change when the text changes. The button SHALL use a CSS `min-width` (or equivalent constraint) that accommodates the widest text variant, preventing table column resize and layout shift on state change.
- REQ-062: The Resend button in `NotificationRow` SHALL have a `min-width` that accommodates the "Resending..." label so that switching between "Resend" and "Resending..." does not cause the Actions column or the table to reflow.

### Resend Button Visibility

- REQ-064: For notifications in `failed` state, the Resend button SHALL always be visible and enabled (clickable). The `failed` state has no auto-retry mechanism.
- REQ-065: For notifications in `not_sent` state where `retry_count < retry_limit` (auto-retry still in progress), the Resend button SHALL be visible but disabled (not clickable). The button SHALL appear with a disabled visual treatment (e.g., reduced opacity, `cursor-not-allowed`).
- REQ-066: For notifications in `not_sent` state where `retry_count >= retry_limit` (retries exhausted), the Resend button SHALL be visible and enabled (clickable).
- REQ-067: The `NotificationRow` component SHALL determine button disabled state by evaluating both the `resending` prop (HTTP request in flight) and the auto-retry condition (`status === 'not_sent' && retry_count < retry_limit`). The button SHALL be disabled if either condition is true.

### Resend Race Condition

- REQ-063: When the `useNotifications` hook receives a WebSocket update that changes a notification's state to a non-actionable state (`pending` or `sending`), it SHALL immediately remove that notification's ID from both the `resending` Set and the `resetting` Set (if tracked separately), regardless of whether the HTTP request has completed. This prevents a render frame where a button is simultaneously disabled (request in progress) and absent (state no longer shows that button).

### Reset Button for Delivered Notifications

- REQ-068: For notifications in `delivered` state, the `NotificationRow` component SHALL display a "Reset" button. The Reset button SHALL NOT appear for any other status.
- REQ-069: The Reset button SHALL call `POST /v1/notify/reset` with the notification's email address when clicked. The `NotificationRowProps` interface SHALL accept an `onReset` callback `(id: string) => void` and a `resetting` boolean prop.
- REQ-070: While the reset HTTP request is in flight, the Reset button SHALL display the text "Resetting..." and SHALL be disabled.
- REQ-071: The Reset button SHALL have a CSS `min-width` (e.g., `min-w-[7.5rem]`) that accommodates the "Resetting..." label so that switching between "Reset" and "Resetting..." does not cause the Actions column or the table to reflow.
- REQ-072: After a successful reset, the WebSocket update SHALL transition the notification to `pending` state. The `useNotifications` hook SHALL remove the notification's ID from the resetting tracking state when it receives a WebSocket update changing the notification's state away from `delivered`.
- REQ-073: The Resend button and the Reset button SHALL NOT both appear on the same notification row. The Resend button appears for `failed` and `not_sent` states; the Reset button appears for `delivered` state. The `pending` and `sending` states show no action button.

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

### Scenario: Table has clean rounded borders without double border at bottom

- **Given** the notifications table contains one or more rows
- **When** the table renders in the browser
- **Then** the wrapping `<div>` SHALL have `rounded-lg`, `border`, and `overflow-hidden` classes
- **And** no `<tr>` inside the table SHALL have a `border-b` class
- **And** the `<tbody>` SHALL use `divide-y` to separate rows
- **And** no `<hr>` element SHALL be present inside or immediately after the table container

### Scenario: Empty state displays message inside table

- **Given** the notification list is empty (zero notifications)
- **When** the dashboard renders
- **Then** the table `<thead>` with column headers SHALL be visible
- **And** the `<tbody>` SHALL contain exactly one `<tr>` with a single `<td>` spanning all columns
- **And** the `<td>` SHALL display the text "No notifications yet"
- **And** the text SHALL be centered with muted color styling

### Scenario: Storybook Empty story shows in-table empty state

- **Given** `Dashboard.stories.tsx` is loaded in Storybook
- **When** the "Empty" story renders
- **Then** the table headers (ID, Email, Status, Retries, Actions) SHALL be visible
- **And** a single row SHALL display "No notifications yet" spanning all columns
- **And** no content SHALL appear outside the table container

### Scenario: Pagination displays page numbers

- **Given** 50 notifications exist and the page size is 20
- **When** the dashboard loads
- **Then** the pagination component SHALL display page buttons "1", "2", "3"
- **And** page 1 SHALL be highlighted as the current page
- **And** the text "Page 1 of 3" SHALL be visible

### Scenario: Pagination shows ellipsis for many pages

- **Given** 200 notifications exist and the page size is 20
- **When** the user navigates to page 5
- **Then** the pagination component SHALL display "1 ... 4 5 6 ... 10"
- **And** page 5 SHALL be highlighted as the current page

### Scenario: Pagination hidden for single page

- **Given** 10 notifications exist and the page size is 20
- **When** the dashboard loads
- **Then** the pagination component SHALL NOT render page buttons or Previous/Next controls

### Scenario: Resend button width remains stable during state change

- **Given** a notification row with status `failed` and the Resend button visible
- **When** the user clicks "Resend" and the button text changes to "Resending..."
- **Then** the button's rendered width SHALL NOT change
- **And** the Actions column width SHALL NOT change
- **And** the table SHALL NOT shift horizontally

### Scenario: Storybook pagination stories exist

- **Given** `Pagination.stories.tsx` is loaded in Storybook
- **When** the stories list is displayed
- **Then** stories SHALL exist for: `ManyPages`, `SinglePage`, `FirstPage`, `LastPage`, and `FewPages`

### Scenario: Resend button disabled during auto-retry for not_sent

- **Given** a notification with status `not_sent`, `retry_count` of 1, and `retry_limit` of 3
- **When** the `NotificationRow` component renders
- **Then** the Resend button SHALL be visible
- **And** the Resend button SHALL be disabled (not clickable)
- **And** the button SHALL have a disabled visual treatment (reduced opacity, `cursor-not-allowed`)

### Scenario: Resend button enabled when retries exhausted for not_sent

- **Given** a notification with status `not_sent`, `retry_count` of 3, and `retry_limit` of 3
- **When** the `NotificationRow` component renders
- **Then** the Resend button SHALL be visible
- **And** the Resend button SHALL be enabled (clickable)

### Scenario: Resend button always enabled for failed status

- **Given** a notification with status `failed`, `retry_count` of 0, and `retry_limit` of 3
- **When** the `NotificationRow` component renders
- **Then** the Resend button SHALL be visible and enabled
- **And** the button SHALL NOT be disabled regardless of retry_count or retry_limit values

### Scenario: WebSocket update clears resending state for non-resendable transition

- **Given** the user has clicked "Resend" on notification `ntf_abc123` and the `resending` Set contains `ntf_abc123`
- **And** the resend HTTP request has not yet completed
- **When** a WebSocket message arrives setting `ntf_abc123` state to `pending`
- **Then** the `useNotifications` hook SHALL remove `ntf_abc123` from the `resending` Set
- **And** the notification row SHALL render with the `pending` StatusBadge and no Resend button
- **And** there SHALL be no render frame where the button is both disabled and absent

### Scenario: Reset button displayed on delivered notification

- **Given** a notification with status `delivered`
- **When** the `NotificationRow` component renders
- **Then** a "Reset" button SHALL be visible and enabled
- **And** no "Resend" button SHALL be present on the row

### Scenario: Reset button not displayed on non-delivered statuses

- **Given** a notification with status `failed`
- **When** the `NotificationRow` component renders
- **Then** no "Reset" button SHALL be present on the row
- **And** the "Resend" button SHALL be visible

### Scenario: Reset button shows loading state during request

- **Given** a notification with status `delivered` and the Reset button visible
- **When** the user clicks "Reset"
- **Then** the button text SHALL change to "Resetting..."
- **And** the button SHALL be disabled
- **And** the button's rendered width SHALL NOT change

### Scenario: Reset completes and WebSocket updates row

- **Given** the user has clicked "Reset" on a delivered notification `ntf_xyz789`
- **When** the reset HTTP request succeeds (HTTP 204)
- **And** a WebSocket message arrives setting `ntf_xyz789` state to `pending`
- **Then** the notification row SHALL display the `pending` StatusBadge
- **And** no action button (Reset or Resend) SHALL be present on the row
- **And** the `useNotifications` hook SHALL remove `ntf_xyz789` from the resetting tracking state

### Scenario: No action button on pending or sending notifications

- **Given** a notification with status `pending`
- **When** the `NotificationRow` component renders
- **Then** no "Resend" button SHALL be present
- **And** no "Reset" button SHALL be present
