import type { Meta, StoryObj } from '@storybook/react'
import { fn } from 'storybook/test'
import { NotificationRow } from './NotificationRow'

const meta = {
  component: NotificationRow,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <table className="min-w-full">
        <tbody>
          <Story />
        </tbody>
      </table>
    ),
  ],
  args: {
    onResend: fn(),
  },
} satisfies Meta<typeof NotificationRow>

export default meta
type Story = StoryObj<typeof meta>

export const Delivered: Story = {
  args: {
    notification: {
      id: 'ntf_abc123',
      email: 'user@example.com',
      status: 'delivered',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const Failed: Story = {
  args: {
    notification: {
      id: 'ntf_def456',
      email: 'bounce@example.com',
      status: 'failed',
      retry_count: 3,
      retry_limit: 3,
    },
  },
}

export const Pending: Story = {
  args: {
    notification: {
      id: 'ntf_ghi789',
      email: 'new@company.com',
      status: 'pending',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const Sending: Story = {
  args: {
    notification: {
      id: 'ntf_jkl012',
      email: 'sending@company.com',
      status: 'sending',
      retry_count: 0,
      retry_limit: 3,
    },
  },
}

export const NotSent: Story = {
  args: {
    notification: {
      id: 'ntf_mno345',
      email: 'retry@company.com',
      status: 'not_sent',
      retry_count: 1,
      retry_limit: 3,
    },
  },
}

export const Resending: Story = {
  args: {
    notification: {
      id: 'ntf_resend01',
      email: 'retry@company.com',
      status: 'failed',
      retry_count: 3,
      retry_limit: 3,
    },
    resending: true,
  },
}
