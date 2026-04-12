# Button Semantic Variants: Use Darker Token for Background -- Tasks

## Task Breakdown

1. Update spec requirements REQ-033 through REQ-036
   - Files: `openspec/specs/frontend-dashboard/spec.md`
   - Verification: Each requirement specifies `bg-semantic-{name}-text text-white` for the button pattern.

2. Add design note distinguishing badge vs. button token usage
   - Files: `openspec/specs/frontend-dashboard/spec.md`
   - Verification: A note appears in the Button Variants section explaining the two patterns.

3. Update the "Button danger variant" scenario
   - Files: `openspec/specs/frontend-dashboard/spec.md`
   - Verification: The scenario asserts white text and the `text` token as background, not the `bg` token.

4. Verify no code changes are needed
   - Files: `web/src/components/Button.tsx`
   - Verification: Implementation already uses `bg-semantic-{name}-text text-white`. Spec now matches code.
