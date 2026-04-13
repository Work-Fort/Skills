# Reset Button for Delivered Notifications — Spec Delta

## frontend-dashboard/spec.md

### Requirements Changed

- REQ-014: The dashboard SHALL provide a resend button that calls `POST /v1/notify/reset` for a given notification.
+ REQ-014: The dashboard SHALL provide a Resend button for `failed`/`not_sent` notifications and a Reset button for `delivered` notifications. Both buttons call `POST /v1/notify/reset` with the notification's email address.

- REQ-061: Interactive buttons whose visible text changes between states (e.g., "Resend" / "Resending...") SHALL have a stable rendered width that does not change when the text changes. The button SHALL use a CSS `min-width` (or equivalent constraint) that accommodates the widest text variant, preventing table column resize and layout shift on state change.
+ REQ-061: Interactive buttons whose visible text changes between states (e.g., "Resend" / "Resending...", "Reset" / "Resetting...") SHALL have a stable rendered width that does not change when the text changes. The button SHALL use a CSS `min-width` (or equivalent constraint) that accommodates the widest text variant, preventing table column resize and layout shift on state change.

- REQ-063: When the `useNotifications` hook receives a WebSocket update that changes a notification's state to a non-resendable state (`pending`, `sending`, or `delivered`), it SHALL immediately remove that notification's ID from the `resending` Set, regardless of whether the resend HTTP request has completed. This prevents a render frame where the button is simultaneously disabled (resending in progress) and absent (state no longer resendable).
+ REQ-063: When the `useNotifications` hook receives a WebSocket update that changes a notification's state to a non-actionable state (`pending` or `sending`), it SHALL immediately remove that notification's ID from both the `resending` Set and the `resetting` Set (if tracked separately), regardless of whether the HTTP request has completed. This prevents a render frame where a button is simultaneously disabled (request in progress) and absent (state no longer shows that button).

### Requirements Added

+ REQ-068: For notifications in `delivered` state, the `NotificationRow` component SHALL display a "Reset" button. The Reset button SHALL NOT appear for any other status.
+ REQ-069: The Reset button SHALL call `POST /v1/notify/reset` with the notification's email address when clicked. The `NotificationRowProps` interface SHALL accept an `onReset` callback `(id: string) => void` and a `resetting` boolean prop.
+ REQ-070: While the reset HTTP request is in flight, the Reset button SHALL display the text "Resetting..." and SHALL be disabled.
+ REQ-071: The Reset button SHALL have a CSS `min-width` (e.g., `min-w-[7.5rem]`) that accommodates the "Resetting..." label so that switching between "Reset" and "Resetting..." does not cause the Actions column or the table to reflow.
+ REQ-072: After a successful reset, the WebSocket update SHALL transition the notification to `pending` state. The `useNotifications` hook SHALL remove the notification's ID from the resetting tracking state when it receives a WebSocket update changing the notification's state away from `delivered`.
+ REQ-073: The Resend button and the Reset button SHALL NOT both appear on the same notification row. The Resend button appears for `failed` and `not_sent` states; the Reset button appears for `delivered` state. The `pending` and `sending` states show no action button.

### Scenarios Added

+ **Reset button displayed on delivered notification**
+ **Reset button not displayed on non-delivered statuses**
+ **Reset button shows loading state during request**
+ **Reset completes and WebSocket updates row**
+ **No action button on pending or sending notifications**
