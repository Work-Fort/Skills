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
  /** When true, the resend button shows a loading state. */
  resending?: boolean
}

// Resend is only available for notifications that did not succeed.
// Delivered notifications are terminal but do not need resending.
const resendableStates: NotificationStatus[] = ['failed', 'not_sent']

export function NotificationRow({
  notification,
  onResend,
  resending = false,
}: NotificationRowProps) {
  const { id, email, status, retry_count, retry_limit } = notification

  // REQ-064/REQ-066: Resend visible for failed and not_sent states.
  const showResend = onResend && resendableStates.includes(status)

  // REQ-065/REQ-067: Disable when resending OR when auto-retry is
  // still in progress (not_sent with retries remaining).
  const retryInProgress = status === 'not_sent' && retry_count < retry_limit
  const disableResend = resending || retryInProgress

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
            className="min-w-[7rem]"
            onClick={() => onResend(id)}
            disabled={disableResend}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
      </td>
    </tr>
  )
}
