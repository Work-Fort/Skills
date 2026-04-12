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
    'bg-semantic-neutral-bg text-semantic-neutral-text',
  sending:
    'bg-semantic-info-bg text-semantic-info-text',
  delivered:
    'bg-semantic-success-bg text-semantic-success-text',
  failed:
    'bg-semantic-danger-bg text-semantic-danger-text',
  not_sent:
    'bg-semantic-warning-bg text-semantic-warning-text',
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
