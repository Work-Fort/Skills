---
type: plan
step: "9"
title: "release:qa mise task"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "9"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
---

# Step 9: release:qa Mise Task

## Overview

Add the missing `release:qa` mise task so QA builds go through the
same task runner as dev and production builds. Today, building a QA
binary requires a manual `go build -tags spa,qa` invocation that
bypasses mise entirely. This step creates
`.mise/tasks/release/qa` following the same conventions as the
existing `release/production` and `release/dev` tasks.

The QA binary is production-like (static, stripped, trimmed) but
includes both the embedded SPA and QA seed data via the `spa,qa`
build tags. This satisfies spec REQ-025.

## Prerequisites

- Step 1 completed: mise task infrastructure and `build:web` /
  `build:email` tasks exist.
- Step 8 completed: the frontend builds successfully via
  `mise run build:web`.

## Tasks

### Task 1: Create the release:qa task script

**Files:**
- Create: `.mise/tasks/release/qa`

**Step 1: Create `.mise/tasks/release/qa`**

Create the file `.mise/tasks/release/qa` with the following content:

```bash
#!/usr/bin/env bash
#MISE description="Build QA binary with embedded SPA and seed data"
#MISE depends=["build:web", "build:email"]
set -euo pipefail

CGO_ENABLED=0 go build \
    -tags spa,qa \
    -ldflags="-s -w" \
    -trimpath \
    -o build/notifier .
```

Design decisions:

- **`depends=["build:web", "build:email"]`** -- matches
  `release:production`. The QA binary embeds the SPA and compiled
  email templates, so both asset builds must run first.
- **`CGO_ENABLED=0`, `-ldflags="-s -w"`, `-trimpath`** -- matches
  production flags. QA tests a production-like binary; the only
  difference is the `qa` build tag that compiles in seed data.
- **`-tags spa,qa`** -- `spa` enables SPA embedding, `qa` enables
  seed data (REQ-003, REQ-025).
- **No `-X main.Version`** -- QA builds do not need a release
  version stamp. The default `dev` value from the `VERSION` var in
  `main.go` is sufficient. This keeps the task simpler and avoids
  confusion about what version a QA binary reports.
- **No `-race`** -- the race detector requires CGO on most
  platforms and is incompatible with `CGO_ENABLED=0`. The dev build
  already covers race detection.

Satisfies: REQ-010, REQ-019, REQ-025.

**Step 2: Make the script executable**

Run: `chmod +x .mise/tasks/release/qa`

**Step 3: Verify mise discovers the task**

Run: `mise task ls 2>/dev/null | grep release:qa`

Expected: one line containing `release:qa` and the description
"Build QA binary with embedded SPA and seed data".

**Step 4: Commit**

`chore(build): add release:qa mise task`

### Task 2: Verify the full build

**Depends on:** Task 1

**Step 1: Run the QA release task**

Run: `mise run release:qa`

Expected: the command exits 0 and produces `build/notifier`.

**Step 2: Confirm the binary exists and is statically linked**

Run: `file build/notifier`

Expected: output contains "statically linked" (confirming
`CGO_ENABLED=0` took effect).

**Step 3: Confirm the binary size is reasonable**

Run: `ls -lh build/notifier`

Expected: the binary size is in the same range as the production
binary (within a few MB -- seed data adds minimal overhead).

## Verification Checklist

- [ ] `.mise/tasks/release/qa` exists and is executable
- [ ] `mise task ls` shows `release:qa` with correct description
- [ ] `mise run release:qa` exits 0 and produces `build/notifier`
- [ ] `file build/notifier` confirms static linking
- [ ] Script includes `set -euo pipefail` (REQ-019)
- [ ] Script includes `#MISE description="..."` (REQ-019)
- [ ] Build tags are `-tags spa,qa` (REQ-025)
- [ ] Flags match production: `CGO_ENABLED=0`, `-ldflags="-s -w"`,
      `-trimpath` (REQ-014 pattern)
- [ ] `mise.toml` has no inline task definitions (REQ-020)
