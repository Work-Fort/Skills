# Startup Error Reporting and Graceful Shutdown Race Fix -- Spec Delta

## service-cli/spec.md

### Requirements Changed

- REQ-016: The root `Execute()` function SHALL call `os.Exit(1)` if the root command returns an error.
+ REQ-016: The root `Execute()` function SHALL print the error to stderr via `fmt.Fprintln(os.Stderr, err)` and then call `os.Exit(1)` if the root command returns an error. The Cobra root command SHALL set `SilenceErrors: true` so that Cobra does not print its own error message; the `Execute()` wrapper is solely responsible for error output.

### Requirements Added

+ REQ-017: The daemon shutdown sequence SHALL follow this order: (1) drain HTTP connections via `srv.Shutdown`, (2) signal MCP SSE clients, (3) cancel the hub context, (4) cancel the runner context, (5) wait for in-flight jobs to finish, (6) close the database store. The store SHALL NOT be closed until all in-flight job runner goroutines have exited.
+ REQ-018: The daemon SHALL expose a mechanism to wait for in-flight jobs after cancelling the runner context (e.g., `runner.Wait()` or equivalent). The wait SHALL be bounded by the shutdown timeout context so that a stuck job does not block shutdown indefinitely.
+ REQ-019: The shutdown timeout context SHALL allow sufficient time for in-flight jobs to complete, accounting for the 6-second email send delay (notification-delivery REQ-016). The timeout SHALL be at least 15 seconds.

### Scenarios Added

+ **Startup error is visible to the user**
+   Given the XDG state directory is not writable
+   When the binary is started with `daemon`
+   Then the binary SHALL print the error to stderr
+   And the binary SHALL exit with code 1
+   And Cobra SHALL NOT print its own error message (SilenceErrors is true)

+ **Graceful shutdown waits for in-flight jobs**
+   Given the daemon is running and processing an email delivery job with a 6-second send delay
+   When a SIGTERM signal is received
+   Then the HTTP server SHALL stop accepting new connections
+   And the runner context SHALL be cancelled
+   And the daemon SHALL wait for the in-flight job to complete before closing the database store
+   And no `sql: database is closed` errors SHALL occur

+ **Shutdown timeout prevents stuck jobs from blocking forever**
+   Given the daemon is running and a job is stuck (e.g., `@slow.com` 30-second delay)
+   When a SIGTERM signal is received
+   Then the daemon SHALL wait for in-flight jobs up to the shutdown timeout (15 seconds)
+   And if the timeout elapses, the daemon SHALL proceed to close the store and exit

### Scenarios Changed

- **Unknown subcommand**
-   Given no subcommand matching `foobar` is registered
-   When the user runs the binary with `foobar`
-   Then the binary SHALL exit with code 1
+ **Unknown subcommand**
+   Given no subcommand matching `foobar` is registered
+   When the user runs the binary with `foobar`
+   Then the binary SHALL exit with code 1
+   And the error message SHALL be printed to stderr
