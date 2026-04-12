# Button Semantic Variants: Use Darker Token for Background -- Design

## Approach

Establish two distinct usage patterns for the semantic color tokens already defined in `brand.json`:

1. **Badge pattern (non-interactive):** `bg-semantic-{name}-bg text-semantic-{name}-text` -- lighter background with darker text. Suitable for status indicators where the element is read-only and the lighter tone provides a subtle, non-competing visual.

2. **Button pattern (interactive):** `bg-semantic-{name}-text text-white` -- the darker `text` token is repurposed as the button background, paired with white text. This produces a bold, high-contrast control that passes WCAG 1.4.3 (4.5:1 text contrast) and clearly signals interactivity.

No new tokens or `brand.json` changes are required. Both patterns derive from the same semantic token pair (`bg` and `text`); the difference is which token serves as background versus foreground.

## Components Affected

- `web/src/components/Button.tsx` -- already implements the correct pattern; no code change needed.
- `openspec/specs/frontend-dashboard/spec.md` -- requirements and scenario text updated to match implementation.

## Risks

- **Token naming may confuse future contributors.** The `text` token is used as a background color in the button pattern. Mitigation: the spec explicitly documents both patterns and explains the rationale, and a design note in the requirements section clarifies the distinction.

## Alternatives Considered

- **Add dedicated `button-bg` tokens to `brand.json`.** Rejected because the existing `text` token values already produce the correct contrast. Adding redundant tokens increases maintenance burden without improving correctness.
- **Use opacity modifiers to darken the `bg` token for buttons.** Rejected because Tailwind opacity modifiers on CSS custom properties are unreliable across color spaces, and the result would still be an approximation of what the `text` token already provides exactly.
