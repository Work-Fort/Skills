export type NotificationStatus =
  | 'pending'
  | 'sending'
  | 'delivered'
  | 'failed'
  | 'not_sent'

export interface StatusBadgeProps {
  status: NotificationStatus
}

const styles: Record<NotificationStatus, string> = {
  pending:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  sending:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  delivered:
    'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  failed:
    'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  not_sent:
    'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
}

const labels: Record<NotificationStatus, string> = {
  pending: 'Pending',
  sending: 'Sending',
  delivered: 'Delivered',
  failed: 'Failed',
  not_sent: 'Not Sent',
}

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${styles[status]}`}
    >
      {labels[status]}
    </span>
  )
}
