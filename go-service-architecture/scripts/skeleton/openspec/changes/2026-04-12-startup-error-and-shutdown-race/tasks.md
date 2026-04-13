# Startup Error Reporting and Graceful Shutdown Race Fix -- Tasks

## Task Breakdown

1. Verify startup error output (spec-only change for #18)
   - Files: `internal/cli/root.go` (read-only verification)
   - Verification: confirm `fmt.Fprintln(os.Stderr, err)` exists before `os.Exit(1)`. No code change needed.

2. Add runner completion signaling in daemon shutdown
   - Files: `cmd/daemon/daemon.go`
   - Add a `done` channel or `sync.WaitGroup` around `go runner.Start(runnerCtx)`
   - After `runnerCancel()`, wait on the completion signal bounded by `shutdownCtx`
   - Verification: start the service, trigger a notification, send SIGTERM during the 6-second email delay, confirm no `sql: database is closed` errors in logs

3. Add E2E or integration test for graceful shutdown with in-flight job
   - Files: `tests/e2e/` or `cmd/daemon/daemon_test.go`
   - Start `RunServer` with a cancellable context, enqueue a job, cancel context, verify no store errors
   - Verification: `mise run test:e2e` passes
