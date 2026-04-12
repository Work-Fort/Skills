---
type: plan
step: "14"
title: "Contrast and Accessibility"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "14"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-7-frontend-foundation
  - step-8-dashboard
---

# Step 14: Contrast and Accessibility

## Overview

Adds automated WCAG contrast testing to the Storybook pipeline using
`@storybook/test-runner` with `axe-playwright`. Currently,
`@storybook/addon-a11y` is installed and shows axe-core results
interactively in the Storybook UI, but nothing gates CI. This step
introduces a test-runner configuration that runs axe-core against
every story (in both light and dark mode) and fails the CI pipeline
on any WCAG 2.1 Level AA violation.

After this step:

- `@storybook/test-runner`, `axe-playwright`, `concurrently`,
  `http-server`, and `wait-on` are installed as devDependencies in
  `web/package.json`.
- A test-runner config at `web/.storybook/test-runner.ts` configures
  axe-playwright with WCAG 2.1 AA tags and injects `preVisit` /
  `postVisit` hooks for light and dark mode checks.
- A mise task `test:a11y` builds Storybook, serves it statically,
  runs the test-runner, and cleans up.
- `mise run ci` depends on `test:a11y`.

Satisfies frontend-dashboard REQ-038 through REQ-045.

## Prerequisites

- Step 7 (Frontend Foundation) and Step 8 (Dashboard) completed:
  Storybook is configured with `.storybook/main.ts` and
  `.storybook/preview.tsx`, all component stories exist.
- `@storybook/addon-a11y` is already in `web/package.json` and
  `.storybook/main.ts`.
- Node.js and npm are available via mise.

## Task Breakdown

### Task 1: Install all npm packages

**Files:**
- Modify: `web/package.json`

**Step 1: Install test-runner and axe-playwright**

Run: `cd web && npm install --save-dev @storybook/test-runner axe-playwright`

This adds `@storybook/test-runner` and `axe-playwright` to
devDependencies. Satisfies REQ-043.

**Step 2: Install CI helper packages**

Run: `cd web && npm install --save-dev concurrently http-server wait-on`

These are used by the `test:a11y` mise task to serve built
Storybook and wait for readiness before running tests.

**Step 3: Add test-storybook script to package.json**

Add to `web/package.json` scripts section:
`"test-storybook": "test-storybook"`

**Step 4: Verify all packages**

Run: `cd web && npm ls @storybook/test-runner axe-playwright concurrently http-server wait-on`
Expected: All five packages listed without errors.

**Step 5: Commit**

`feat(web): add storybook test-runner, axe-playwright, and CI helpers`

### Task 2: Create test-runner config

**Files:**
- Create: `web/.storybook/test-runner.ts`

**Step 1: Write the test-runner config**

The `@storybook/test-runner` for Storybook 10.x auto-discovers
`.storybook/test-runner.ts` as the configuration file. This file
exports a `TestRunnerConfig` with `preVisit` and `postVisit` hooks.

```ts
import type { TestRunnerConfig } from '@storybook/test-runner'
import { checkA11y, injectAxe } from 'axe-playwright'

const a11yOptions = {
  detailedReport: true,
  detailedReportOptions: { html: true },
  axeOptions: {
    runOnly: {
      type: 'tag' as const,
      values: ['wcag2a', 'wcag2aa', 'wcag21aa'],
    },
  },
}

const config: TestRunnerConfig = {
  async preVisit(page) {
    await injectAxe(page)
  },
  async postVisit(page) {
    // Check light mode
    await page.evaluate(() =>
      document.documentElement.classList.remove('dark')
    )
    await checkA11y(page, '#storybook-root', a11yOptions)

    // Check dark mode
    await page.evaluate(() =>
      document.documentElement.classList.add('dark')
    )
    await checkA11y(page, '#storybook-root', a11yOptions)
  },
}

export default config
```

Satisfies REQ-038, REQ-039, REQ-040, REQ-044.

**Step 2: Commit**

`feat(a11y): add test-runner config with WCAG AA axe checks`

### Task 3: Create mise task for accessibility tests

**Files:**
- Create: `.mise/tasks/test/a11y`

**Step 1: Write the task**

```bash
#!/usr/bin/env bash
#MISE description="Run Storybook accessibility tests (WCAG 2.1 AA)"
set -euo pipefail

cd web

# Build Storybook for static serving (faster than dev mode in CI)
npx storybook build --quiet 2>/dev/null

# Serve static Storybook, run tests, then clean up
npx concurrently --kill-others --success first \
  "npx http-server storybook-static --port 6006 --silent" \
  "npx wait-on http://127.0.0.1:6006 && npx test-storybook --url http://127.0.0.1:6006"
```

Satisfies REQ-041.

**Step 2: Make executable**

Run: `chmod +x .mise/tasks/test/a11y`

**Step 3: Verify task is listed**

Run: `mise tasks | grep a11y`
Expected: `test:a11y` appears in the output.

**Step 4: Commit**

`feat(ci): add mise test:a11y task for WCAG contrast checks`

### Task 4: Add test:a11y to CI depends

**Files:**
- Modify: `.mise/tasks/ci`

**Step 1: Update CI depends**

Change `#MISE depends=["lint:go", "lint:web", "test:unit", "build:go"]`
to `#MISE depends=["lint:go", "lint:web", "test:unit", "test:a11y", "build:go"]`.

Satisfies REQ-042.

**Step 2: Commit**

`feat(ci): add a11y checks to CI pipeline`

## Verification Checklist

1. `cd web && npx test-storybook --help` prints usage
2. `web/.storybook/test-runner.ts` exists and exports a config
   with `preVisit` and `postVisit` hooks
3. `.mise/tasks/test/a11y` is executable and listed by `mise tasks`
4. `grep test:a11y .mise/tasks/ci` shows the dependency
5. `cd web && npm ls @storybook/test-runner axe-playwright` shows
   both packages installed
6. `mise run lint:web` passes (no type errors in new files)
