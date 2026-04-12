---
type: plan
step: "13"
title: "Design Tokens"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "13"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-7-frontend-foundation
  - step-8-dashboard
---

# Step 13: Design Tokens

## Overview

Introduces a shared design token system for fonts and semantic colors.
Currently, StatusBadge hardcodes Tailwind color scale classes (e.g.,
`bg-yellow-100`), `pending` and `sending` both use yellow making them
indistinguishable, `not_sent` uses gray instead of orange, and the
Button component only has `primary`/`secondary` variants. Brand fonts
are not defined anywhere.

This step:

1. Adds `fontSans` and `fontMono` to `brand.json` and wires them into
   the dashboard `@theme` block and the Maizzle email
   `tailwind.config.js`. (Satisfies REQ-025, REQ-026, REQ-027)
2. Adds a `semantic` color token object to `brand.json` with `success`,
   `warning`, `danger`, `info`, and `neutral` keys, each containing
   light/dark bg/text values. (Satisfies REQ-028)
3. Declares semantic CSS custom properties in `index.css` with dark mode
   overrides. (Satisfies REQ-029)
4. Refactors StatusBadge to use semantic tokens: pending=neutral(yellow),
   sending=info(purple), delivered=success(green), failed=danger(red),
   not_sent=warning(orange). (Satisfies REQ-030, REQ-031)
5. Adds `success`, `warning`, `info`, `danger` button variants using the
   same semantic tokens. (Satisfies REQ-032 through REQ-036)
6. Updates Storybook stories for both components. (Satisfies REQ-037)

## Prerequisites

- Step 8 completed: StatusBadge, Button, and their Storybook stories
  exist in `web/src/components/`.
- `brand.json` exists at project root with color keys.
- `index.css` has a `@theme` block with brand color custom properties.
- Maizzle email `tailwind.config.js` imports `brand.json`.

## Tasks

### Task 1: Add font keys to brand.json

**Files:**
- Modify: `brand.json`

**Step 1: Update brand.json with font stacks**

```json
{
  "primary": "#1a1a2e",
  "accent": "#e94560",
  "surface": "#16213e",
  "text": "#eaeaea",
  "fontSans": "Inter, ui-sans-serif, system-ui, sans-serif",
  "fontMono": "JetBrains Mono, ui-monospace, monospace"
}
```

**Step 2: Verify the file is valid JSON**

Run: `node -e "require('./brand.json')"` from the project root.
Expected: No output (success). Any syntax error prints an exception.

### Task 2: Add semantic color tokens to brand.json

**Files:**
- Modify: `brand.json`

**Step 1: Add the semantic object to brand.json**

```json
{
  "primary": "#1a1a2e",
  "accent": "#e94560",
  "surface": "#16213e",
  "text": "#eaeaea",
  "fontSans": "Inter, ui-sans-serif, system-ui, sans-serif",
  "fontMono": "JetBrains Mono, ui-monospace, monospace",
  "semantic": {
    "success": {
      "light": { "bg": "#dcfce7", "text": "#166534" },
      "dark":  { "bg": "#052e16", "text": "#86efac" }
    },
    "warning": {
      "light": { "bg": "#ffedd5", "text": "#9a3412" },
      "dark":  { "bg": "#431407", "text": "#fdba74" }
    },
    "danger": {
      "light": { "bg": "#fee2e2", "text": "#991b1b" },
      "dark":  { "bg": "#450a0a", "text": "#fca5a5" }
    },
    "info": {
      "light": { "bg": "#f3e8ff", "text": "#6b21a8" },
      "dark":  { "bg": "#3b0764", "text": "#d8b4fe" }
    },
    "neutral": {
      "light": { "bg": "#fef9c3", "text": "#854d0e" },
      "dark":  { "bg": "#422006", "text": "#fde047" }
    }
  }
}
```

**Step 2: Verify the file is valid JSON**

Run: `node -e "const b = require('./brand.json'); console.log(Object.keys(b.semantic))"` from the project root.
Expected: `[ 'success', 'warning', 'danger', 'info', 'neutral' ]`

**Step 3: Commit**

`feat(brand): add font stacks and semantic color tokens to brand.json`

### Task 3: Wire fonts and semantic tokens into dashboard CSS

**Files:**
- Modify: `web/src/index.css`

**Step 1: Update the @theme block and add dark mode overrides**

Replace the entire contents of `web/src/index.css` with:

```css
@import "tailwindcss";

/* Dark mode: class strategy (add/remove "dark" on <html>) */
@custom-variant dark (&:where(.dark, .dark *));

/* Brand colors and fonts — must match brand.json at project root */
@theme {
  --font-sans: "Inter", "ui-sans-serif", "system-ui", "sans-serif";
  --font-mono: "JetBrains Mono", "ui-monospace", "monospace";

  --color-brand-primary: #1a1a2e;
  --color-brand-accent: #e94560;
  --color-brand-surface: #16213e;
  --color-brand-text: #eaeaea;

  /* Semantic tokens — light mode defaults */
  --color-semantic-success-bg: #dcfce7;
  --color-semantic-success-text: #166534;
  --color-semantic-warning-bg: #ffedd5;
  --color-semantic-warning-text: #9a3412;
  --color-semantic-danger-bg: #fee2e2;
  --color-semantic-danger-text: #991b1b;
  --color-semantic-info-bg: #f3e8ff;
  --color-semantic-info-text: #6b21a8;
  --color-semantic-neutral-bg: #fef9c3;
  --color-semantic-neutral-text: #854d0e;
}

/* Semantic tokens — dark mode overrides */
.dark {
  --color-semantic-success-bg: #052e16;
  --color-semantic-success-text: #86efac;
  --color-semantic-warning-bg: #431407;
  --color-semantic-warning-text: #fdba74;
  --color-semantic-danger-bg: #450a0a;
  --color-semantic-danger-text: #fca5a5;
  --color-semantic-info-bg: #3b0764;
  --color-semantic-info-text: #d8b4fe;
  --color-semantic-neutral-bg: #422006;
  --color-semantic-neutral-text: #fde047;
}
```

**Step 2: Verify Tailwind processes the file**

Run: `cd web && npx vite build --mode development 2>&1 | tail -5`
Expected: Build succeeds without CSS errors.

### Task 4: Wire fonts into Maizzle email tailwind.config.js

**Files:**
- Modify: `email/tailwind.config.js`

**Step 1: Add fontFamily to the email Tailwind config**

Replace the entire contents of `email/tailwind.config.js` with:

```js
import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const brand = require('../brand.json')

/** @type {import('tailwindcss').Config} */
export default {
  content: ['emails/**/*.html'],
  presets: [
    require('tailwindcss-preset-email'),
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          primary: brand.primary,
          accent: brand.accent,
          surface: brand.surface,
          text: brand.text,
        },
      },
      fontFamily: {
        sans: brand.fontSans.split(', '),
        mono: brand.fontMono.split(', '),
      },
    },
  },
}
```

**Step 2: Commit**

`feat(tokens): wire font and semantic tokens into dashboard CSS and email config`

### Task 5: Refactor StatusBadge to use semantic tokens

**Files:**
- Modify: `web/src/components/StatusBadge.tsx`

**Step 1: Replace the component with semantic token classes**

Replace the entire contents of `web/src/components/StatusBadge.tsx` with:

```tsx
export type NotificationStatus =
  | 'pending'
  | 'sending'
  | 'delivered'
  | 'failed'
  | 'not_sent'

export interface StatusBadgeProps {
  status: NotificationStatus
}

const styles: Record<NotificationStatus, string> = {
  pending:
    'bg-semantic-neutral-bg text-semantic-neutral-text',
  sending:
    'bg-semantic-info-bg text-semantic-info-text',
  delivered:
    'bg-semantic-success-bg text-semantic-success-text',
  failed:
    'bg-semantic-danger-bg text-semantic-danger-text',
  not_sent:
    'bg-semantic-warning-bg text-semantic-warning-text',
}

const labels: Record<NotificationStatus, string> = {
  pending: 'Pending',
  sending: 'Sending',
  delivered: 'Delivered',
  failed: 'Failed',
  not_sent: 'Not Sent',
}

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${styles[status]}`}
    >
      {labels[status]}
    </span>
  )
}
```

**Step 2: Verify the component builds**

Run: `cd web && npx vite build --mode development 2>&1 | tail -5`
Expected: Build succeeds.

**Step 3: Commit**

`feat(StatusBadge): use semantic color tokens for status differentiation`

### Task 6: Add button variants using semantic tokens

**Files:**
- Modify: `web/src/components/Button.tsx`

**Step 1: Replace the component with expanded variants**

Replace the entire contents of `web/src/components/Button.tsx` with:

```tsx
import { type ButtonHTMLAttributes } from 'react'

export type ButtonVariant =
  | 'primary'
  | 'secondary'
  | 'success'
  | 'warning'
  | 'info'
  | 'danger'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
}

export function Button({
  variant = 'primary',
  className = '',
  children,
  ...props
}: ButtonProps) {
  const base =
    'inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none'

  const variants: Record<ButtonVariant, string> = {
    primary:
      'bg-brand-accent text-white hover:bg-brand-accent/90 focus:ring-brand-accent',
    secondary:
      'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus:ring-brand-accent dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700',
    success:
      'bg-semantic-success-text text-white hover:opacity-90 focus:ring-semantic-success-text',
    warning:
      'bg-semantic-warning-text text-white hover:opacity-90 focus:ring-semantic-warning-text',
    info:
      'bg-semantic-info-text text-white hover:opacity-90 focus:ring-semantic-info-text',
    danger:
      'bg-semantic-danger-text text-white hover:opacity-90 focus:ring-semantic-danger-text',
  }

  return (
    <button
      className={`${base} ${variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
```

Note: Semantic button variants use the semantic `text` color (the
darker/more saturated value) as the background to ensure high contrast
with white text. This is intentional -- badges use the light `bg` color
with dark `text` for non-interactive display, while buttons use the
inverse for interactive affordance.

**Step 2: Update the component barrel export to include ButtonVariant**

In `web/src/components/index.ts`, add `ButtonVariant` to the Button export:

```ts
export { Button } from './Button'
export type { ButtonProps, ButtonVariant } from './Button'
```

**Step 3: Verify the component builds**

Run: `cd web && npx vite build --mode development 2>&1 | tail -5`
Expected: Build succeeds.

**Step 4: Commit**

`feat(Button): add success, warning, info, danger semantic variants`

### Task 7: Update Storybook stories

**Files:**
- Modify: `web/src/components/Button.stories.tsx`
- Modify: `web/src/components/StatusBadge.stories.tsx`

**Step 1: Update Button.stories.tsx with new variants**

Replace the entire contents of `web/src/components/Button.stories.tsx` with:

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { Button } from './Button'

const meta = {
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['primary', 'secondary', 'success', 'warning', 'info', 'danger'],
    },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Primary: Story = {
  args: { variant: 'primary', children: 'Send Notification' },
}

export const Secondary: Story = {
  args: { variant: 'secondary', children: 'Cancel' },
}

export const Success: Story = {
  args: { variant: 'success', children: 'Confirm' },
}

export const Warning: Story = {
  args: { variant: 'warning', children: 'Proceed' },
}

export const Info: Story = {
  args: { variant: 'info', children: 'Details' },
}

export const Danger: Story = {
  args: { variant: 'danger', children: 'Delete' },
}

export const Disabled: Story = {
  args: { variant: 'primary', children: 'Send Notification', disabled: true },
}
```

**Step 2: StatusBadge stories are already correct**

The existing StatusBadge stories already cover all five statuses.
No changes needed -- they will automatically render with the new
semantic token colors.

**Step 3: Verify Storybook builds**

Run: `cd web && npx storybook build 2>&1 | tail -10`
Expected: Build succeeds.

**Step 4: Commit**

`feat(stories): add button variant stories and update argTypes`

## Verification Checklist

- [ ] `brand.json` contains `fontSans`, `fontMono`, and `semantic` keys
- [ ] `node -e "require('./brand.json')"` succeeds
- [ ] `web/src/index.css` declares `--font-sans`, `--font-mono`, and all 10 semantic color properties
- [ ] `web/src/index.css` `.dark` block overrides all 10 semantic properties
- [ ] `email/tailwind.config.js` reads `fontSans` and `fontMono` from brand.json
- [ ] `StatusBadge.tsx` uses `bg-semantic-*` classes, not hardcoded color scale classes
- [ ] `StatusBadge.tsx` maps: pending=neutral, sending=info, delivered=success, failed=danger, not_sent=warning
- [ ] `Button.tsx` exports `ButtonVariant` type with 6 variants
- [ ] `Button.tsx` has variant styles for success, warning, info, danger
- [ ] `Button.stories.tsx` has stories: Primary, Secondary, Success, Warning, Info, Danger, Disabled
- [ ] `cd web && npx vite build` succeeds with no errors
- [ ] `cd web && npx storybook build` succeeds with no errors
- [ ] TypeScript compiles with no errors: `cd web && npx tsc -b`
