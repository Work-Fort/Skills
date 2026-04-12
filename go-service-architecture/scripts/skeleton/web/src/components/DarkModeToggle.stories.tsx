import type { Meta, StoryObj } from '@storybook/react'
import { DarkModeToggle } from './DarkModeToggle'

const meta = {
  component: DarkModeToggle,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div className="flex items-center gap-4 p-4">
        <span className="text-sm text-gray-500 dark:text-gray-400">
          Click the toggle to switch themes:
        </span>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof DarkModeToggle>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
