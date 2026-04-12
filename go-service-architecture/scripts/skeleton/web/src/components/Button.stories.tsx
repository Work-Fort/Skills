import type { Meta, StoryObj } from '@storybook/react'
import { Button } from './Button'

const meta = {
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['primary', 'secondary', 'success', 'warning', 'info', 'danger'],
    },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Primary: Story = {
  args: { variant: 'primary', children: 'Send Notification' },
}

export const Secondary: Story = {
  args: { variant: 'secondary', children: 'Cancel' },
}

export const Success: Story = {
  args: { variant: 'success', children: 'Confirm' },
}

export const Warning: Story = {
  args: { variant: 'warning', children: 'Proceed' },
}

export const Info: Story = {
  args: { variant: 'info', children: 'Details' },
}

export const Danger: Story = {
  args: { variant: 'danger', children: 'Delete' },
}

export const Disabled: Story = {
  args: { variant: 'primary', children: 'Send Notification', disabled: true },
}
