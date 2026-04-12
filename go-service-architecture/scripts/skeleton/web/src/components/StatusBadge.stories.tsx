import type { Meta, StoryObj } from '@storybook/react'
import { StatusBadge } from './StatusBadge'

const meta = {
  component: StatusBadge,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['pending', 'sending', 'delivered', 'failed', 'not_sent'],
    },
  },
} satisfies Meta<typeof StatusBadge>

export default meta
type Story = StoryObj<typeof meta>

export const Pending: Story = {
  args: { status: 'pending' },
}

export const Sending: Story = {
  args: { status: 'sending' },
}

export const Delivered: Story = {
  args: { status: 'delivered' },
}

export const Failed: Story = {
  args: { status: 'failed' },
}

export const NotSent: Story = {
  args: { status: 'not_sent' },
}
