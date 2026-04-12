# Button Semantic Variants: Use Darker Token for Background

## Summary

Update REQ-033 through REQ-036 so that Button semantic variants use the darker `text` token as the background color with white text, instead of the lighter `bg` token with colored text. This aligns the spec with the implemented behavior and WCAG contrast requirements.

## Motivation

The spec originally mandated `bg-semantic-{name}-bg text-semantic-{name}-text` for both badges and buttons. During implementation, this pattern was found to produce insufficient contrast for interactive button elements: the light `bg` token (e.g., `#dcfce7` for success) paired with white text fails WCAG 1.4.3's 4.5:1 contrast ratio. The implementer correctly used `bg-semantic-{name}-text text-white` instead, which provides high-contrast, accessible buttons. Code review confirmed this is the right UX choice.

## Affected Specs

- `openspec/specs/frontend-dashboard/spec.md` -- REQ-033 through REQ-036 (button variant color patterns) and the "Button danger variant" scenario.

## Scope

**In scope:**
- Updating button variant requirements (REQ-033 through REQ-036) to specify `bg-semantic-{name}-text text-white`.
- Adding a design note distinguishing badge vs. button token usage.
- Updating the button danger variant scenario to match.

**Out of scope:**
- StatusBadge requirements (REQ-030, REQ-031) remain unchanged; badges correctly use the lighter `bg` token.
- Semantic token definitions in `brand.json` (REQ-028) are unchanged.
- No new tokens are introduced.
