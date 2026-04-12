import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { Pagination } from './Pagination'

const meta = {
  component: Pagination,
  tags: ['autodocs'],
  args: {
    onPrevious: fn(),
    onNext: fn(),
  },
} satisfies Meta<typeof Pagination>

export default meta
type Story = StoryObj<typeof meta>

export const BothEnabled: Story = {
  args: { hasPrevious: true, hasNext: true },
}

export const FirstPage: Story = {
  args: { hasPrevious: false, hasNext: true },
}

export const LastPage: Story = {
  args: { hasPrevious: true, hasNext: false },
}

export const SinglePage: Story = {
  args: { hasPrevious: false, hasNext: false },
}
