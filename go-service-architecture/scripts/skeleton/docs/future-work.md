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

## Resend Button UX

- Consider whether the resend button should appear on notifications
  in `not_sent` state that still have retries remaining. Currently
  it shows for both `failed` and `not_sent`. The auto-retry may
  resolve `not_sent` on its own — showing resend might confuse users
  or cause a race between manual reset and auto-retry
- Options: only show resend after retries are exhausted (retry_count
  >= retry_limit), or show it always but with a tooltip explaining
  that auto-retry is in progress
