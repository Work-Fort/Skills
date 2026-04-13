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
  /** Called when the user clicks "Retry". Only shown for failed/not_sent states. */
  onResend?: (id: string) => void
  /** When true, the retry button shows a loading state. */
  resending?: boolean
  /** Called when the user clicks "Re-deliver". Only shown for delivered state. */
  onReset?: (id: string) => void
  /** When true, the re-deliver button shows a loading state. */
  resetting?: boolean
}

// Resend is only available for notifications that did not succeed.
// Delivered notifications are terminal but do not need resending.
const resendableStates: NotificationStatus[] = ['failed', 'not_sent']

export function NotificationRow({
  notification,
  onResend,
  resending = false,
  onReset,
  resetting = false,
}: NotificationRowProps) {
  const { id, email, status, retry_count, retry_limit } = notification

  // REQ-064/REQ-066: Resend visible for failed and not_sent states.
  const showResend = onResend && resendableStates.includes(status)

  // REQ-065/REQ-067: Disable when resending OR when auto-retry is
  // still in progress (not_sent with retries remaining).
  const retryInProgress = status === 'not_sent' && retry_count < retry_limit
  const disableResend = resending || retryInProgress

  // REQ-068: Reset visible only for delivered state.
  const showReset = onReset && status === 'delivered'

  return (
    <tr>
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
          <Button
            variant="secondary"
            className="min-w-[7.5rem]"
            onClick={() => onResend(id)}
            disabled={disableResend}
          >
            {resending ? 'Retrying...' : 'Retry'}
          </Button>
        )}
        {showReset && (
          <Button
            variant="secondary"
            className="min-w-[8.5rem]"
            onClick={() => onReset(id)}
            disabled={resetting}
          >
            {resetting ? 'Re-delivering...' : 'Re-deliver'}
          </Button>
        )}
      </td>
    </tr>
  )
}
