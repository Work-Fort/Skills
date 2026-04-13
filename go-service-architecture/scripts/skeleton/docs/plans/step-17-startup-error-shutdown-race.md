---
type: plan
step: "17"
title: "Startup error reporting and shutdown race fix"
status: pending
assessment_status: complete
provenance:
  source: issue
  issue_id: "18, 21"
  roadmap_step: "17"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-2-cli-and-database
  - step-3-notification-delivery
---

# Step 17: Startup Error Reporting and Shutdown Race Fix

## Overview

Fix two reliability bugs. Issue #18: startup errors were silently
swallowed because `SilenceErrors: true` suppressed Cobra output
but no replacement stderr print existed. This was already fixed in
code -- `root.go` now has `fmt.Fprintln(os.Stderr, err)` before
`os.Exit(1)`. The spec was updated to match (REQ-016). This plan
verifies no further code change is needed.

Issue #21: during graceful shutdown the database store closes (via
`defer`) before the job runner goroutine finishes in-flight jobs,
producing `sql: database is closed` errors. The fix adds a
configurable `--shutdown-timeout` flag (default 60s), replaces the
hardcoded 15-second HTTP drain timeout, adds a done channel around
the runner goroutine, and waits for it bounded by the shutdown
timeout before the store closes. One timeout governs the entire
shutdown sequence.

Satisfies REQ-016 (verify only), REQ-017, REQ-018, REQ-019.

**Spec delta:** REQ-019 currently says "at least 15 seconds". It
must be updated to reflect the configurable timeout (default 60s)
and the new `--shutdown-timeout` flag, `NOTIFIER_SHUTDOWN_TIMEOUT`
env var, and YAML `shutdown.timeout` config key. The spec-writer
agent should update REQ-019 and add a new requirement for the flag.

## Prerequisites

- Steps 1-16 implemented and passing.
- Spec `service-cli/spec.md` already updated with REQ-016 through
  REQ-019 and new scenarios.

## Tasks

### Task 1: Verify startup error output (issue #18)

**Files:**
- Read-only: `internal/cli/root.go`

**Step 1: Confirm existing code satisfies REQ-016**

Read `internal/cli/root.go`. Verify:
1. `SilenceErrors: true` is set on the root command (line 37).
2. `fmt.Fprintln(os.Stderr, err)` exists in `Execute()` before
   `os.Exit(1)` (lines 49-50).

Expected: Both conditions hold. No code change needed.

**Step 2: Manual smoke test**

Run:
```bash
mkdir -p /tmp/notifier-test-unwritable/noperm && chmod 000 /tmp/notifier-test-unwritable/noperm
XDG_STATE_HOME=/tmp/notifier-test-unwritable/noperm mise run build:go && \
  XDG_STATE_HOME=/tmp/notifier-test-unwritable/noperm ./notifier daemon
```

Expected: Error message printed to stderr, exit code 1, no Cobra
usage output. Clean up with `chmod 755 /tmp/notifier-test-unwritable/noperm && rm -rf /tmp/notifier-test-unwritable`.

### Task 2: Add --shutdown-timeout flag and resolveDuration helper

**Files:**
- Modify: `cmd/daemon/daemon.go:32-49` (NewCmd)
- Modify: `cmd/daemon/daemon.go:51-87` (runWithFS)
- Modify: `cmd/daemon/daemon.go:268-288` (add resolveDuration after resolveInt)

**Step 1: Add the flag to NewCmd**

In `NewCmd`, add the `--shutdown-timeout` flag after the existing
`--dev-url` flag (line 48):

```go
	cmd.Flags().Int("shutdown-timeout", 60, "Shutdown timeout in seconds")
```

**Step 2: Add resolveDuration helper**

Add after the `resolveInt` function (after line 288):

```go
// resolveDuration reads a timeout value (in seconds) from koanf or
// cobra flag and returns it as a time.Duration.
func resolveDuration(cmd *cobra.Command, key string) time.Duration {
	seconds := resolveInt(cmd, key)
	return time.Duration(seconds) * time.Second
}
```

**Step 3: Resolve the flag in runWithFS**

In `runWithFS`, after the `devURL` resolution (line 63), add:

```go
	shutdownTimeout := resolveDuration(cmd, "shutdown-timeout")
```

**Step 4: Pass shutdownTimeout through ServerConfig**

Add `ShutdownTimeout time.Duration` to `ServerConfig` (after
`DevURL string`):

```go
type ServerConfig struct {
	Bind            string
	Port            int
	DSN             string
	SMTPHost        string
	SMTPPort        int
	SMTPFrom        string
	Version         string
	Dev             bool
	DevURL          string
	ShutdownTimeout time.Duration
	FrontendFS      embed.FS
}
```

Update the `RunServer` call in `runWithFS` to include:

```go
		ShutdownTimeout: shutdownTimeout,
```

**Step 5: Verify build**

Run: `mise run build:go`
Expected: Build succeeds with no errors.

**Step 6: Commit**

`feat(daemon): add --shutdown-timeout flag with koanf precedence`

### Task 3: Wait for runner goroutine before closing store (issue #21)

**Files:**
- Modify: `cmd/daemon/daemon.go:174-175` (runner goroutine)
- Modify: `cmd/daemon/daemon.go:242-265` (shutdown sequence)

**Step 1: Add done channel around runner goroutine**

Replace the runner goroutine block at lines 174-175:

```go
	// Start the job runner in a goroutine.
	go runner.Start(runnerCtx)
```

With:

```go
	// Start the job runner in a goroutine.
	// runnerDone closes when Start returns, signalling that all
	// in-flight jobs have finished (REQ-017, REQ-018).
	runnerDone := make(chan struct{})
	go func() {
		runner.Start(runnerCtx)
		close(runnerDone)
	}()
```

**Step 2: Replace hardcoded 15s timeout with configurable value**

Replace line 242:

```go
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
```

With:

```go
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 60 * time.Second
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
```

**Step 3: Wait for runner completion in shutdown sequence**

Replace the shutdown tail at lines 260-265:

```go
	// 4. Cancel runner -- job runner drains current job, then exits.
	runnerCancel()

	// 5. Close database (store.Close) happens in the existing defer
	//    block above.
	return nil
```

With:

```go
	// 4. Cancel runner -- job runner drains current job, then exits.
	runnerCancel()

	// 5. Wait for in-flight jobs to finish, bounded by shutdown
	//    timeout so a stuck job cannot block forever (REQ-018).
	select {
	case <-runnerDone:
		slog.Info("runner stopped")
	case <-shutdownCtx.Done():
		slog.Warn("shutdown timeout waiting for runner")
	}

	// 6. Close database (store.Close) happens in the existing defer
	//    block above.
	return nil
```

**Step 4: Verify build**

Run: `mise run build:go`
Expected: Build succeeds with no errors.

**Step 5: Run existing tests**

Run: `mise run test:unit`
Expected: All tests pass.

**Step 6: Commit**

`fix(daemon): wait for in-flight jobs before closing store on shutdown`

### Task 4: Add integration test for graceful shutdown

**Files:**
- Create: `cmd/daemon/shutdown_test.go`

**Step 1: Write the test**

```go
package daemon

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/config"
)

func TestGracefulShutdownWaitsForRunner(t *testing.T) {
	// Set up temp XDG dirs — same pattern as TestDaemonHealthEndpoint.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := config.InitDirs(); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(); err != nil {
		t.Fatal(err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	dbPath := filepath.Join(tmp, "state", "notifier", "test.db")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := ServerConfig{
		Bind:            "127.0.0.1",
		Port:            port,
		DSN:             dbPath,
		SMTPHost:        "127.0.0.1",
		SMTPPort:        1025,
		SMTPFrom:        "test@localhost",
		Version:         "test",
		ShutdownTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, cfg)
	}()

	// Give the server a moment to start.
	time.Sleep(200 * time.Millisecond)

	// Cancel the context to trigger shutdown.
	cancel()

	// Wait for RunServer to return.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunServer returned error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("RunServer did not return within 20 seconds after cancel")
	}
}
```

**Step 2: Run the test**

Run: `mise exec -- go test -run TestGracefulShutdownWaitsForRunner ./cmd/daemon/ -count=1 -timeout=30s`
Expected: PASS -- RunServer returns cleanly within the timeout.

**Step 3: Commit**

`test(daemon): add graceful shutdown integration test`

### Task 5: Update REQ-019 spec delta

**Files:**
- Modify: `openspec/specs/service-cli/spec.md` (REQ-019 and related scenario)

**Step 1: Update REQ-019 text**

Change REQ-019 from "at least 15 seconds" to reflect the configurable
timeout: default 60 seconds, configurable via `--shutdown-timeout`
flag, `NOTIFIER_SHUTDOWN_TIMEOUT` env var, or `shutdown.timeout` YAML
config key.

**Step 2: Update the "Shutdown timeout prevents stuck jobs" scenario**

Update the scenario to use the configurable default of 60 seconds
instead of the hardcoded 15 seconds.

**Step 3: Commit**

`docs(spec): update REQ-019 to reflect configurable shutdown timeout`

## Verification Checklist

- [ ] `internal/cli/root.go` has `fmt.Fprintln(os.Stderr, err)` before `os.Exit(1)` and `SilenceErrors: true`
- [ ] `cmd/daemon/daemon.go` has `--shutdown-timeout` flag with default 60
- [ ] `resolveDuration` helper follows the same koanf precedence pattern as `resolveInt`
- [ ] `ShutdownTimeout` field added to `ServerConfig`
- [ ] Hardcoded `15*time.Second` replaced with `cfg.ShutdownTimeout`
- [ ] `runnerDone` channel created; shutdown waits on it bounded by `shutdownCtx`
- [ ] `NOTIFIER_SHUTDOWN_TIMEOUT` env var works (koanf maps it automatically)
- [ ] `mise run build:go` succeeds
- [ ] `mise run test:unit` passes
- [ ] `mise exec -- go test ./cmd/daemon/ -count=1` passes
- [ ] REQ-019 spec updated to reflect configurable shutdown timeout (default 60s)
- [ ] Manual test: send SIGTERM during email delivery, no `sql: database is closed` in logs
