import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const brand = require('../brand.json')

/** @type {import('tailwindcss').Config} */
export default {
  content: ['emails/**/*.html'],
  presets: [
    require('tailwindcss-preset-email'),
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          primary: brand.primary,
          accent: brand.accent,
          surface: brand.surface,
          text: brand.text,
        },
      },
      fontFamily: {
        sans: brand.fontSans.split(', '),
        mono: brand.fontMono.split(', '),
      },
    },
  },
}
