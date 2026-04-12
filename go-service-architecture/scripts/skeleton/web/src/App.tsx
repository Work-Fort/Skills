import {
  DarkModeToggle,
  NotificationRow,
  Pagination,
} from './components'
import { useNotifications } from './hooks/useNotifications'

function App() {
  const {
    notifications,
    loading,
    error,
    hasNext,
    hasPrevious,
    goNext,
    goPrevious,
    resend,
    resending,
  } = useNotifications()

  return (
    <div className="min-h-screen bg-white dark:bg-brand-primary">
      {/* Header */}
      <header className="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
        <div className="mx-auto flex max-w-5xl items-center justify-between">
          <h1 className="text-xl font-bold text-gray-900 dark:text-brand-text">
            Notifier Dashboard
          </h1>
          <DarkModeToggle />
        </div>
      </header>

      {/* Main content */}
      <main className="mx-auto max-w-5xl px-6 py-6">
        {/* Error banner */}
        {error && (
          <div
            className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200"
            role="alert"
          >
            {error}
          </div>
        )}

        {/* Loading state */}
        {loading && notifications.length === 0 ? (
          <div className="flex items-center justify-center py-20">
            <p className="text-gray-500 dark:text-gray-400">
              Loading notifications...
            </p>
          </div>
        ) : notifications.length === 0 ? (
          <div className="flex items-center justify-center py-20">
            <p className="text-gray-500 dark:text-gray-400">
              No notifications yet.
            </p>
          </div>
        ) : (
          <>
            {/* Notifications table */}
            <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
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
                  {notifications.map((n) => (
                    <NotificationRow
                      key={n.id}
                      notification={n}
                      onResend={resend}
                      resending={resending.has(n.id)}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            <Pagination
              hasPrevious={hasPrevious}
              hasNext={hasNext}
              onPrevious={goPrevious}
              onNext={goNext}
            />
          </>
        )}
      </main>
    </div>
  )
}

export default App
