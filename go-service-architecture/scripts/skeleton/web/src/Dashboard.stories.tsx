import type { Meta, StoryObj } from '@storybook/react'
import {
  DarkModeToggle,
  NotificationRow,
  Pagination,
  type Notification,
} from './components'

const mockNotifications: Notification[] = [
  {
    id: 'ntf_a1b2c3d4',
    email: 'alice@company.com',
    status: 'delivered',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_e5f6g7h8',
    email: 'bob@example.com',
    status: 'failed',
    retry_count: 3,
    retry_limit: 3,
  },
  {
    id: 'ntf_i9j0k1l2',
    email: 'carol@company.com',
    status: 'sending',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_m3n4o5p6',
    email: 'dave@company.com',
    status: 'pending',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_q7r8s9t0',
    email: 'eve@company.com',
    status: 'not_sent',
    retry_count: 1,
    retry_limit: 3,
  },
]

function DashboardLayout({
  notifications,
  error,
}: {
  notifications: Notification[]
  error?: string
}) {
  return (
    <div className="min-h-screen bg-white dark:bg-brand-primary">
      <header className="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
        <div className="mx-auto flex max-w-5xl items-center justify-between">
          <h1 className="text-xl font-bold text-gray-900 dark:text-brand-text">
            Notifier Dashboard
          </h1>
          <DarkModeToggle />
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-6">
        {error && (
          <div
            className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200"
            role="alert"
          >
            {error}
          </div>
        )}
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-brand-surface">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  ID
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Email
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Status
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Retries
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-brand-primary">
              {notifications.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="py-12 text-center text-gray-500 dark:text-gray-400"
                  >
                    No notifications yet
                  </td>
                </tr>
              ) : (
                notifications.map((n) => (
                  <NotificationRow
                    key={n.id}
                    notification={n}
                    onResend={() => {}}
                  />
                ))
              )}
            </tbody>
          </table>
        </div>
        <Pagination
          hasPrevious={false}
          hasNext={true}
          onPrevious={() => {}}
          onNext={() => {}}
        />
      </main>
    </div>
  )
}

const meta = {
  component: DashboardLayout,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<typeof DashboardLayout>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    notifications: mockNotifications,
  },
}

export const Empty: Story = {
  args: {
    notifications: [],
  },
}

export const AllDelivered: Story = {
  args: {
    notifications: mockNotifications
      .slice(0, 3)
      .map((n) => ({ ...n, status: 'delivered' as const })),
  },
}

export const AllFailed: Story = {
  args: {
    notifications: mockNotifications
      .slice(0, 3)
      .map((n) => ({
        ...n,
        status: 'failed' as const,
        retry_count: 3,
      })),
  },
}

export const ErrorState: Story = {
  args: {
    notifications: mockNotifications.slice(0, 2),
    error: 'Failed to load notifications',
  },
}
