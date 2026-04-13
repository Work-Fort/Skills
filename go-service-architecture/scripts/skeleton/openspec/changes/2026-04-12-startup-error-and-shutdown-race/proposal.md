# Startup Error Reporting and Graceful Shutdown Race Fix

## Summary

Fix two reliability issues: (1) startup errors are silently swallowed due to `SilenceErrors: true` without stderr output, and (2) the database store closes before in-flight job runner goroutines finish, causing `sql: database is closed` errors during shutdown.

## Motivation

Issue #18: Users see no output when the service fails to start (e.g., unwritable XDG directory, invalid config). The process exits with code 1 but no error message, making troubleshooting impossible.

Issue #21: During graceful shutdown, the runner context is cancelled but the store is closed immediately via `defer` before in-flight jobs (which include a 6-second email send delay) can complete. This produces `sql: database is closed` errors as the job handler tries to update notification state after the store is gone.

## Affected Specs

- `openspec/specs/service-cli/spec.md` -- updated REQ-016 for stderr output, added REQ-017/018/019 for shutdown ordering and runner wait

## Scope

**In scope:**
- Print error to stderr before `os.Exit(1)` in `Execute()`
- Wait for in-flight runner jobs before closing the store during shutdown
- Shutdown timeout handling for stuck jobs

**Out of scope:**
- Changing the 15-second shutdown timeout value
- Modifying the email send delay
- ConsoleSender context-aware delay cancellation on shutdown
