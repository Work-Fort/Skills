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

Three small color changes to pass the existing a11y test suite (33 tests). All
fixes bring foreground/background contrast ratios above the WCAG AA 4.5:1
threshold.

## Prerequisites

- All 33 a11y tests currently exist; some fail due to insufficient contrast.

## Task Breakdown

### Task 1: Darken brand accent color

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

### Task 2: Fix NotificationRow dark-mode text contrast

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

### Task 3: Fix DarkModeToggle stories helper text contrast

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

## Verification Checklist

1. Run `mise run test:a11y` — all 33 tests pass.
2. Confirm `brand.json` accent is `#d43555`.
3. Confirm `index.css` accent is `#d43555`.
4. Confirm NotificationRow uses `dark:text-gray-300` on both cells.
5. Confirm DarkModeToggle stories uses `text-gray-600`.
