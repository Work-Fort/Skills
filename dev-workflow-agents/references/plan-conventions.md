# Plan Conventions

Standards for writing implementation plans. Required reading for
planners and developers.

## 1. Build Commands

All commands must use the project's task runner (e.g., `mise run`).
Never use raw language-specific commands for standard operations.

| Operation | Use | Don't use |
|-----------|-----|-----------|
| Build | `mise run build:go` | `go build` |
| Test | `mise run test:unit` | `go test` |
| Lint | `mise run lint:go` | `golangci-lint run` |

**Exception:** Targeted test runs by name during TDD are acceptable
with the language's native test runner:

```bash
go test -run TestEntityCreate ./internal/...
```

## 2. TDD Flow

For tasks that add testable functionality, use this flow:

1. **Write the failing test.** Include the full test code in the plan.
2. **Run the test.** Verify it fails with the expected error.
3. **Implement the code.** Include the full implementation in the plan.
4. **Run the test.** Verify it passes.
5. **Commit.** At the commit point specified in the plan.

The plan must include both the test code and the implementation code.
The developer copies from the plan — they should not invent details.

## 3. Plan Structure

````markdown
---
(YAML frontmatter)
---

# Step N: Title

## Overview
What this step achieves and why.

## Prerequisites
What must be true before starting.

## Tasks

### Task 1: Description

**Files:**
- Create: `exact/path/to/file.go`
- Modify: `exact/path/to/existing.go:42-58`
- Test: `exact/path/to/file_test.go`

**Step 1: Write the failing test**

```go
func TestSpecificBehavior(t *testing.T) {
    // full test code
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestSpecificBehavior ./internal/...`
Expected: FAIL with "undefined: Function"

**Step 3: Write minimal implementation**

```go
func Function(input string) string {
    // full implementation
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestSpecificBehavior ./internal/...`
Expected: PASS

**Step 5: Commit**

`feat(scope): add specific feature`

### Task 2: ...

## Verification Checklist
- [ ] Build succeeds with no warnings
- [ ] All tests pass
- [ ] Linter produces no warnings
- [ ] Manual smoke tests (if applicable)
````

## 3a. Bite-Sized Step Granularity

Each step is ONE action (2-5 minutes). Do not combine actions:

- "Write the test" — step
- "Run it to verify it fails" — separate step
- "Implement the code" — separate step
- "Run it to verify it passes" — separate step
- "Commit" — separate step

## 3b. Exact File Paths

Always include exact file paths. For modifications, include line
numbers so the developer knows exactly where to look:

- `Create: internal/infra/email/sender.go`
- `Modify: internal/infra/httpapi/server.go:87-103`
- `Test: internal/infra/email/sender_test.go`

## 3c. Commands with Expected Output

Every "run" step must include the exact command AND what the output
should look like. For failing tests, include the expected error message
so the developer can confirm they're failing for the right reason:

```
Run: `go test -run TestSessionTimeout ./tests/e2e/...`
Expected: FAIL with "expected 401 after timeout, got 200"
```

## 4. Commit Messages

Format: `<type>(<scope>): <description>`

| Type | Use for |
|------|---------|
| `feat` | New functionality |
| `fix` | Bug fix |
| `test` | Test-only changes |
| `chore` | Build, config, tooling |
| `refactor` | Code restructuring without behavior change |
| `docs` | Documentation only |

Scope: module or package name relevant to the change.

## 5. Code in Plans

- Include full code blocks — not pseudocode or descriptions
- Show complete file contents for new files
- Show the specific function/section to change for modifications
- Include import statements when adding new dependencies
- Use the project's actual types, not placeholders

## 6. Task Dependencies

If tasks depend on each other, state the dependency explicitly:

```markdown
### Task 3: Add handler
**Depends on:** Task 1 (entity type), Task 2 (store interface)
```

## 7. Scope Guidelines

- Target 5-15 tasks per plan
- Plans over ~1500 lines should be split into sub-steps
- Each task should be completable in one sitting
- Group related changes into a single task
- One commit point per logical unit of work
