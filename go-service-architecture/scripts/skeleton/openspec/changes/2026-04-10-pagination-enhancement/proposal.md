# Pagination Enhancement

## Summary

Add total count and total pages metadata to the paginated list endpoint, and update the frontend pagination component to display numbered page buttons and a "Page X of Y" indicator. The MCP `list_notifications` tool response also gains the same totals.

## Motivation

The current pagination is Previous/Next only with no page context. Users cannot see how many notifications exist or how far through the list they are. Adding `total_count` and `total_pages` to the API response, along with numbered page buttons in the frontend, gives users positional awareness and direct page access.

## Affected Specs

- `openspec/specs/notification-management/spec.md` -- New `total_count` and `total_pages` fields in the `meta` response object; new `CountNotifications` store query requirement.
- `openspec/specs/frontend-dashboard/spec.md` -- Pagination component updated from Previous/Next to numbered page buttons with "Page X of Y" text; new Storybook story variants.
- `openspec/specs/mcp-integration/spec.md` -- The `list_notifications` tool response gains `total_count` and `total_pages` (covered by the notification-management spec change since MCP mirrors REST).

## Scope

**In scope:**
- `meta` response object gains `total_count` (integer) and `total_pages` (integer) fields.
- A `CountNotifications` method on the `NotificationStore` interface (a `SELECT COUNT(*)` query).
- Frontend `Pagination` component gains numbered page buttons, current page highlighting, and "Page X of Y" text.
- Storybook stories for many-page, single-page, and current-page-highlighting variants.
- MCP `list_notifications` response includes the same totals.

**Out of scope:**
- Changing the cursor-based pagination mechanism to offset-based. The cursor (`after` / `next_cursor`) remains the primary navigation mechanism.
- Server-side page number calculation based on offset. Page numbers are derived client-side from `total_pages`.
- Search, filtering, or sorting of notifications.
