# Reset Button for Delivered Notifications — Design

## Approach

Extend `NotificationRow` with an `onReset` callback and `resetting` boolean prop, mirroring the existing `onResend`/`resending` pattern. The Reset button renders only for `delivered` status. The `useNotifications` hook tracks resetting state in a Set (same pattern as the `resending` Set) and clears it on WebSocket state transitions away from `delivered`.

## Components Affected

- `web/src/components/NotificationRow.tsx` — add Reset button for `delivered` status, add `onReset`/`resetting` props
- `web/src/hooks/useNotifications.ts` — add `resetting` Set tracking, expose `resetNotification` callback, clear resetting state on WebSocket updates
- `web/src/components/Dashboard.tsx` — wire `onReset`/`resetting` props through to `NotificationRow`
- `web/src/api.ts` — add `resetNotification(email: string)` API client function (may already exist as the Resend button uses the same endpoint)
- `web/src/components/NotificationRow.stories.tsx` — add story for delivered state with Reset button

## Risks

- **Shared endpoint confusion:** Both Resend and Reset call the same `POST /v1/notify/reset` endpoint. The naming distinction is purely UI-level. Mitigation: clear prop naming (`onResend` vs `onReset`) and JSDoc comments.
- **Dual tracking Sets:** The hook now tracks both `resending` and `resetting` Sets. Mitigation: WebSocket handler clears both on non-actionable state transitions.

## Alternatives Considered

- **Single "Reset" button for all resettable states:** Rejected because the existing Resend button has established UX conventions (disabled during auto-retry, etc.) that don't apply to delivered notifications.
- **Confirmation modal:** Out of scope — the reset is non-destructive (notification gets re-sent) and matches the one-click pattern of Resend.
