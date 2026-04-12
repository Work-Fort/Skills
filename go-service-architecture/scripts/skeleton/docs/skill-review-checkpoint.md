# Skill Review Checkpoint

After completing each implementation step of the skeleton app, perform
a review of all skills and architecture documents. This step is
specific to building the skeleton — it's a dogfooding loop that
ensures the skills we're using stay accurate.

## When

After each step's code review passes, before moving to the next step's
planning phase.

## What to Check

1. **Architecture reference** — Does any example code conflict with
   what we actually implemented? Wrong function signatures, outdated
   API usage, missing parameters, incorrect patterns?

2. **Go frontend embed skill** — Does the Storybook, Vite, or embed
   pattern match what we built? Any gaps?

3. **Library stack** — Did we discover a library issue, version
   incompatibility, or better alternative during implementation?

4. **Plan conventions** — Did the plan format work well? Were tasks
   the right granularity? Did anything cause confusion for the
   developer?

5. **Agent prompts** — Did any agent (planner, assessor, developer,
   reviewer, QA) miss something that should be in their instructions?

6. **OpenSpec specs** — Did implementation reveal spec requirements
   that were wrong, missing, or ambiguous?

## Output

- Targeted edits to skills, agents, and architecture docs
- Commit with message: `fix(skills): update from step N lessons learned`

## Rule

Do not skip this step. Do not batch it. Review after every step
while the context is fresh.
