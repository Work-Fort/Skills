import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { Pagination } from './Pagination'

const meta = {
  component: Pagination,
  tags: ['autodocs'],
  args: {
    onPrevious: fn(),
    onNext: fn(),
    onPageChange: fn(),
  },
} satisfies Meta<typeof Pagination>

export default meta
type Story = StoryObj<typeof meta>

export const FewPages: Story = {
  args: {
    currentPage: 3,
    totalPages: 5,
    hasPrevious: true,
    hasNext: true,
  },
}

export const ManyPages: Story = {
  args: {
    currentPage: 5,
    totalPages: 10,
    hasPrevious: true,
    hasNext: true,
  },
}

export const SinglePage: Story = {
  args: {
    currentPage: 1,
    totalPages: 1,
    hasPrevious: false,
    hasNext: false,
  },
}

export const FirstPage: Story = {
  args: {
    currentPage: 1,
    totalPages: 10,
    hasPrevious: false,
    hasNext: true,
  },
}

export const LastPage: Story = {
  args: {
    currentPage: 10,
    totalPages: 10,
    hasPrevious: true,
    hasNext: false,
  },
}
