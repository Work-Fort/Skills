---
type: plan
step: "7"
title: "Frontend Foundation"
status: pending
assessment_status: in_progress
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "7"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-2-cli-and-database
  - step-3-notification-delivery
  - step-4-state-machine
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
---

# Step 7: Frontend Foundation

## Overview

Scaffold the React + TypeScript + Vite frontend, configure Tailwind CSS
with dark mode support, set up Storybook for isolated component
development, build the reusable UI component library, wire up Go embed
files for single-binary deployment, and add the SPA handler with dev
proxy support.

After this step:

- A React + TypeScript app exists in `web/` with Vite as the build
  tool and Tailwind CSS v4 for styling.
- Dark mode works via Tailwind's class strategy, with system preference
  detection and manual toggle via a `DarkModeToggle` component.
- Brand colors from `brand.json` are available as Tailwind utility
  classes (`bg-brand-primary`, `text-brand-accent`, etc.), shared with
  the email templates established in Step 3.
- Five reusable components exist: `Button`, `Pagination`,
  `DarkModeToggle`, `StatusBadge`, and `NotificationRow`.
- Each component has a Storybook story in CSF 3.0 format with
  `tags: ['autodocs']` and accessibility checks via `@storybook/addon-a11y`.
- The Go binary embeds the frontend via conditional build tags
  (`//go:build spa` / `//go:build !spa`).
- The SPA handler serves static files with `index.html` fallback for
  client-side routing and immutable cache headers for hashed assets.
- A dev proxy forwards requests to the Vite dev server during
  development.
- `mise run dev:storybook` starts Storybook on port 6006.

## Prerequisites

- Step 6 completed: MCP tools, MCP bridge, WebSocket hub and endpoint,
  worker broadcasting state transitions
- Go 1.26.0 (pinned in `mise.toml`)
- Node 22 (pinned in `mise.toml`)
- `mise` CLI available on PATH
- `brand.json` exists at project root (created in Step 3)

## New Dependencies

### NPM (in `web/`)

| Package | Version | Purpose |
|---------|---------|---------|
| `react` | ^19 | UI library |
| `react-dom` | ^19 | React DOM renderer |
| `vite` | ^8 | Build tool and dev server |
| `@vitejs/plugin-react` | ^6 | React fast refresh and JSX transform |
| `tailwindcss` | ^4 | CSS framework |
| `@tailwindcss/vite` | ^4 | Tailwind CSS Vite plugin |
| `typescript` | ^5 | Type checking |
| `@types/react` | ^19 | React type definitions |
| `@types/react-dom` | ^19 | React DOM type definitions |
| `storybook` | ^10 | Storybook CLI and core |
| `@storybook/react-vite` | ^10 | Storybook React + Vite framework |
| `@storybook/addon-a11y` | ^10 | Accessibility testing addon (axe-core) |

Note: `npm install` resolves exact versions. The caret ranges above
are targets -- npm may select a compatible patch version. Run
`npm install` during Task 1 to fetch all dependencies.

### Go

No new Go dependencies. The `embed`, `io/fs`, `net/http`,
`net/http/httputil`, and `net/url` packages are all in the standard
library.

## Spec Traceability

All tasks trace to `openspec/specs/frontend-dashboard/spec.md`:

| Task | Spec Requirements |
|------|-------------------|
| Task 1: Vite scaffold | REQ-001 |
| Task 2: Tailwind + dark mode | REQ-016, REQ-017, REQ-024 |
| Task 3: Storybook setup | REQ-019, REQ-020, REQ-021, REQ-022 |
| Task 4: Button component | REQ-023 |
| Task 5: StatusBadge component | REQ-023 |
| Task 6: Pagination component | REQ-023 |
| Task 7: DarkModeToggle component | REQ-018, REQ-023 |
| Task 8: NotificationRow component | REQ-023 |
| Task 9: Go embed files | REQ-002, REQ-006 |
| Task 10: SPA handler | REQ-003, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011 |
| Task 11: Dev proxy | REQ-004, REQ-005 |
| Task 12: Daemon wiring | REQ-004, REQ-011 |

## Tasks

### Task 1: Vite + React + TypeScript Scaffold

Satisfies: REQ-001 (React + TypeScript application built with Vite,
source in `web/`, build output in `web/dist/`).

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/tsconfig.app.json`
- Create: `web/tsconfig.node.json`
- Create: `web/vite.config.ts`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/vite-env.d.ts`
- Delete: `web/.gitkeep`

**Step 1: Create `web/package.json`**

```json
{
  "name": "notifier-dashboard",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "lint": "tsc -b",
    "preview": "vite preview",
    "storybook": "storybook dev",
    "build-storybook": "storybook build"
  },
  "dependencies": {
    "react": "^19.1.0",
    "react-dom": "^19.1.0"
  },
  "devDependencies": {
    "@storybook/addon-a11y": "^10.3.5",
    "@storybook/react-vite": "^10.3.5",
    "@tailwindcss/vite": "^4.2.2",
    "@types/react": "^19.1.2",
    "@types/react-dom": "^19.1.2",
    "@vitejs/plugin-react": "^6.0.1",
    "storybook": "^10.3.5",
    "tailwindcss": "^4.2.2",
    "typescript": "^5.8.3",
    "vite": "^8.0.8"
  }
}
```

**Step 2: Create `web/tsconfig.json`**

```json
{
  "files": [],
  "references": [
    { "path": "./tsconfig.app.json" },
    { "path": "./tsconfig.node.json" }
  ]
}
```

**Step 3: Create `web/tsconfig.app.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedSideEffectImports": true,
    "resolveJsonModule": true
  },
  "include": ["src"]
}
```

**Step 4: Create `web/tsconfig.node.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2023"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedSideEffectImports": true
  },
  "include": ["vite.config.ts"]
}
```

**Step 5: Create `web/vite.config.ts`**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
```

**Step 6: Create `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Notifier Dashboard</title>
  </head>
  <body class="bg-white text-gray-900 dark:bg-gray-950 dark:text-gray-100">
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

**Step 7: Create `web/src/vite-env.d.ts`**

```typescript
/// <reference types="vite/client" />
```

**Step 8: Create `web/src/main.tsx`**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

**Step 9: Create `web/src/App.tsx`**

```tsx
function App() {
  return (
    <div className="min-h-screen p-8">
      <h1 className="text-2xl font-bold text-brand-primary dark:text-brand-text">
        Notifier Dashboard
      </h1>
      <p className="mt-2 text-gray-600 dark:text-gray-400">
        Frontend foundation — components will be added here.
      </p>
    </div>
  )
}

export default App
```

**Step 10: Delete `web/.gitkeep` and run `npm install`**

Run: `rm web/.gitkeep && cd web && npm install`
Expected: `node_modules/` created, `package-lock.json` generated.

**Step 11: Verify the dev server starts**

Run: `cd web && npx vite --host 127.0.0.1 --port 5173 &` then
`sleep 3 && curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:5173/ && kill %1`
Expected: `200`

**Step 12: Verify the production build succeeds**

Run: `cd web && npx tsc -b && npx vite build`
Expected: Output shows `dist/index.html` and `dist/assets/` with
hashed `.js` and `.css` files.

**Step 13: Commit**

`feat(web): scaffold React + TypeScript + Vite frontend`

---

### Task 2: Tailwind CSS with Dark Mode and Brand Colors

Satisfies: REQ-016 (Tailwind CSS), REQ-017 (dark mode via class
strategy), REQ-024 (brand colors from `brand.json`).

**Depends on:** Task 1 (Vite scaffold)

**Files:**
- Create: `web/src/index.css`
- Create: `web/src/brand.ts`

**Step 1: Create `web/src/index.css`**

Tailwind CSS v4 uses CSS-first configuration. The `@theme` directive
defines custom design tokens. The `@custom-variant` directive defines
a custom dark variant that uses the `class` strategy instead of
`prefers-color-scheme`.

Brand colors are hardcoded in the CSS to match `brand.json`. This is a
deliberate trade-off: Tailwind v4's CSS-first config cannot import JSON
at build time without a custom plugin, and the brand palette changes
rarely enough that manual synchronization is acceptable. Both this file
and `email/tailwind.config.js` reference the same values from
`brand.json`.

```css
@import "tailwindcss";

/* Dark mode: class strategy (add/remove "dark" on <html>) */
@custom-variant dark (&:where(.dark, .dark *));

/* Brand colors — must match brand.json at project root */
@theme {
  --color-brand-primary: #1a1a2e;
  --color-brand-accent: #e94560;
  --color-brand-surface: #16213e;
  --color-brand-text: #eaeaea;
}
```

This gives Tailwind utilities like `bg-brand-primary`,
`text-brand-accent`, `border-brand-surface`, and `text-brand-text`.

**Step 2: Create `web/src/brand.ts`**

A TypeScript constant that mirrors `brand.json` for use in components
that need programmatic access to brand values (e.g., chart colors,
dynamic styles). Components should prefer Tailwind utility classes;
this file exists for the rare cases where a raw hex value is needed.

```typescript
/**
 * Brand color palette. Must stay in sync with:
 * - brand.json (project root — single source of truth)
 * - web/src/index.css (@theme block)
 * - email/tailwind.config.js (Maizzle email build)
 */
export const brand = {
  primary: '#1a1a2e',
  accent: '#e94560',
  surface: '#16213e',
  text: '#eaeaea',
} as const
```

**Step 3: Verify Tailwind classes work in the dev build**

Run: `cd web && npx vite build`
Expected: Build succeeds. The output CSS in `dist/assets/` contains
the brand color values (`#1a1a2e`, `#e94560`, etc.) and dark mode
selectors (`.dark`).

**Step 4: Commit**

`feat(web): add Tailwind CSS v4 with dark mode and brand colors`

---

### Task 3: Storybook Setup

Satisfies: REQ-019 (`@storybook/react-vite`), REQ-020 (CSF 3.0 with
autodocs), REQ-021 (dark mode decorator), REQ-022 (`@storybook/addon-a11y`).

**Depends on:** Task 2 (Tailwind CSS)

**Files:**
- Create: `web/.storybook/main.ts`
- Create: `web/.storybook/preview.tsx`

**Step 1: Create `web/.storybook/main.ts`**

```typescript
import type { StorybookConfig } from '@storybook/react-vite'

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  framework: '@storybook/react-vite',
  addons: [
    '@storybook/addon-a11y',
  ],
}

export default config
```

**Step 2: Create `web/.storybook/preview.tsx`**

The decorator toggles the `dark` class on `<html>` based on a global
toolbar control. This lets developers preview every component in both
light and dark mode from within Storybook.

```tsx
import type { Preview } from '@storybook/react'
import '../src/index.css'

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Toggle light/dark mode',
      toolbar: {
        title: 'Theme',
        items: ['light', 'dark'],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    theme: 'light',
  },
  decorators: [
    (Story, context) => {
      const theme = context.globals.theme ?? 'light'
      document.documentElement.classList.toggle('dark', theme === 'dark')
      return <Story />
    },
  ],
}

export default preview
```

**Step 3: Verify Storybook starts**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts on port 6006 with no errors. The `--ci`
flag prevents the browser from opening and exits after startup check.

**Step 4: Commit**

`feat(web): add Storybook with dark mode decorator and a11y addon`

---

### Task 4: Button Component and Story

Satisfies: REQ-023 (reusable components with individual story files).

**Depends on:** Task 3 (Storybook setup)

**Files:**
- Create: `web/src/components/Button.tsx`
- Create: `web/src/components/Button.stories.tsx`

**Step 1: Create `web/src/components/Button.tsx`**

```tsx
import { type ButtonHTMLAttributes } from 'react'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary'
}

export function Button({
  variant = 'primary',
  className = '',
  children,
  ...props
}: ButtonProps) {
  const base =
    'inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none'

  const variants: Record<string, string> = {
    primary:
      'bg-brand-accent text-white hover:bg-brand-accent/90 focus:ring-brand-accent',
    secondary:
      'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus:ring-brand-accent dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700',
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

**Step 2: Create `web/src/components/Button.stories.tsx`**

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { Button } from './Button'

const meta = {
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['primary', 'secondary'] },
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

export const Disabled: Story = {
  args: { variant: 'primary', children: 'Send Notification', disabled: true },
}
```

**Step 3: Verify the story renders in Storybook**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts. The Button story appears under Components
with Primary, Secondary, and Disabled variants. The Accessibility panel
shows no violations.

**Step 4: Commit**

`feat(web): add Button component with Storybook story`

---

### Task 5: StatusBadge Component and Story

Satisfies: REQ-023 (reusable components with individual story files).

**Depends on:** Task 3 (Storybook setup)

**Files:**
- Create: `web/src/components/StatusBadge.tsx`
- Create: `web/src/components/StatusBadge.stories.tsx`

**Step 1: Create `web/src/components/StatusBadge.tsx`**

Displays the notification state as a colored badge. Colors match the
notification state semantics: green for delivered, red for failed,
yellow for pending/sending, gray for not_sent.

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
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  sending:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  delivered:
    'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  failed:
    'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  not_sent:
    'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
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

**Step 2: Create `web/src/components/StatusBadge.stories.tsx`**

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { StatusBadge } from './StatusBadge'

const meta = {
  component: StatusBadge,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['pending', 'sending', 'delivered', 'failed', 'not_sent'],
    },
  },
} satisfies Meta<typeof StatusBadge>

export default meta
type Story = StoryObj<typeof meta>

export const Pending: Story = {
  args: { status: 'pending' },
}

export const Sending: Story = {
  args: { status: 'sending' },
}

export const Delivered: Story = {
  args: { status: 'delivered' },
}

export const Failed: Story = {
  args: { status: 'failed' },
}

export const NotSent: Story = {
  args: { status: 'not_sent' },
}
```

**Step 3: Verify the story renders in Storybook**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts. The StatusBadge story shows all five
status variants with correct colors. The Accessibility panel shows
no violations.

**Step 4: Commit**

`feat(web): add StatusBadge component with Storybook story`

---

### Task 6: Pagination Component and Story

Satisfies: REQ-023 (reusable components with individual story files).

**Depends on:** Task 4 (Button component)

**Files:**
- Create: `web/src/components/Pagination.tsx`
- Create: `web/src/components/Pagination.stories.tsx`

**Step 1: Create `web/src/components/Pagination.tsx`**

Cursor-based pagination control. Shows "Previous" and "Next" buttons.
The component does not manage pagination state -- it receives callbacks
and disabled flags from the parent.

```tsx
import { Button } from './Button'

export interface PaginationProps {
  /** Called when the user clicks "Previous". */
  onPrevious: () => void
  /** Called when the user clicks "Next". */
  onNext: () => void
  /** Disable the "Previous" button (e.g., on the first page). */
  hasPrevious: boolean
  /** Disable the "Next" button (e.g., on the last page). */
  hasNext: boolean
}

export function Pagination({
  onPrevious,
  onNext,
  hasPrevious,
  hasNext,
}: PaginationProps) {
  return (
    <nav
      className="flex items-center justify-between border-t border-gray-200 px-4 py-3 dark:border-gray-700"
      aria-label="Pagination"
    >
      <Button
        variant="secondary"
        onClick={onPrevious}
        disabled={!hasPrevious}
      >
        Previous
      </Button>
      <Button
        variant="secondary"
        onClick={onNext}
        disabled={!hasNext}
      >
        Next
      </Button>
    </nav>
  )
}
```

**Step 2: Create `web/src/components/Pagination.stories.tsx`**

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { Pagination } from './Pagination'

const meta = {
  component: Pagination,
  tags: ['autodocs'],
  args: {
    onPrevious: fn(),
    onNext: fn(),
  },
} satisfies Meta<typeof Pagination>

export default meta
type Story = StoryObj<typeof meta>

export const BothEnabled: Story = {
  args: { hasPrevious: true, hasNext: true },
}

export const FirstPage: Story = {
  args: { hasPrevious: false, hasNext: true },
}

export const LastPage: Story = {
  args: { hasPrevious: true, hasNext: false },
}

export const SinglePage: Story = {
  args: { hasPrevious: false, hasNext: false },
}
```

**Step 3: Verify the story renders in Storybook**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts. The Pagination story shows all four
variants. Previous/Next buttons are correctly enabled/disabled. The
Accessibility panel shows no violations (the `nav` landmark with
`aria-label` satisfies a11y rules).

**Step 4: Commit**

`feat(web): add Pagination component with Storybook story`

---

### Task 7: DarkModeToggle Component and Story

Satisfies: REQ-018 (dark mode switcher with system preference
detection and manual toggle), REQ-023 (reusable components with
individual story files).

**Depends on:** Task 3 (Storybook setup)

**Files:**
- Create: `web/src/hooks/useDarkMode.ts`
- Create: `web/src/components/DarkModeToggle.tsx`
- Create: `web/src/components/DarkModeToggle.stories.tsx`

**Step 1: Create `web/src/hooks/useDarkMode.ts`**

Detects system preference via `prefers-color-scheme`, persists the
user's manual choice to `localStorage`, and toggles the `dark` class
on `<html>`. On first load, if no manual choice is stored, the system
preference is used.

```typescript
import { useEffect, useState } from 'react'

type Theme = 'light' | 'dark'

const STORAGE_KEY = 'theme'
const SOURCE_KEY = 'theme-source'

function getSystemTheme(): Theme {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

function getInitialTheme(): Theme {
  if (typeof window === 'undefined') return 'light'
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  return getSystemTheme()
}

export function useDarkMode(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  // Apply the theme to the DOM and persist if user-chosen.
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
  }, [theme])

  // Follow system preference changes when the user has not explicitly
  // toggled. The SOURCE_KEY flag distinguishes "user chose this" from
  // "system preference was detected".
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = (e: MediaQueryListEvent) => {
      if (localStorage.getItem(SOURCE_KEY) !== 'user') {
        setTheme(e.matches ? 'dark' : 'light')
      }
    }
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

  const toggle = () => {
    setTheme((prev) => {
      const next = prev === 'dark' ? 'light' : 'dark'
      localStorage.setItem(STORAGE_KEY, next)
      localStorage.setItem(SOURCE_KEY, 'user')
      return next
    })
  }

  return [theme, toggle]
}
```

**Step 2: Create `web/src/components/DarkModeToggle.tsx`**

```tsx
import { useDarkMode } from '../hooks/useDarkMode'

export function DarkModeToggle() {
  const [theme, toggle] = useDarkMode()

  return (
    <button
      onClick={toggle}
      className="rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
      aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
    >
      {theme === 'dark' ? (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 20 20"
          fill="currentColor"
          className="h-5 w-5"
          aria-hidden="true"
        >
          <path
            fillRule="evenodd"
            d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z"
            clipRule="evenodd"
          />
        </svg>
      ) : (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 20 20"
          fill="currentColor"
          className="h-5 w-5"
          aria-hidden="true"
        >
          <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
        </svg>
      )}
    </button>
  )
}
```

**Step 3: Create `web/src/components/DarkModeToggle.stories.tsx`**

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { DarkModeToggle } from './DarkModeToggle'

const meta = {
  component: DarkModeToggle,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div className="flex items-center gap-4 p-4">
        <span className="text-sm text-gray-500 dark:text-gray-400">
          Click the toggle to switch themes:
        </span>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof DarkModeToggle>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
```

**Step 4: Verify the toggle works in Storybook**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts. The DarkModeToggle story renders.
Clicking the button toggles the `dark` class on `<html>` and switches
the icon between sun (light mode) and moon (dark mode). The
Accessibility panel shows no violations (the button has `aria-label`).

**Step 5: Commit**

`feat(web): add DarkModeToggle component with system preference detection`

---

### Task 8: NotificationRow Component and Story

Satisfies: REQ-023 (reusable components with individual story files).

**Depends on:** Task 4 (Button), Task 5 (StatusBadge)

**Files:**
- Create: `web/src/components/NotificationRow.tsx`
- Create: `web/src/components/NotificationRow.stories.tsx`

**Step 1: Create `web/src/components/NotificationRow.tsx`**

A table row displaying a single notification. Used inside the
notifications table that will be built in Step 8 (Dashboard). The
component renders the notification ID, email, status badge, retry
count/limit, and an action slot for the resend button.

```tsx
import { StatusBadge, type NotificationStatus } from './StatusBadge'
import { Button } from './Button'

export interface Notification {
  id: string
  email: string
  status: NotificationStatus
  retry_count: number
  retry_limit: number
}

export interface NotificationRowProps {
  notification: Notification
  /** Called when the user clicks "Resend". Only shown for failed/not_sent states. */
  onResend?: (id: string) => void
}

// Resend is only available for notifications that did not succeed.
// Delivered notifications are terminal but do not need resending.
const resendableStates: NotificationStatus[] = ['failed', 'not_sent']

export function NotificationRow({ notification, onResend }: NotificationRowProps) {
  const { id, email, status, retry_count, retry_limit } = notification
  const showResend = onResend && resendableStates.includes(status)

  return (
    <tr className="border-b border-gray-200 dark:border-gray-700">
      <td className="whitespace-nowrap px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">
        {id}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm">
        {email}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <StatusBadge status={status} />
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
        {retry_count} / {retry_limit}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        {showResend && (
          <Button variant="secondary" onClick={() => onResend(id)}>
            Resend
          </Button>
        )}
      </td>
    </tr>
  )
}
```

**Step 2: Create `web/src/components/NotificationRow.stories.tsx`**

```tsx
import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { NotificationRow } from './NotificationRow'

const meta = {
  component: NotificationRow,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <table className="min-w-full">
        <tbody>
          <Story />
        </tbody>
      </table>
    ),
  ],
  args: {
    onResend: fn(),
  },
} satisfies Meta<typeof NotificationRow>

export default meta
type Story = StoryObj<typeof meta>

export const Delivered: Story = {
  args: {
    notification: {
      id: 'ntf_abc123',
      email: 'user@example.com',
      status: 'delivered',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const Failed: Story = {
  args: {
    notification: {
      id: 'ntf_def456',
      email: 'bounce@example.com',
      status: 'failed',
      retry_count: 3,
      retry_limit: 3,
    },
  },
}

export const Pending: Story = {
  args: {
    notification: {
      id: 'ntf_ghi789',
      email: 'new@company.com',
      status: 'pending',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const Sending: Story = {
  args: {
    notification: {
      id: 'ntf_jkl012',
      email: 'sending@company.com',
      status: 'sending',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const NotSent: Story = {
  args: {
    notification: {
      id: 'ntf_mno345',
      email: 'retry@company.com',
      status: 'not_sent',
      retry_count: 1,
      retry_limit: 3,
    },
  },
}
```

**Step 3: Verify the story renders in Storybook**

Run: `cd web && npx storybook dev --port 6006 --ci`
Expected: Storybook starts. The NotificationRow story shows all five
notification states. The "Resend" button appears only for Failed and
NotSent stories. The Accessibility panel shows no violations.

**Step 4: Commit**

`feat(web): add NotificationRow component with Storybook story`

---

### Task 9: Go Embed Files

Satisfies: REQ-002 (two embed files with build tags), REQ-006
(`fs.Sub` to strip prefix).

**Depends on:** Task 1 (Vite scaffold produces `web/dist/`)

**Files:**
- Create: `embed.go`
- Create: `embed_spa.go`

Go's `//go:embed` directive does not allow `..` in paths, so embed
files cannot live in `cmd/daemon/` and reference `web/dist/`. Instead,
the embed files live at the project root (package `main`), where
`web/dist` is a direct child path. The `WebFS` variable is exported
so `cmd/daemon` can import it via the root package.

However, since `cmd/daemon` imports `package main` is not possible
(circular), we use a different approach: the embed files live at the
project root as `package main`, and `main.go` passes `WebFS` to the
CLI layer, which passes it to the daemon. This follows the same
pattern as `Version` being set in `main.go` and passed to `cli.Version`.

**Step 1: Create `embed.go` at the project root**

This file compiles when the `spa` build tag is NOT set (i.e., during
development). The embedded filesystem is empty, so the SPA handler
serves nothing -- the dev proxy handles requests instead.

```go
//go:build !spa

package main

import "embed"

// webFS is empty when built without the "spa" tag.
// Use --dev to proxy to Vite's dev server during development.
var webFS embed.FS
```

**Step 2: Create `embed_spa.go` at the project root**

This file compiles when the `spa` build tag IS set (production and QA
builds). It embeds the entire `web/dist/` directory. The `all:` prefix
includes dotfiles and files normally excluded by Go's embed rules.

```go
//go:build spa

package main

import "embed"

// webFS holds the Vite build output. Built via:
//   mise run build:web && go build -tags spa
//
//go:embed all:web/dist
var webFS embed.FS
```

**Step 3: Pass `webFS` through the CLI layer to daemon**

Modify `main.go` to pass `webFS` to the CLI:

```go
package main

import "github.com/workfort/notifier/internal/cli"

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	cli.Version = Version
	cli.WebFS = webFS
	cli.Execute()
}
```

Modify `internal/cli/root.go` to accept and forward `WebFS`:

Add a package-level variable:

```go
// WebFS holds the embedded frontend filesystem, set from main.go.
var WebFS embed.FS
```

Add the `embed` import. Then pass it to the daemon command:

```go
cmd.AddCommand(daemon.NewCmd(WebFS))
```

Modify `cmd/daemon/daemon.go` `NewCmd` to accept `embed.FS`:

```go
func NewCmd(webFS embed.FS) *cobra.Command {
```

Store `webFS` in a package-level variable or closure so `run` and
`RunServer` can access it. The simplest approach is a closure:

```go
func NewCmd(frontendFS embed.FS) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithFS(cmd, args, frontendFS)
		},
	}
	// ... flags ...
	return cmd
}
```

Then rename `run` to `runWithFS` with the additional parameter, and
pass `frontendFS` into `ServerConfig`.

**Step 4: Verify the dev build compiles (no spa tag)**

Run: `go build ./...`
Expected: Build succeeds. The `webFS` variable is an empty
`embed.FS`.

**Step 5: Verify the spa build compiles after building the frontend**

Run: `cd web && npm run build && cd .. && go build -tags spa ./...`
Expected: Build succeeds. The `webFS` variable contains the
`web/dist/` tree.

**Step 6: Commit**

`feat(web): add go:embed files with spa/!spa build tags`

---

### Task 10: SPA Handler

Satisfies: REQ-003 (serve SPA from embedded filesystem), REQ-007
(serve static files), REQ-008 (index.html fallback), REQ-009
(immutable cache headers for `assets/`), REQ-010 (no immutable cache
on `index.html`), REQ-011 (API routes take priority).

**Files:**
- Create: `internal/infra/httpapi/spa.go`
- Test: `internal/infra/httpapi/spa_test.go`

**Step 1: Write the failing test**

Create `internal/infra/httpapi/spa_test.go`:

```go
package httpapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestNewSPAHandler_ServesIndexHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html content, got %q", body)
	}
}

func TestNewSPAHandler_FallbackToIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/notifications/ntf_abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html fallback, got %q", body)
	}
}

func TestNewSPAHandler_ServesStaticFile(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             {Data: []byte("<html>app</html>")},
		"assets/index-abc123.js": {Data: []byte("console.log('app')")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "console.log('app')" {
		t.Fatalf("expected JS content, got %q", body)
	}
}

func TestNewSPAHandler_ImmutableCacheForAssets(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             {Data: []byte("<html>app</html>")},
		"assets/index-abc123.js": {Data: []byte("console.log('app')")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	expected := "public, max-age=31536000, immutable"
	if cc != expected {
		t.Fatalf("expected Cache-Control %q, got %q", expected, cc)
	}
}

func TestNewSPAHandler_NoCacheForIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc == "public, max-age=31536000, immutable" {
		t.Fatal("index.html must not receive immutable cache headers")
	}
}

func TestNewSPAHandler_FallbackNoCacheHeaders(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/notifications/ntf_abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html fallback, got %q", body)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "public, max-age=31536000, immutable" {
		t.Fatal("fallback to index.html must not receive immutable cache headers")
	}
}

// Ensure the function signature accepts fs.FS so it works with
// the output of fs.Sub(webFS, "dist").
var _ = func(fsys fs.FS) http.Handler {
	return NewSPAHandler(fsys)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestNewSPAHandler ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: NewSPAHandler"

**Step 3: Write the SPA handler implementation**

Create `internal/infra/httpapi/spa.go`:

```go
package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// NewSPAHandler serves static files from the given filesystem. If a
// requested path does not match a real file, it falls back to
// index.html for client-side routing. Files under assets/ receive
// immutable cache headers since Vite content-hashes their filenames.
func NewSPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}

		// Serve the file if it exists.
		if _, err := fs.Stat(fsys, path); err == nil {
			// Hashed assets get long-lived cache headers.
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback: serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestNewSPAHandler ./internal/infra/httpapi/...`
Expected: PASS (all six tests)

**Step 5: Commit**

`feat(httpapi): add SPA handler with index.html fallback and cache headers`

---

### Task 11: Dev Proxy

Satisfies: REQ-004 (dev mode proxies to Vite), REQ-005
(`NewSPADevProxy` function with `httputil.NewSingleHostReverseProxy`).

**Files:**
- Create: `internal/infra/httpapi/devproxy.go`
- Test: `internal/infra/httpapi/devproxy_test.go`

**Step 1: Write the failing test**

Create `internal/infra/httpapi/devproxy_test.go`:

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSPADevProxy_ForwardsRequest(t *testing.T) {
	// Start a fake Vite dev server.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vite-dev-server"))
	}))
	defer backend.Close()

	proxy := NewSPADevProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "vite-dev-server" {
		t.Fatalf("expected proxied content, got %q", body)
	}
}

func TestNewSPADevProxy_PreservesPath(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewSPADevProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/src/main.tsx", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if gotPath != "/src/main.tsx" {
		t.Fatalf("expected path /src/main.tsx, got %q", gotPath)
	}
}

func TestNewSPADevProxy_PanicsOnInvalidURL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid URL, got none")
		}
	}()
	NewSPADevProxy("://not-a-url")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestNewSPADevProxy ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: NewSPADevProxy"

**Step 3: Write the dev proxy implementation**

Create `internal/infra/httpapi/devproxy.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewSPADevProxy returns an HTTP handler that reverse-proxies all
// requests to the given development server URL (typically Vite on
// http://localhost:5173). Used during development for hot reload.
//
// Panics if devURL is not a valid URL. This follows the MustCompile
// convention — the URL is known at startup, so a parse failure is a
// programmer error.
func NewSPADevProxy(devURL string) http.Handler {
	target, err := url.Parse(devURL)
	if err != nil {
		panic(fmt.Sprintf("httpapi: invalid dev proxy URL %q: %v", devURL, err))
	}
	return httputil.NewSingleHostReverseProxy(target)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestNewSPADevProxy ./internal/infra/httpapi/...`
Expected: PASS (all three tests)

**Step 5: Commit**

`feat(httpapi): add dev proxy to Vite dev server`

---

### Task 12: Daemon Wiring (--dev flag, SPA handler, route priority)

Satisfies: REQ-004 (`--dev` flag), REQ-011 (API routes before SPA
catch-all).

**Depends on:** Task 9 (embed files), Task 10 (SPA handler), Task 11
(dev proxy)

**Files:**
- Modify: `cmd/daemon/daemon.go:30-43` (add --dev and --dev-url flags)
- Modify: `cmd/daemon/daemon.go:78-88` (add fields to ServerConfig)
- Modify: `cmd/daemon/daemon.go:92-181` (wire SPA handler into server)
- Modify: `cmd/daemon/daemon_test.go:47` (update ServerConfig for new fields)

**Step 1: Update `NewCmd` to accept `embed.FS` and add flags**

Task 9 already updated `NewCmd` to accept `frontendFS embed.FS` and
changed `run` to `runWithFS`. Now add `--dev` and `--dev-url` flags
to the command:

```go
cmd.Flags().Bool("dev", false, "Enable dev mode (proxy to Vite dev server)")
cmd.Flags().String("dev-url", "http://localhost:5173", "Vite dev server URL")
```

**Step 2: Add flag resolution in `runWithFS`**

After the existing `smtpFrom` resolution, add:

```go
dev, _ := cmd.Flags().GetBool("dev")
devURL := resolveString(cmd, "dev-url")
```

**Step 3: Extend `ServerConfig` with frontend fields**

Add three fields to `ServerConfig`:

```go
type ServerConfig struct {
	Bind       string
	Port       int
	DSN        string
	SMTPHost   string
	SMTPPort   int
	SMTPFrom   string
	Version    string
	Dev        bool
	DevURL     string
	FrontendFS embed.FS
}
```

Add the `embed` import at the top of the file.

Pass the new fields from `runWithFS`:

```go
return RunServer(ctx, ServerConfig{
	Bind:       bind,
	Port:       port,
	DSN:        dsn,
	SMTPHost:   smtpHost,
	SMTPPort:   smtpPort,
	SMTPFrom:   smtpFrom,
	Version:    version,
	Dev:        dev,
	DevURL:     devURL,
	FrontendFS: frontendFS,
})
```

**Step 4: Wire the SPA handler into `RunServer`**

In the `RunServer` function, after the MCP handler creation and
before the `mux` setup, add the SPA handler selection.

**Spec delta:** REQ-006 specifies `fs.Sub(webFS, "dist")`, which
assumed the embed file would live in `cmd/web/` where the embedded
`web/dist` directory would appear as just `dist`. Since the embed
file lives at the project root (required because `//go:embed` does
not allow `..` paths), the embedded tree contains `web/dist/...`, so
the correct sub-path is `"web/dist"`. This is a non-functional
deviation from the spec wording that should be recorded for
traceability. The spec should be updated to reflect `"web/dist"`.

When built without the `spa` tag, `cfg.FrontendFS` is a zero-value
`embed.FS` (empty filesystem). The `fs.Sub` call succeeds on an
empty FS (it defers failure to file-access time), and the SPA handler
returns 404s for all requests. This is acceptable for non-spa builds
because the `--dev` flag should be used instead. However, if neither
`spa` tag nor `--dev` is set, the handler silently serves nothing.
Add a log warning for this case so operators know why the UI is blank.

```go
// Select SPA handler: dev proxy or embedded filesystem.
var spaHandler http.Handler
if cfg.Dev {
	spaHandler = httpapi.NewSPADevProxy(cfg.DevURL)
	slog.Info("dev mode: proxying to Vite", "url", cfg.DevURL)
} else {
	// Spec delta: REQ-006 says fs.Sub(webFS, "dist") but the embed
	// lives at the project root, so the path is "web/dist".
	distFS, err := fs.Sub(cfg.FrontendFS, "web/dist")
	if err != nil {
		return fmt.Errorf("embedded SPA: %w", err)
	}

	// Warn if the embedded FS is empty (built without -tags spa).
	if _, readErr := fs.Stat(distFS, "index.html"); readErr != nil {
		slog.Warn("no embedded frontend found; built without -tags spa?",
			"hint", "use --dev to proxy to Vite, or rebuild with -tags spa")
	}

	spaHandler = httpapi.NewSPAHandler(distFS)
}
```

Add the required import at the top of the file:

```go
"io/fs"
```

(The `httpapi` import already exists.)

**Step 5: Register the SPA catch-all AFTER all API routes**

In the mux setup, add the SPA handler as the last route:

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))
mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx))
mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

// SPA catch-all — must be last so API routes take priority.
mux.Handle("/", spaHandler)
```

**Step 6: Update `daemon_test.go` for new `ServerConfig` fields**

The existing `daemon_test.go` calls `RunServer(ctx, ServerConfig{...})`
without the new `Dev`, `DevURL`, and `FrontendFS` fields. While Go's
zero-value semantics mean it compiles, the `RunServer` function now
calls `fs.Sub(cfg.FrontendFS, "web/dist")` when `cfg.Dev` is false,
and a zero-value `embed.FS` produces an empty sub-filesystem. Since
the existing test hits `/v1/health` (not the SPA catch-all), the
empty SPA handler does not cause a failure. However, to make the test
explicit about its configuration and prevent future test fragility,
set `Dev: true` in the test's `ServerConfig`:

```go
RunServer(ctx, ServerConfig{
	// ... existing fields ...
	Dev:    true,
	DevURL: "http://localhost:5173",
})
```

This makes the test independent of the embedded frontend: it uses
dev-proxy mode, which does not require an `embed.FS` with content.
The dev proxy target is irrelevant because the test only hits
`/v1/health`, not the SPA catch-all route.

**Step 7: Verify the build succeeds (dev mode, no spa tag)**

Run: `go build ./...`
Expected: Build succeeds. The empty `embed.FS` from `embed.go` is
used. When `--dev` is not set, `RunServer` logs a warning about the
missing embedded frontend but does not fail.

**Step 8: Verify the build succeeds (spa tag, after frontend build)**

Run: `cd web && npm run build && cd .. && go build -tags spa ./...`
Expected: Build succeeds with the embedded frontend.

**Step 9: Verify the QA build succeeds**

Run: `cd web && npm run build && cd .. && go build -tags spa,qa ./...`
Expected: Build succeeds.

**Step 10: Verify existing tests still pass**

Run: `go test ./cmd/daemon/...`
Expected: PASS. The updated `ServerConfig` with `Dev: true` avoids
the empty `embed.FS` path entirely.

**Step 11: Commit**

`feat(daemon): wire SPA handler with --dev flag and route priority`

---

### Task 13: Mise Tasks for Frontend

Satisfies: Operational tooling for the frontend development workflow.

The `build:web`, `dev:web`, and `dev:storybook` tasks already exist
from previous steps and are compatible with this plan's frontend
setup: `build:web` runs `cd web && npm run build`, `dev:web` runs
`cd web && npm run dev`, and `dev:storybook` runs
`cd web && npm run storybook -- --port 6006`. All three match the
`package.json` scripts defined in Task 1. This task adds the missing
`lint:web` and `clean:web` tasks and updates the `ci` task to include
frontend linting.

**Files:**
- Create: `.mise/tasks/lint/web`
- Create: `.mise/tasks/clean/web`
- Modify: `.mise/tasks/ci`

**Step 1: Create `.mise/tasks/lint/web`**

```bash
#!/usr/bin/env bash
#MISE description="Type-check the frontend (tsc)"
set -euo pipefail

cd web
npx tsc -b
```

**Step 2: Create `.mise/tasks/clean/web`**

```bash
#!/usr/bin/env bash
#MISE description="Remove frontend build artifacts"
set -euo pipefail

rm -rf web/dist
```

**Step 3: Make the new task files executable**

Run: `chmod +x .mise/tasks/lint/web .mise/tasks/clean/web`
Expected: Files are executable.

**Step 4: Update the `ci` task depends line**

The existing `.mise/tasks/ci` has a `#MISE depends=` line listing
`lint:go`, `test:unit`, and `build:go`. Add `"lint:web"` to the
depends array. Only change the `depends` line -- preserve the existing
description and echo message:

Change:
```
#MISE depends=["lint:go", "test:unit", "build:go"]
```

To:
```
#MISE depends=["lint:go", "lint:web", "test:unit", "build:go"]
```

**Step 5: Verify tasks are listed**

Run: `mise tasks`
Expected: Output includes `lint:web`, `clean:web`, `dev:storybook`,
`dev:web`, and `build:web` among the full task list.

**Step 6: Verify `mise run lint:web` passes**

Run: `cd web && npm install && cd .. && mise run lint:web`
Expected: Exit 0, no type errors.

**Step 7: Commit**

`chore(mise): add lint:web and clean:web tasks, update ci`

---

### Task 14: Component Index Export

Groups all component exports into a single barrel file for clean
imports in Step 8 (Dashboard).

**Files:**
- Create: `web/src/components/index.ts`

**Step 1: Create `web/src/components/index.ts`**

```typescript
export { Button } from './Button'
export type { ButtonProps } from './Button'

export { StatusBadge } from './StatusBadge'
export type { StatusBadgeProps, NotificationStatus } from './StatusBadge'

export { Pagination } from './Pagination'
export type { PaginationProps } from './Pagination'

export { DarkModeToggle } from './DarkModeToggle'

export { NotificationRow } from './NotificationRow'
export type { NotificationRowProps, Notification } from './NotificationRow'
```

**Step 2: Verify the barrel file compiles**

Run: `cd web && npx tsc -b`
Expected: Exit 0, no errors.

**Step 3: Commit**

`feat(web): add component barrel export`

---

## Verification Checklist

- [ ] `cd web && npm install` succeeds
- [ ] `cd web && npm run build` produces `web/dist/index.html` and `web/dist/assets/` with hashed files
- [ ] `cd web && npx tsc -b` passes with no errors
- [ ] `go build ./...` succeeds (dev build, no spa tag)
- [ ] `go build -tags spa ./...` succeeds (after `npm run build` in `web/`)
- [ ] `go build -tags spa,qa ./...` succeeds (QA build)
- [ ] `go test ./internal/infra/httpapi/...` passes all SPA handler and dev proxy tests (6 SPA + 3 proxy)
- [ ] `go test ./cmd/daemon/...` passes with updated `ServerConfig`
- [ ] `mise run lint:go` produces no warnings
- [ ] `mise run lint:web` passes (TypeScript type check)
- [ ] `mise run build:web` produces `web/dist/`
- [ ] `mise run dev:storybook` starts Storybook on port 6006
- [ ] Storybook shows all component stories: Button, StatusBadge, Pagination, DarkModeToggle, NotificationRow
- [ ] Storybook dark mode toolbar toggle switches all components between light and dark mode
- [ ] Storybook Accessibility panel shows no violations for any component
- [ ] All stories render with `tags: ['autodocs']` (autodocs tab visible)
- [ ] SPA handler serves `index.html` at `/` (REQ-007)
- [ ] SPA handler falls back to `index.html` for unknown paths (REQ-008)
- [ ] SPA handler sets `Cache-Control: public, max-age=31536000, immutable` on `assets/*` (REQ-009)
- [ ] SPA handler does NOT set immutable cache on `index.html` (REQ-010)
- [ ] SPA handler does NOT set immutable cache on fallback responses (REQ-010)
- [ ] `GET /v1/health` returns health JSON, not index.html (REQ-011: API route priority)
- [ ] Dev proxy forwards requests to Vite dev server when `--dev` is passed (REQ-004)
- [ ] Brand colors (`bg-brand-primary`, `text-brand-accent`) work in both frontend and Storybook (REQ-024)
- [ ] `dark:` Tailwind variants apply when the `dark` class is on `<html>` (REQ-017)
- [ ] Storybook theme toolbar shows "light" selected on initial load (initialGlobals)
- [ ] NotificationRow shows Resend button only for Failed and NotSent states, not Delivered
- [ ] DarkModeToggle follows system preference on first visit (no localStorage yet)
- [ ] DarkModeToggle stops following system preference after user explicitly toggles
- [ ] `.gitignore` already covers `web/node_modules/` and `web/dist/` (verified, not modified)
