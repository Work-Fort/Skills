# Startup Error Reporting and Graceful Shutdown Race Fix -- Design

## Approach

### #18 -- Startup Error Reporting

The code already has `fmt.Fprintln(os.Stderr, err)` in `Execute()` (root.go:49). The spec was missing this requirement. The spec update documents the existing behavior. No code change needed.

### #21 -- Graceful Shutdown Race

The current shutdown sequence cancels the runner context (step 4) and then returns from `RunServer`, which triggers `defer store.Close()`. The runner goroutine (`go runner.Start(runnerCtx)`) may still be executing a job when the store closes.

The fix adds a synchronization point between runner cancellation and store closure. After cancelling the runner context, `RunServer` must wait for the runner goroutine to exit before returning. This can be done by:

1. Running `runner.Start()` in a goroutine that signals completion via a channel or `sync.WaitGroup`
2. After `runnerCancel()`, waiting on that signal (bounded by `shutdownCtx`)
3. Only then allowing `RunServer` to return (which triggers the deferred `store.Close()`)

## Components Affected

- `cmd/daemon/daemon.go` -- add runner completion signaling and wait between runner cancel and return
- `internal/cli/root.go` -- no change needed (already prints to stderr)

## Risks

- If `jobs.Runner.Start()` does not exit promptly after context cancellation (e.g., blocks on a long-running job), the shutdown timeout will eventually force exit. This is acceptable.
- The `@slow.com` QA simulation (30-second delay) exceeds the 15-second shutdown timeout and will be interrupted. This is acceptable for QA simulation.

## Alternatives Considered

- Increasing the shutdown timeout to 35+ seconds to accommodate `@slow.com`: rejected because it would make normal shutdown unnecessarily slow for a QA-only edge case.
- Making the ConsoleSender delay context-aware so it aborts early on shutdown: a good improvement but out of scope for this fix.
