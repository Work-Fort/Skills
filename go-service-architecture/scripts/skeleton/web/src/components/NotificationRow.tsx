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
  const showResend = onResend && resendableStates.includes(status)

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
            onClick={() => onResend(id)}
            disabled={resending}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
      </td>
    </tr>
  )
}
