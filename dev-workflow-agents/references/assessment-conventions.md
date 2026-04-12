# Assessment Conventions

Standard structure, verification methodology, and checklist for plan assessments.

## Assessment Document Structure

Every assessment file follows this structure exactly. Do not add or skip sections.

### 1. Summary

Two to four sentences. State what plan was assessed, the verdict, and the count of findings by severity.

```
## Summary

Assessment of [plan name]. Verdict: [PASS | PASS WITH NOTES | FAIL].
Findings: [N] MUST FIX, [N] SHOULD FIX, [N] NICE TO HAVE.
```

### 2. Requirements Traceability Table

A table mapping every requirement to plan tasks. This section comes first after the summary because it is the most important check.

```
## Requirements Traceability

| Requirement | Plan Task(s) | Status |
|---|---|---|
| R1: [description] | Task 2.1, Task 2.3 | COVERED |
| R2: [description] | — | MISSING [MUST FIX] |
| — | Task 3.4 | NO REQUIREMENT (scope creep) [SHOULD FIX] |
```

Rules:
- Every requirement from the spec must appear in the left column.
- Every plan task must appear in the middle column at least once.
- MISSING requirements are always MUST FIX.
- Tasks with no corresponding requirement are SHOULD FIX.

### 3. Dependency Verification Table

A table covering every external dependency the plan introduces or modifies.

```
## Dependency Verification

| Dependency | Type | Version | Registry/Source | Status | Notes |
|---|---|---|---|---|---|
| [name] | package | 1.2.3 | [registry] | VERIFIED | No advisories |
| [name] | package | 0.9.0 | [registry] | FAIL | Version does not exist |
| [name] | runtime binary | 3.x | [source] | VERIFIED | Available on target |
| [name] | runtime binary | 2.1 | [source] | FAIL | Not available for target arch |
```

Types: `package`, `runtime binary`, `system library`, `external service`.

For each dependency verify:
- It exists at the specified version.
- No known security advisories at that version.
- License is compatible with the project.
- For runtime binaries: available on the target platform and architecture.

### 4. API Compatibility

A table covering every external API call the plan makes.

```
## API Compatibility

| API / Function | Plan Says | Actual (per docs) | Status |
|---|---|---|---|
| service.create(name, opts) | Returns ID (string) | Returns ID (string) | MATCH |
| service.delete(id) | Accepts int | Accepts string | MISMATCH [MUST FIX] |
| lib.parse(input, flag) | 2 params | 3 params (input, flag, encoding) | MISMATCH [MUST FIX] |
```

For each API call verify against official documentation:
- Function name and namespace
- Parameter names, types, and order
- Required vs optional parameters
- Return type
- Error/exception types
- Deprecation status

### 5. Findings

Individual findings, each tagged with severity. Group by category.

```
## Findings

### Architecture

**[MUST FIX]** Circular dependency between module A and module B.
The plan has module A importing from B and B importing from A.
*Remediation:* Extract shared types into a common module that both A and B import.

### Testing

**[SHOULD FIX]** No test for empty input case in the parser task.
The plan tests valid and malformed input but not empty input.
*Remediation:* Add a test case in the parser test suite that passes an empty string and asserts the expected error.

### Build System

**[NICE TO HAVE]** Build script could use a variable for the output directory instead of hardcoding the path.
*Remediation:* Define an OUTPUT_DIR variable at the top of the build script.
```

Every finding must have:
1. Severity tag in bold: `**[MUST FIX]**`, `**[SHOULD FIX]**`, or `**[NICE TO HAVE]**`
2. One-line description of the problem
3. Context explaining why it is a problem
4. Concrete remediation — what specifically to change, not "fix this"

Categories (use whichever apply, skip empty categories):
- Requirements
- Architecture
- API Compatibility
- Dependencies
- Testing
- Build System
- Security
- Performance

### 6. Verdict

```
## Verdict

**[FAIL | PASS WITH NOTES | PASS]**

[One sentence justification referencing the key findings.]
```

Verdict rules:
- Any MUST FIX -> FAIL
- Only SHOULD FIX and/or NICE TO HAVE -> PASS WITH NOTES
- No findings -> PASS

## Verification Methodology

### Source File Verification

1. For every file path in the plan, read the file. If the file does not exist and the plan does not create it, that is a MUST FIX.
2. For every function, type, or constant the plan references in an existing file, confirm it exists and has the signature the plan assumes.
3. For every modification the plan describes, confirm the target code region matches what the plan expects (has not changed since the plan was written).

### API Verification

1. Identify every external API call in the plan (library functions, HTTP endpoints, CLI tool invocations).
2. Locate the official documentation for each API at the version the plan targets.
3. Compare the plan's usage against the documentation. Check: function name, parameter list, parameter types, return type, error behavior, deprecation status.
4. If official documentation is unavailable, note this as a risk in the findings.

### Dependency Verification

1. For each package dependency: query the package registry to confirm the package exists at the specified version. Check for security advisories.
2. For each runtime binary: confirm the binary is available for the target platform and architecture. Check version compatibility.
3. For each external service: confirm the service endpoint is reachable and the API version the plan targets is still supported.

### Requirements Verification

1. List every deliverable from the requirements or spec.
2. For each deliverable, identify which plan tasks produce it.
3. Any deliverable with no task is MUST FIX.
4. Any task with no deliverable is SHOULD FIX (scope creep risk).
5. Check that acceptance criteria in the requirements have corresponding test cases in the plan.

## Full Checklist

This is the complete checklist an assessor runs before finalizing. Items are ordered by priority.

### Requirements
- [ ] Every requirement maps to at least one plan task
- [ ] Every plan task maps to at least one requirement
- [ ] Acceptance criteria have corresponding test cases

### Source Files
- [ ] Every file path in the plan exists or is explicitly created by the plan
- [ ] Every referenced function, type, or constant exists with the expected signature
- [ ] Every modification target matches current file contents

### External APIs
- [ ] Every API call matches official documentation for function name
- [ ] Parameter names, types, and order match
- [ ] Return types match
- [ ] No deprecated APIs are used without explicit justification
- [ ] Authentication requirements are correctly described

### Dependencies
- [ ] Every package exists at the specified version in its registry
- [ ] No known security advisories for any dependency at specified version
- [ ] All licenses are compatible with the project
- [ ] No unnecessary dependencies (standard library alternatives exist)
- [ ] Every runtime binary is available on the target platform

### Architecture
- [ ] No circular dependencies introduced
- [ ] Error handling is specified for all fallible operations
- [ ] Concurrency concerns addressed where applicable
- [ ] Follows existing project conventions and patterns

### Testing
- [ ] Every deliverable has at least one test
- [ ] Edge cases are identified and tested
- [ ] Integration tests cover cross-boundary interactions
- [ ] Test approach matches project's existing test conventions

### Build System
- [ ] Build configuration changes are syntactically correct
- [ ] New build targets or scripts are correctly defined
- [ ] CI/CD changes align with existing pipeline setup

### Assessment Quality
- [ ] Every finding has a severity tag
- [ ] Every finding has a concrete remediation
- [ ] Verdict follows severity rules (any MUST FIX = FAIL)
- [ ] Assessment file is marked as temporary and will not be committed
