# Future Work

Items identified during the skeleton app build that are out of scope
for the initial implementation but worth revisiting.

## Typography and Font Faces

- Add `fontSans` and `fontMono` to `brand.json` as the single source
  of truth for typography across dashboard and email
- Switch email templates to a sans-serif stack (currently falling back
  to browser/client default serif)
- Dashboard should also reference brand fonts via Tailwind `@theme`
- Email font stacks need safe fallbacks — web fonts are unreliable in
  email clients (Gmail strips `@font-face`, Outlook ignores it)
- Consider: Inter, system-ui, sans-serif for body; JetBrains Mono or
  monospace for code/IDs
- Both the Maizzle `tailwind.config.js` and the dashboard `index.css`
  `@theme` block should consume the same font values from `brand.json`

## Contrast and Accessibility

- Dark mode has contrast issues — dark text on dark backgrounds in
  several components. The brand palette (`#1a1a2e` primary, `#16213e`
  surface, `#eaeaea` text) needs WCAG AA contrast ratio verification
  for all combinations
- Add automated contrast testing to Storybook using the pattern from
  Scope's documentation repo: a custom test-runner that validates
  WCAG 1.4.11 non-text contrast in both light and dark modes (see
  `/home/kazw/Work/WorkFort/documentation/storybook/lit/.storybook/test-runner.ts`)
- The `@storybook/addon-a11y` is already installed but only runs
  axe-core checks on individual stories. The Scope pattern goes
  further: it tests border contrast ratios for form inputs, buttons,
  checkboxes, and radio buttons against a 3:1 minimum
- Run contrast checks in CI as part of `mise run ci`

## StatusBadge Color Differentiation

- `pending` and `sending` both use yellow — they should be visually
  distinct since they represent different stages
- Change `sending` to purple to differentiate from `pending` (yellow)
- Final palette: pending=yellow, sending=purple, delivered=green,
  failed=red, not_sent=orange

## Button Variants

- Add color variants that match the status badge palette for
  visual consistency: success (green), warning (orange), info
  (purple/blue), danger (red), in addition to existing primary/secondary
- The status badge colors and button colors should come from the same
  semantic token set so they're always in sync

## Pagination Enhancement

- Current pagination is Previous/Next only with no page context
- Add page numbers and total page count (e.g., "Page 2 of 5")
- Backend needs: `GET /v1/notifications` response should include
  `meta.total_count` and `meta.total_pages` alongside existing
  `has_more` and `next_cursor`
- This requires a `SELECT COUNT(*)` query in both SQLite and
  PostgreSQL stores
- Frontend needs: Pagination component updated to show numbered
  page buttons and "Page X of Y" text
- MCP `list_notifications` tool response should also include totals
- Storybook story should show variants with many pages, single page,
  and current-page highlighting

## @example.com Auto-Fail Should Be Dev/QA Only

- The `@example.com` rejection is hardcoded in the SMTP sender with
  no build tag gating — production builds will also reject these
  addresses, which is wrong
- The auto-fail should only apply in dev and QA builds. Options:
  - **Build tag** (`//go:build qa`): Consistent with how seed data
    is gated. Requires a rebuild to toggle. Simple, no runtime cost.
  - **Config flag** (`smtp.simulate_failures: true`): Toggled via
    config file or env var without rebuilding. More flexible but
    adds a runtime check on every send and a config surface that
    could be misconfigured in production.
- Build tag is the simpler and safer choice — it matches the existing
  QA build pattern and makes it impossible to accidentally enable
  simulated failures in production

## QA Build: Console-Only Email, No Mailpit Required

- QA builds should be fully standalone — no Mailpit, no SMTP server
- Replace the SMTP sender with a console sender in QA builds that
  logs the email content (to, subject, body) to slog and returns
  success. The state machine still transitions normally.
- Use the same `//go:build qa` / `//go:build !qa` pattern:
  - QA: `ConsoleSender` logs to stdout, no network call
  - Dev: real SMTP sender pointing at Mailpit for actual email testing
  - Production: real SMTP sender pointing at production SMTP
- This means QA testers and demos only need the single binary — no
  external dependencies at all

## QA Build: Simulated Failure Domains

- Extend the QA sender with a map of domain-based simulated behaviors
  gated by `//go:build qa`:
  - `@example.com` — permanent failure
  - `@fail.com` — simulated timeout
  - `@slow.com` — extra-long delay (e.g., 30s)
- In non-QA builds the map is nil, no matches, no runtime cost
- This lets QA exercise all state machine paths without a real SMTP
  server

## Missing release:qa Mise Task

- There's no `release:qa` task — QA builds currently require manual
  `go build -tags spa,qa` which bypasses mise
- Add `.mise/tasks/release/qa` that builds with `-tags spa,qa` and
  includes the seed data, matching the pattern of `release:dev` and
  `release:production`
- The architecture docs should mention all three release tasks

## Dashboard Table Border Styling

- Extra horizontal rule at the bottom of the table beneath the
  rounded border — looks like a double border
- The bottom border appears thicker than the sides
- Fix: check for a stray `<hr>`, extra `border-bottom` on the last
  row, or a table `border-collapse` issue conflicting with
  `rounded` corners

## Empty State UX

- The empty dashboard just shows a blank table with headers — no
  indication that the empty state is intentional
- Add a full-width table row with a centered message like "No
  notifications yet" when the list is empty
- This applies to both the live dashboard and the Empty Storybook
  story variant

## Resend Button UX

- Consider whether the resend button should appear on notifications
  in `not_sent` state that still have retries remaining. Currently
  it shows for both `failed` and `not_sent`. The auto-retry may
  resolve `not_sent` on its own — showing resend might confuse users
  or cause a race between manual reset and auto-retry
- Options: only show resend after retries are exhausted (retry_count
  >= retry_limit), or show it always but with a tooltip explaining
  that auto-retry is in progress
