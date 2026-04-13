# Reset Button for Delivered Notifications

## Summary

Add a "Reset" button to delivered notification rows in the dashboard that calls `POST /v1/notify/reset`, enabling the full reset-to-re-delivery cycle from the UI.

## Motivation

The API already supports resetting delivered notifications via `POST /v1/notify/reset`, but this functionality is only accessible via API/MCP. The dashboard's Resend button only appears on `failed` and `not_sent` states, leaving no UI path to reset a delivered notification. This is needed for manual testing of the reset flow and for operational use.

## Affected Specs

- `openspec/specs/frontend-dashboard/spec.md` — adds Reset button requirements, updates shared requirements (REQ-014, REQ-061, REQ-063), adds scenarios

## Scope

**In scope:**
- Reset button UI on delivered notification rows
- Loading state during reset request
- Layout stability (min-width)
- WebSocket-driven state update after reset
- Updating existing requirements that assumed only Resend existed

**Out of scope:**
- Backend changes (reset endpoint already exists)
- Confirmation dialog before reset
- Batch reset of multiple notifications
