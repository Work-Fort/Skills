# Future Work

Items identified during the skeleton app build that are out of scope
for the initial implementation but worth revisiting.

## Typography and Font Faces

- Add `fontSans` and `fontMono` to `brand.json` as the single source
  of truth for typography across dashboard and email
- Switch email templates to a sans-serif stack (currently falling back
  to browser/client default serif)
- Dashboard should also reference brand fonts via Tailwind `@theme`
- Email font stacks need safe fallbacks — web fonts are unreliable in
  email clients (Gmail strips `@font-face`, Outlook ignores it)
- Consider: Inter, system-ui, sans-serif for body; JetBrains Mono or
  monospace for code/IDs
- Both the Maizzle `tailwind.config.js` and the dashboard `index.css`
  `@theme` block should consume the same font values from `brand.json`
