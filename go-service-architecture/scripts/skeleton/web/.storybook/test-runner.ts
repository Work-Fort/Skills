import type { TestRunnerConfig } from '@storybook/test-runner'
import { checkA11y, injectAxe } from 'axe-playwright'

const a11yOptions = {
  detailedReport: true,
  detailedReportOptions: { html: true },
  axeOptions: {
    runOnly: {
      type: 'tag' as const,
      values: ['wcag2a', 'wcag2aa', 'wcag21aa'],
    },
  },
}

const config: TestRunnerConfig = {
  async preVisit(page) {
    await injectAxe(page)
  },
  async postVisit(page) {
    // Check light mode
    await page.evaluate(() =>
      document.documentElement.classList.remove('dark')
    )
    await checkA11y(page, '#storybook-root', a11yOptions)

    // Check dark mode
    await page.evaluate(() =>
      document.documentElement.classList.add('dark')
    )
    await checkA11y(page, '#storybook-root', a11yOptions)
  },
}

export default config
