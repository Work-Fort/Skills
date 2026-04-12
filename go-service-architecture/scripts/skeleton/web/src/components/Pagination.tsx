import { Button } from './Button'

export interface PaginationProps {
  /** Called when the user clicks "Previous". */
  onPrevious: () => void
  /** Called when the user clicks "Next". */
  onNext: () => void
  /** Disable the "Previous" button (e.g., on the first page). */
  hasPrevious: boolean
  /** Disable the "Next" button (e.g., on the last page). */
  hasNext: boolean
}

export function Pagination({
  onPrevious,
  onNext,
  hasPrevious,
  hasNext,
}: PaginationProps) {
  return (
    <nav
      className="flex items-center justify-between border-t border-gray-200 px-4 py-3 dark:border-gray-700"
      aria-label="Pagination"
    >
      <Button
        variant="secondary"
        onClick={onPrevious}
        disabled={!hasPrevious}
      >
        Previous
      </Button>
      <Button
        variant="secondary"
        onClick={onNext}
        disabled={!hasNext}
      >
        Next
      </Button>
    </nav>
  )
}
