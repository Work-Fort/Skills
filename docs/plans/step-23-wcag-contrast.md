---
type: plan
step: "23"
title: "WCAG AA contrast fixes"
status: pending
assessment_status: not_needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "23"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans: []
---

# Step 23: WCAG AA Contrast Fixes

## Overview

Color fixes to pass the existing a11y test suite (33 tests). All fixes bring
foreground/background contrast ratios above the WCAG AA 4.5:1 threshold.

The first three tasks were completed in the initial pass, reducing failures from
21 to 19. Task 4 addresses the remaining failures: semantic button variants
(success, warning, info, danger) use light pastel backgrounds in dark mode with
white text, producing ~1.4-1.9:1 contrast ratios.

## Prerequisites

- All 33 a11y tests currently exist; 19 still fail due to insufficient contrast
  on semantic button variants in dark mode.

## Task Breakdown

### Task 1: Darken brand accent color (DONE)

The accent `#e94560` does not meet 4.5:1 contrast with white text. Replace it
with `#d43555` (passes 4.5:1).

**Files:**
- Modify: `go-service-architecture/scripts/skeleton/brand.json:2` (accent value)
- Modify: `go-service-architecture/scripts/skeleton/web/src/index.css:12` (@theme accent)

**Step 1: Update brand.json**

In `brand.json`, change the accent value:

```json
"accent": "#d43555",
```

**Step 2: Update index.css @theme block**

In `web/src/index.css`, change line 12:

```css
--color-brand-accent: #d43555;
```

**Step 3: Commit**

`fix(web): darken brand accent to #d43555 for WCAG AA contrast`

---

### Task 2: Fix NotificationRow dark-mode text contrast (DONE)

The ID cell (line 50) and retries cell (line 59) use `dark:text-gray-400` which
is too dim against the dark background. Bump to `dark:text-gray-300`.

**Files:**
- Modify: `go-service-architecture/scripts/skeleton/web/src/components/NotificationRow.tsx:50,59`

**Step 1: Update both cells**

Line 50 — change `dark:text-gray-400` to `dark:text-gray-300`:

```tsx
<td className="whitespace-nowrap px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-300">
```

Line 59 — same change:

```tsx
<td className="whitespace-nowrap px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
```

**Step 2: Commit**

`fix(web): bump NotificationRow dark text to gray-300 for WCAG AA`

---

### Task 3: Fix DarkModeToggle stories helper text contrast (DONE)

The decorator helper text in the stories file uses `text-gray-500` in light
mode, which fails 4.5:1 on a white background. Change to `text-gray-600`.

**Files:**
- Modify: `go-service-architecture/scripts/skeleton/web/src/components/DarkModeToggle.stories.tsx:10`

**Step 1: Update helper text class**

Line 10 — change `text-gray-500` to `text-gray-600`:

```tsx
<span className="text-sm text-gray-600 dark:text-gray-400">
```

**Step 2: Commit**

`fix(web): darken DarkModeToggle helper text for WCAG AA contrast`

---

### Task 4: Fix semantic button variant text contrast in dark mode

In dark mode, the semantic `-text` CSS custom properties resolve to light pastel
colors (e.g., success = `#86efac`, warning = `#fdba74`, info = `#d8b4fe`,
danger = `#fca5a5`). These are used as button **backgrounds** via
`bg-semantic-{name}-text`. The button text is hardcoded `text-white`, which
produces contrast ratios of ~1.4-1.9:1 against those pastel backgrounds.

The fix: in dark mode, switch button text from white to `gray-900` (`#111827`),
which provides >7:1 contrast against all four pastel backgrounds.

**Files:**
- Modify: `go-service-architecture/scripts/skeleton/web/src/components/Button.tsx:30-36`

**Step 1: Update semantic variant classes**

In `Button.tsx`, change each semantic variant to replace `text-white` with
`text-white dark:text-gray-900`:

```tsx
const variants: Record<ButtonVariant, string> = {
    primary:
      'bg-brand-accent text-white hover:bg-brand-accent/90 focus:ring-brand-accent',
    secondary:
      'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus:ring-brand-accent dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700',
    success:
      'bg-semantic-success-text text-white dark:text-gray-900 hover:opacity-90 focus:ring-semantic-success-text',
    warning:
      'bg-semantic-warning-text text-white dark:text-gray-900 hover:opacity-90 focus:ring-semantic-warning-text',
    info:
      'bg-semantic-info-text text-white dark:text-gray-900 hover:opacity-90 focus:ring-semantic-info-text',
    danger:
      'bg-semantic-danger-text text-white dark:text-gray-900 hover:opacity-90 focus:ring-semantic-danger-text',
  }
```

**Contrast verification (dark mode backgrounds vs `#111827`):**

| Variant | Background | vs `#111827` | Ratio |
|---------|-----------|--------------|-------|
| Success | `#86efac` | dark on light | ~10.5:1 |
| Warning | `#fdba74` | dark on light | ~11.2:1 |
| Info    | `#d8b4fe` | dark on light | ~8.9:1 |
| Danger  | `#fca5a5` | dark on light | ~8.4:1 |

All exceed 4.5:1 (AA) and 7:1 (AAA).

**Light mode is unaffected** — `text-white` still applies in light mode where
the `-text` tokens are dark greens/reds/purples, maintaining existing contrast.

**Step 2: Run a11y tests**

Run: `mise run test:a11y`
Expected: all 33 tests pass.

**Step 3: Commit**

`fix(web): use dark text on semantic buttons in dark mode for WCAG AA`

---

## Verification Checklist

1. Run `mise run test:a11y` — all 33 tests pass.
2. Confirm `brand.json` accent is `#d43555`.
3. Confirm `index.css` accent is `#d43555`.
4. Confirm NotificationRow uses `dark:text-gray-300` on both cells.
5. Confirm DarkModeToggle stories uses `text-gray-600`.
6. Confirm Button.tsx semantic variants use `text-white dark:text-gray-900`.
7. Visually check Storybook: semantic buttons should show dark text on pastel
   backgrounds in dark mode, white text on dark backgrounds in light mode.
