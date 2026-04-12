import type { Preview } from '@storybook/react'
import '../src/index.css'

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Toggle light/dark mode',
      toolbar: {
        title: 'Theme',
        items: ['light', 'dark'],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    theme: 'light',
  },
  decorators: [
    (Story, context) => {
      const theme = context.globals.theme ?? 'light'
      document.documentElement.classList.toggle('dark', theme === 'dark')
      return <Story />
    },
  ],
}

export default preview
