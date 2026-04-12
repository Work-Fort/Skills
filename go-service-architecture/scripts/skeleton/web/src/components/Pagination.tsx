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
  /** Current page number (1-indexed). */
  currentPage: number
  /** Total number of pages. */
  totalPages: number
  /** Called when the user clicks a numbered page button. */
  onPageChange: (page: number) => void
}

/**
 * Compute visible page numbers with ellipsis placeholders.
 * Returns an array of page numbers and null for ellipsis gaps.
 * Ellipsis logic applies when totalPages >= 7.
 */
function getPageNumbers(
  currentPage: number,
  totalPages: number,
): (number | null)[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }

  const pages: (number | null)[] = []

  // Always show first page.
  pages.push(1)

  // Left ellipsis: if current page is far enough from the start.
  if (currentPage > 3) {
    pages.push(null)
  }

  // Pages around the current page.
  const start = Math.max(2, currentPage - 1)
  const end = Math.min(totalPages - 1, currentPage + 1)
  for (let i = start; i <= end; i++) {
    pages.push(i)
  }

  // Right ellipsis: if current page is far enough from the end.
  if (currentPage < totalPages - 2) {
    pages.push(null)
  }

  // Always show last page.
  pages.push(totalPages)

  return pages
}

export function Pagination({
  onPrevious,
  onNext,
  hasPrevious,
  hasNext,
  currentPage,
  totalPages,
  onPageChange,
}: PaginationProps) {
  // REQ-058: Hide pagination entirely when 0 or 1 pages.
  if (totalPages <= 1) {
    return null
  }

  const pageNumbers = getPageNumbers(currentPage, totalPages)

  return (
    <nav
      className="flex items-center justify-between border-t border-gray-200 bg-white px-4 py-3 dark:border-gray-700 dark:bg-brand-primary"
      aria-label="Pagination"
    >
      <Button
        variant="secondary"
        onClick={onPrevious}
        disabled={!hasPrevious}
      >
        Previous
      </Button>

      <div className="flex items-center gap-1">
        {pageNumbers.map((page, index) =>
          page === null ? (
            <span
              key={`ellipsis-${index}`}
              className="px-2 text-gray-700 dark:text-brand-text"
              aria-hidden="true"
            >
              ...
            </span>
          ) : (
            <Button
              key={page}
              variant={page === currentPage ? 'primary' : 'secondary'}
              onClick={() => onPageChange(page)}
              aria-current={page === currentPage ? 'page' : undefined}
              aria-label={`Page ${page}`}
            >
              {page}
            </Button>
          ),
        )}
      </div>

      <div className="text-sm text-gray-700 dark:text-brand-text">
        Page {currentPage} of {totalPages}
      </div>

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
