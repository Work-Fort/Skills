# Button Semantic Variants: Use Darker Token for Background -- Spec Delta

## frontend-dashboard/spec.md

### Requirements Changed

- REQ-033: The `success` variant SHALL use semantic success token colors (`bg-semantic-success-bg text-semantic-success-text` in light mode, auto-switching via CSS custom properties in dark mode). Hover state SHALL darken the background by 10% using Tailwind opacity modifier or a dedicated hover token.
+ REQ-033: The `success` variant SHALL use the semantic success `text` token as background with white text (`bg-semantic-success-text text-white`), auto-switching via CSS custom properties in dark mode. Hover state SHALL darken the background by 10% using Tailwind opacity modifier or a dedicated hover token.

- REQ-034: The `warning` variant SHALL use semantic warning token colors with the same pattern as `success`.
+ REQ-034: The `warning` variant SHALL use the semantic warning `text` token as background with white text (`bg-semantic-warning-text text-white`), following the same button pattern as `success`.

- REQ-035: The `info` variant SHALL use semantic info token colors with the same pattern as `success`.
+ REQ-035: The `info` variant SHALL use the semantic info `text` token as background with white text (`bg-semantic-info-text text-white`), following the same button pattern as `success`.

- REQ-036: The `danger` variant SHALL use semantic danger token colors with the same pattern as `success`.
+ REQ-036: The `danger` variant SHALL use the semantic danger `text` token as background with white text (`bg-semantic-danger-text text-white`), following the same button pattern as `success`.

### Scenarios Changed

- **Button danger variant renders with red semantic tokens**
-   Given a `Button` with `variant="danger"`
-   When the component renders in light mode
-   Then the button background SHALL use `--color-semantic-danger-bg` (`#fee2e2`)
-   And the button text SHALL use `--color-semantic-danger-text` (`#991b1b`)
+ **Button danger variant renders with red semantic tokens**
+   Given a `Button` with `variant="danger"`
+   When the component renders in light mode
+   Then the button background SHALL use `--color-semantic-danger-text` (`#991b1b`)
+   And the button text SHALL be white (`#ffffff`)
