import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { Plugin } from 'vite'

interface SemanticColor {
  light: { bg: string; text: string }
  dark: { bg: string; text: string }
}

interface Brand {
  primary: string
  accent: string
  surface: string
  text: string
  fontSans: string
  fontMono: string
  semantic: Record<string, SemanticColor>
}

function quote(csv: string): string {
  return csv.split(', ').map(f => '"' + f.trim() + '"').join(', ')
}

function generateTheme(brand: Brand): string {
  const lines: string[] = ['@theme {']

  lines.push('  --font-sans: ' + quote(brand.fontSans) + ';')
  lines.push('  --font-mono: ' + quote(brand.fontMono) + ';')
  lines.push('')
  lines.push('  --color-brand-primary: ' + brand.primary + ';')
  lines.push('  --color-brand-accent: ' + brand.accent + ';')
  lines.push('  --color-brand-surface: ' + brand.surface + ';')
  lines.push('  --color-brand-text: ' + brand.text + ';')
  lines.push('')

  for (const [name, values] of Object.entries(brand.semantic)) {
    lines.push('  --color-semantic-' + name + '-bg: ' + values.light.bg + ';')
    lines.push('  --color-semantic-' + name + '-text: ' + values.light.text + ';')
  }

  lines.push('}')
  return lines.join('\n')
}

/**
 * Vite plugin that reads brand.json and injects @theme values into
 * index.css. Replaces the comment placeholder with the generated
 * @theme block, making brand.json the single source of truth.
 */
export default function brandTheme(): Plugin {
  const placeholder = '/* @brand-theme */'

  return {
    name: 'brand-theme',
    enforce: 'pre',
    transform(code, id) {
      if (!id.endsWith('index.css')) return
      if (!code.includes(placeholder)) return

      const brandPath = resolve(__dirname, '../brand.json')
      const brand: Brand = JSON.parse(readFileSync(brandPath, 'utf-8'))

      return code.replace(placeholder, generateTheme(brand))
    },
  }
}
