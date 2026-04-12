---
name: openspec
description: Spec-driven planning framework for coding agents. Generates requirements specs (SHALL language), scenarios (Given-When-Then), proposals, and spec deltas for existing codebases. Use when user says "write a spec", "create OpenSpec", "proposal", "spec delta", "requirements", "plan a change", "add a capability spec", "update requirements", "review spec changes", or "openspec proposal". Covers directory layout, spec format, proposal workflow, and diff-style spec deltas. Install via npm install -g @fission-ai/openspec@latest. No API keys, no MCP.
license: MIT
compatibility: opencode
metadata:
  author: Kaz Walker
  version: "1.0"
---

# OpenSpec

A lightweight spec-driven framework that acts as a universal planning layer for coding agents. Specs are living documentation stored alongside code in the repo. Brownfield-focused -- designed for existing codebases.

Install: `npm install -g @fission-ai/openspec@latest`. No API keys, no MCP.

## Directory Structure

```
openspec/
  specs/
    <capability-name>/
      spec.md           -- requirements and scenarios for this capability
  changes/
    <change-id>/
      proposal.md       -- what's being changed and why
      design.md         -- technical decisions
      tasks.md          -- implementation breakdown by phase
      specs/
        <affected-spec>/
          spec.md       -- spec delta (updated requirements)
```

Specs are organized by **capability**, not by feature request. A capability is a cohesive unit of system behavior (e.g., `session-management`, `user-onboarding`, `search`). Multiple feature requests may touch the same capability spec.

## Spec Format

Each `spec.md` has two sections: purpose and requirements with scenarios.

### Purpose

One paragraph describing what the capability does and why it exists.

### Requirements

Use SHALL language for all requirements. SHALL means the system must do this -- it is mandatory and testable.

```markdown
### Requirements

- REQ-1: The system SHALL authenticate users via email and password.
- REQ-2: The system SHALL lock accounts after 5 consecutive failed login attempts.
- REQ-3: The system SHALL expire sessions after the configured duration.
```

### Scenarios

Each requirement gets one or more scenarios in Given-When-Then format.

```markdown
#### Scenario: Default session timeout
- GIVEN a user has authenticated
- WHEN 24 hours pass without activity
- THEN invalidate the session token
- AND require re-authentication

#### Scenario: Account lockout after failed attempts
- GIVEN a user has entered incorrect credentials 4 times
- WHEN they enter incorrect credentials a 5th time
- THEN lock the account for 30 minutes
- AND send a security notification email
```

## Proposal Workflow

Triggered via `/openspec:proposal <description>`. The agent:

1. Searches existing specs and codebase for relevant context
2. Identifies which capability specs are affected
3. Generates three documents plus spec deltas:

**proposal.md** -- What is being changed and why. States the problem, the user impact, and the high-level approach. This is the document reviewers read first.

**design.md** -- Technical decisions. Covers architecture choices, trade-offs considered, and rationale. References affected specs by name.

**tasks.md** -- Implementation breakdown organized by phase. Each task is concrete and actionable. Tasks reference the spec requirements they satisfy.

**specs/\<affected-spec\>/spec.md** -- Spec deltas for each affected capability. Shows requirement changes as diffs.

## Spec Deltas

Spec deltas show how requirements change, using diff-style markup so reviewers understand intent changes without reading code.

```markdown
### Requirements (delta)

- REQ-3 (modified):
  - The system SHALL expire sessions after configured duration.
  + The system SHALL support configurable session expiration with remember-me option.

- REQ-7 (new):
  + The system SHALL extend session duration to 30 days when remember-me is active.
```

Added requirements use `+` prefix. Removed requirements use `-` prefix. Modified requirements show the old line with `-` and the new line with `+`. Unchanged requirements are omitted from the delta.

New scenarios follow the same pattern:

```markdown
#### Scenario: Remember-me extends session (new)
- GIVEN a user authenticates with remember-me enabled
- WHEN 24 hours pass without activity
- THEN the session SHALL remain valid
- AND the session SHALL expire after 30 days of inactivity
```

## Key Principles

1. **Capability-oriented.** Specs map to system capabilities, not feature requests. Multiple changes may update the same spec.
2. **SHALL for requirements.** Every requirement uses SHALL — in both requirements AND scenario THEN clauses. This makes specs prescriptive, not descriptive.
3. **Given-When-Then for scenarios.** Scenarios are concrete, verifiable, and serve as acceptance criteria. Use specific values — exact HTTP status codes, exact response bodies, exact error messages.
4. **Precise, not abstract.** Specify exact libraries, status codes, response formats, configuration values, and architectural constraints. Vague specs produce wrong implementations.
5. **Living documents.** Specs evolve as requirements change. Spec deltas track the evolution.
6. **Minimal process.** Get to a good enough plan quickly. Perfect specs are not the goal -- shared understanding is.
6. **Git-based collaboration.** Proposals and spec deltas are part of PRs. Review specs alongside code.

## Integration with Development Workflow

Specs are not standalone artifacts. They feed into the development cycle:

- **Planning:** The planner reads specs to understand current requirements before generating implementation plans.
- **Implementation:** Tasks in `tasks.md` reference spec requirements (REQ-1, REQ-2) for traceability.
- **Verification:** The assessor checks that implementation satisfies the requirements listed in the spec.
- **Review:** Spec deltas are part of the PR. Reviewers approve requirement changes alongside code changes.
- **Future work:** Specs persist as context. When a new change touches a capability, the agent reads the existing spec first.

## Quick Reference

| Artifact | Location | Purpose |
|----------|----------|---------|
| Capability spec | `openspec/specs/<name>/spec.md` | Current requirements and scenarios |
| Proposal | `openspec/changes/<id>/proposal.md` | What and why |
| Design | `openspec/changes/<id>/design.md` | Technical decisions |
| Tasks | `openspec/changes/<id>/tasks.md` | Implementation breakdown |
| Spec delta | `openspec/changes/<id>/specs/<name>/spec.md` | Requirement changes |
