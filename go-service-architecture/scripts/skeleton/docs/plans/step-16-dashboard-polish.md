---
type: plan
step: "16"
title: "Dashboard Polish"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "16"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-7-frontend-foundation
  - step-8-dashboard
  - step-13-design-tokens
---

# Step 16: Dashboard Polish

## Overview

Fixes two visual defects in the notifications dashboard: a double
border at the bottom of table rows and a broken empty state that
renders outside the table.

Currently `NotificationRow` adds `border-b` to every `<tr>`,
doubling up with the `divide-y` already on `<tbody>`. The fix
removes the per-row `border-b` so that `divide-y` is the sole
source of inter-row borders (REQ-047).

The container `<div>` uses `overflow-x-auto` but needs
`overflow-hidden` to clip child content to the rounded corners
(REQ-046).

The empty state currently renders as a standalone `<p>` outside
the table. It must render inside the same table structure (same
container `<div>`, same `<table>`, same `<thead>`) with a single
`<td colSpan={5}>` row showing "No notifications yet" (REQ-050
through REQ-052). The `Dashboard.stories.tsx` Empty story has the
same problem and must also show the in-table empty state
(REQ-053).

After this step:

- No `<tr>` in the table carries `border-b` classes.
- The wrapping `<div>` has `overflow-hidden` for rounded corner
  clipping.
- Empty notifications render as a single spanning row inside the
  table with headers visible.
- The Storybook Empty story renders identically to the live
  empty state.

Satisfies frontend-dashboard REQ-046 through REQ-053.

## Prerequisites

- Step 8 (Dashboard) completed: `App.tsx`, `NotificationRow.tsx`,
  and `Dashboard.stories.tsx` exist with the current table layout.

## Task Breakdown

### Task 1: Remove border-b from NotificationRow

**Files:**
- Modify: `web/src/components/NotificationRow.tsx:33`

**Step 1: Remove the per-row border classes**

In `NotificationRow.tsx`, change the `<tr>` className from:

```tsx
<tr className="border-b border-gray-200 dark:border-gray-700">
```

to:

```tsx
<tr>
```

The `<tbody>` in both `App.tsx` and `Dashboard.stories.tsx`
already carries `divide-y divide-gray-200 dark:divide-gray-700`,
which applies a top border to every row except the first. The
per-row `border-b` doubled the last row's border against the
container edge.

Satisfies REQ-047.

**Step 2: Verify visually**

Run: `cd web && npx storybook dev -p 6006 --no-open &`

Open the Default story in a browser and confirm:

- Rows are separated by single hairline borders (from `divide-y`).
- The last row has no double border at the bottom.
- The table outer edge comes solely from the container `<div>`.

Stop Storybook after verification.

**Step 3: Commit**

`fix(dashboard): remove per-row border-b, rely on tbody divide-y`

### Task 2: Fix container overflow for rounded corners

**Files:**
- Modify: `web/src/App.tsx:61`
- Modify: `web/src/Dashboard.stories.tsx:73`

**Step 1: Update the container div in App.tsx**

In `App.tsx`, change the table container `<div>` className from:

```tsx
<div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
```

to:

```tsx
<div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
```

Satisfies REQ-046.

**Step 2: Update the container div in Dashboard.stories.tsx**

In `Dashboard.stories.tsx`, apply the same change to the
`DashboardLayout` component's table container `<div>` (line 73):

```tsx
<div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
```

**Step 3: Commit**

`fix(dashboard): use overflow-hidden on table container for rounded corners`

### Task 3: Add in-table empty state to App.tsx

**Files:**
- Modify: `web/src/App.tsx:46-103`

**Step 1: Restructure the conditional rendering**

Replace the current ternary that hides the table when empty. The
table with its headers must always render; only the `<tbody>`
content changes. Replace lines 46-103 of `App.tsx` (from the
loading conditional through the closing of `</main>`) with:

```tsx
        {/* Loading state */}
        {loading && notifications.length === 0 ? (
          <div className="flex items-center justify-center py-20">
            <p className="text-gray-500 dark:text-gray-400">
              Loading notifications...
            </p>
          </div>
        ) : (
          <>
            {/* Notifications table */}
            <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
              <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                <thead className="bg-gray-50 dark:bg-brand-surface">
                  <tr>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      ID
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Email
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Status
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Retries
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-brand-primary">
                  {notifications.length === 0 ? (
                    <tr>
                      <td
                        colSpan={5}
                        className="py-12 text-center text-gray-500 dark:text-gray-400"
                      >
                        No notifications yet
                      </td>
                    </tr>
                  ) : (
                    notifications.map((n) => (
                      <NotificationRow
                        key={n.id}
                        notification={n}
                        onResend={resend}
                        resending={resending.has(n.id)}
                      />
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            <Pagination
              hasPrevious={hasPrevious}
              hasNext={hasNext}
              onPrevious={goPrevious}
              onNext={goNext}
            />
          </>
        )}
```

The empty `<td>` uses `colSpan={5}` (hardcoded to the 5 header
columns: ID, Email, Status, Retries, Actions), `py-12` for
vertical padding, `text-center` for centering, and
`text-gray-500 dark:text-gray-400` for muted color.

Satisfies REQ-050, REQ-051, REQ-052.

**Step 2: Verify the empty state renders correctly**

Run: `cd web && npx storybook dev -p 6006 --no-open &`

Open the Empty story and confirm:

- Table headers (ID, Email, Status, Retries, Actions) are visible.
- A single row shows "No notifications yet" centered with muted
  text.
- No content appears outside the table container.

Stop Storybook after verification.

**Step 3: Commit**

`fix(dashboard): render empty state inside table with spanning row`

### Task 4: Fix Empty story in Dashboard.stories.tsx

**Files:**
- Modify: `web/src/Dashboard.stories.tsx:93-104`

**Step 1: Update DashboardLayout to render in-table empty state**

In `Dashboard.stories.tsx`, replace the `<tbody>` and its
content (lines 94-103, inside the `DashboardLayout` component)
with:

```tsx
            <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-brand-primary">
              {notifications.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="py-12 text-center text-gray-500 dark:text-gray-400"
                  >
                    No notifications yet
                  </td>
                </tr>
              ) : (
                notifications.map((n) => (
                  <NotificationRow
                    key={n.id}
                    notification={n}
                    onResend={() => {}}
                  />
                ))
              )}
            </tbody>
```

This mirrors the `App.tsx` logic so the Empty story renders
identically to the live empty state: headers visible, single
spanning row with "No notifications yet".

Satisfies REQ-053.

**Step 2: Verify the Empty story**

Run: `cd web && npx storybook dev -p 6006 --no-open &`

Open the Empty story and confirm:

- Table headers (ID, Email, Status, Retries, Actions) are
  visible.
- A single row shows "No notifications yet" spanning all columns.
- No standalone message appears outside the table.

Also verify the Default, AllDelivered, AllFailed, and ErrorState
stories still render correctly with notification rows.

Stop Storybook after verification.

**Step 3: Run lint**

Run: `mise run lint:web`
Expected: PASS with no errors or warnings.

**Step 4: Commit**

`fix(dashboard): update storybook Empty story to in-table empty state`

## Verification Checklist

1. `grep 'border-b' web/src/components/NotificationRow.tsx`
   returns no matches (REQ-047)
2. `grep 'overflow-hidden' web/src/App.tsx` returns a match on
   the table container div (REQ-046)
3. `grep 'overflow-hidden' web/src/Dashboard.stories.tsx` returns
   a match (REQ-046)
4. `grep 'colSpan={5}' web/src/App.tsx` returns a match inside
   the empty state `<td>` (REQ-050)
5. `grep 'No notifications yet' web/src/App.tsx` returns a match
   inside a `<td>`, not a standalone `<p>` (REQ-051, REQ-052)
6. `grep 'colSpan={5}' web/src/Dashboard.stories.tsx` returns a
   match (REQ-053)
7. `grep '<hr' web/src/App.tsx` returns no matches (REQ-049)
8. `mise run lint:web` passes with no errors
