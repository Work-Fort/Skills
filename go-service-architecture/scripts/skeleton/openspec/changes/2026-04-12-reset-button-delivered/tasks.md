# Reset Button for Delivered Notifications — Tasks

## Task Breakdown

1. Add `onReset` and `resetting` props to `NotificationRow`
   - Files: `web/src/components/NotificationRow.tsx`
   - Verification: Component renders Reset button only for `delivered` status; shows "Resetting..." when `resetting` is true; button has `min-w-[7.5rem]`

2. Add `resetting` Set and `resetNotification` to `useNotifications` hook
   - Files: `web/src/hooks/useNotifications.ts`
   - Verification: Clicking Reset adds ID to resetting Set; WebSocket update to non-delivered state clears it

3. Wire Reset props through Dashboard
   - Files: `web/src/components/Dashboard.tsx`
   - Verification: Delivered rows show Reset button; clicking calls the API; loading state displays

4. Add/verify API client function for reset
   - Files: `web/src/api.ts`
   - Verification: `resetNotification` calls `POST /v1/notify/reset` with the email

5. Add Storybook stories for Reset button states
   - Files: `web/src/components/NotificationRow.stories.tsx`
   - Verification: Stories exist for delivered-with-reset, delivered-resetting; accessibility checks pass
