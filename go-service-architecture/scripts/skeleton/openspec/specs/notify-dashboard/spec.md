# Notification Dashboard

## Purpose

The notification dashboard is a React + TypeScript frontend built with Vite and styled with Tailwind CSS, embedded in the Go binary. It provides a web interface for viewing and managing notifications. It displays all notified emails with their delivery results, state machine states, and retry information, and offers a "Resend" action per row. The frontend is conditionally compiled via a `spa` build tag and supports a Vite dev server proxy for development. Real-time state updates are pushed to the browser via WebSocket.

## Requirements

- REQ-1: The system SHALL serve a React + TypeScript single-page application built with Vite from the Go binary when built with the `spa` build tag.
- REQ-2: The system SHALL embed the built frontend assets in the Go binary via `//go:embed` with the `spa` build tag.
- REQ-3: The system SHALL expose a GET `/v1/notifications` API endpoint that returns a paginated JSON response of notification records with cursor-based pagination.
- REQ-4: Each notification record in the API response SHALL include at minimum: email address, delivery result (success/failure/pending), current state machine state, retry count, and retry limit.
- REQ-5: The dashboard SHALL render a table listing all notifications with columns for: email address, delivery result, state machine state, and retry count.
- REQ-6: Each row in the dashboard table SHALL include a "Resend" button.
- REQ-7: The "Resend" button SHALL call POST `/v1/notify/reset` for the row's email address, then call POST `/v1/notify` for the same address, then refresh the notification list.
- REQ-8: The dashboard SHALL display visual feedback while the resend operation is in progress (e.g., loading state on the button).
- REQ-9: In development mode (without the `spa` build tag), the system SHALL proxy frontend requests to a Vite dev server for hot module replacement.
- REQ-10: The SPA SHALL handle client-side routing so that refreshing any dashboard URL serves the `index.html` fallback.
- REQ-11: The system SHALL serve the SPA assets at the root path `/` and API endpoints at `/v1/`.
- REQ-12: The dashboard SHALL use Tailwind CSS for styling with support for both light and dark mode, respecting the user's system preference.
- REQ-13: The system SHALL expose a WebSocket endpoint at GET `/v1/ws` that pushes real-time notification state updates to connected browsers.
- REQ-14: The dashboard SHALL connect to the WebSocket endpoint on load and update the notification list in real time as states change, without requiring manual refresh.
- REQ-15: The dashboard SHALL support pagination for the notifications list, loading additional pages as the user scrolls or clicks a "load more" control.
- REQ-16: The dashboard and email templates SHALL share the same brand color palette and styling, extracted from a shared configuration, to maintain consistent branding across web UI and transactional email.

## Scenarios

#### Scenario: Dashboard lists all notifications
- GIVEN the database contains notifications for "a@test.com" (delivered), "b@test.com" (failed), and "c@test.com" (pending)
- WHEN a user opens the dashboard in a browser
- THEN the dashboard SHALL display a table with three rows
- AND each row SHALL show the email, delivery result, current state, and retry count

#### Scenario: Resend via dashboard
- GIVEN the dashboard is displaying a notification for "user@real.com" in `delivered` state
- WHEN the user clicks the "Resend" button for that row
- THEN the dashboard SHALL call POST `/v1/notify/reset` with `{"email": "user@real.com"}`
- AND then call POST `/v1/notify` with `{"email": "user@real.com"}`
- AND then refresh the notification list
- AND the row SHALL reflect the updated state

#### Scenario: Resend button shows loading state
- GIVEN the dashboard is displaying notifications
- WHEN the user clicks a "Resend" button
- THEN the button SHALL show a loading indicator
- AND the button SHALL be disabled until the operation completes

#### Scenario: Embedded SPA served from binary
- GIVEN the binary is built with the `spa` build tag
- WHEN a GET request is made to `/`
- THEN the system SHALL serve the embedded `index.html`
- AND static assets (JS, CSS) SHALL be served from the embedded filesystem

#### Scenario: Dev mode proxies to Vite
- GIVEN the binary is built without the `spa` build tag
- WHEN a GET request is made to `/` in development
- THEN the system SHALL proxy the request to the Vite dev server

#### Scenario: API endpoint returns paginated notification list
- GIVEN the database contains twenty notification records
- WHEN a GET request is made to `/v1/notifications` without a cursor
- THEN the system SHALL return HTTP 200 with the first page of notification objects
- AND each object SHALL contain `email`, `result`, `state`, `retry_count`, and `retry_limit` fields
- AND the response SHALL include a cursor for fetching the next page

#### Scenario: WebSocket pushes real-time state updates
- GIVEN a browser is connected to the WebSocket at `/v1/ws`
- AND a notification for "user@real.com" is in `sending` state
- WHEN the notification transitions to `delivered`
- THEN the WebSocket SHALL push a state update message to the connected browser
- AND the dashboard SHALL update the row for "user@real.com" without a full page refresh

#### Scenario: Dashboard displays retry count
- GIVEN the database contains a notification in `not_sent` state with retry count 2 and retry limit 3
- WHEN a user opens the dashboard in a browser
- THEN the row for that notification SHALL display the retry count (2) and indicate retries remaining

#### Scenario: Dark mode respects system preference
- GIVEN the user's operating system is set to dark mode
- WHEN the user opens the dashboard in a browser
- THEN the dashboard SHALL render with a dark color scheme
- AND all UI elements SHALL be legible and styled appropriately for dark mode

#### Scenario: Shared branding between dashboard and email
- GIVEN the service is running with default brand configuration
- WHEN a user views the dashboard AND an email is sent
- THEN the dashboard color palette SHALL match the colors used in the email template

## Storybook & Component Development

### Requirements

- REQ-17: The project SHALL include a Storybook setup using `@storybook/react-vite` that shares the existing Vite configuration.
- REQ-18: The following reusable components SHALL be developed and documented in Storybook: Button, Pagination, DarkModeToggle, StatusBadge, and NotificationRow.
- REQ-19: Each Storybook story SHALL use CSF 3.0 format with controls (argTypes) and `tags: ['autodocs']` for automatic documentation generation.
- REQ-20: Storybook SHALL include the `@storybook/addon-a11y` accessibility addon, and all component stories SHALL pass automated accessibility checks.
- REQ-21: Dark mode switching SHALL use Tailwind's `dark:` class strategy by toggling the `dark` class on the `<html>` element.
- REQ-22: Storybook preview SHALL include a theme decorator that allows switching between light and dark themes within the Storybook UI.
- REQ-23: The assembled dashboard page SHALL have its own Storybook story showing the full view with mocked notification data.

### Scenarios

#### Scenario: Viewing a component in Storybook
- GIVEN Storybook is running via `mise run dev:storybook`
- WHEN a developer navigates to the Button story in the sidebar
- THEN Storybook SHALL render the Button component with interactive controls
- AND the autodocs tab SHALL display generated documentation with prop types and defaults

#### Scenario: Switching dark/light mode in Storybook
- GIVEN a developer is viewing any component story in Storybook
- WHEN the developer toggles the theme decorator from light to dark
- THEN the `<html>` element inside the story iframe SHALL have the `dark` class added
- AND the component SHALL re-render using Tailwind `dark:` variant styles

#### Scenario: Accessibility check in Storybook
- GIVEN a developer is viewing the StatusBadge story in Storybook
- WHEN the accessibility addon panel is open
- THEN the addon SHALL run automated axe-core checks against the rendered component
- AND the panel SHALL report zero violations for color contrast, ARIA attributes, and keyboard navigation
