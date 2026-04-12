---
type: plan
step: "12"
title: "WebSocket Origin Validation"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "12"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-6-mcp-and-websocket
  - step-11-security-fixes
---

# Step 12: WebSocket Origin Validation

## Overview

Adds origin validation to the WebSocket upgrade endpoint to close
the cross-site WebSocket hijacking vulnerability identified in future
work #11. Currently `websocket.Accept` is called with `nil` options,
so any webpage on any domain can open a WebSocket and receive all
broadcast state-change events.

The fix threads an `allowedOrigins []string` from daemon configuration
through to `HandleWS`, which passes it to
`websocket.AcceptOptions{OriginPatterns: allowedOrigins}`. The
`coder/websocket` library handles the `Origin` header check natively.

Deliverables:

1. **Handler signature change** -- `HandleWS` accepts
   `allowedOrigins []string` and forwards it to `websocket.Accept`.
   Satisfies REQ-003, REQ-020.

2. **Configuration wiring** -- The daemon reads `ws.allowed_origins`
   from koanf (YAML) or `NOTIFIER_WS_ALLOWED_ORIGINS` (env,
   comma-separated). Defaults to `["localhost:*", "127.0.0.1:*"]`.
   Satisfies REQ-021, REQ-022.

3. **Test updates** -- Existing handler tests pass the correct origin
   patterns. A new test verifies that connections from disallowed
   origins are rejected. Satisfies REQ-023 and the four origin
   validation scenarios in the spec.

## Prerequisites

- Step 6 completed: `HandleWS`, `Hub`, and `Client` exist in
  `internal/infra/ws/`.
- Step 11 completed: `NewHub(maxConns int)` signature and
  `ReadPump` read limit are already in place.

## Tasks

### Task 1: Update `HandleWS` to accept and forward allowed origins

**Files:**
- Modify: `internal/infra/ws/handler.go:18-30`

**Step 1: Update the function signature and Accept call**

Replace the entire `HandleWS` function:

```go
// HandleWS returns an http.HandlerFunc that upgrades HTTP connections
// to WebSocket and registers clients with the hub (REQ-001, REQ-003,
// REQ-020).
//
// The connCtx parameter provides the lifecycle context for all
// connections. After websocket.Accept hijacks the connection,
// r.Context() is unreliable (it may be cancelled when the HTTP
// handler returns). Use a context derived from the hub's lifecycle
// instead.
//
// The allowedOrigins parameter is forwarded to
// websocket.AcceptOptions{OriginPatterns: allowedOrigins}. Passing
// nil disables origin checking and is prohibited (REQ-003).
func HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: allowedOrigins,
		})
		if err != nil {
			return // Accept writes the HTTP error response.
		}
		client := NewClient(hub, conn)
		hub.Register(client)

		go client.WritePump(connCtx)
		client.ReadPump(connCtx) // blocks until disconnect
	}
}
```

**Step 2: Verify the package compiles**

Run: `go build ./internal/infra/ws/...`
Expected: FAIL -- `cmd/daemon/daemon.go` calls `HandleWS(hub, hubCtx)` with two arguments but the signature now requires three. The `ws` package itself compiles. The test file will also fail to compile until Task 3.

### Task 2: Wire allowed origins through daemon configuration

**Files:**
- Modify: `cmd/daemon/daemon.go:56-64` (flag resolution block)
- Modify: `cmd/daemon/daemon.go:203` (mux registration)

**Step 1: Add the `resolveAllowedOrigins` helper**

The koanf env provider converts `NOTIFIER_WS_ALLOWED_ORIGINS` to
`ws.allowed.origins` (three levels deep), but the YAML key
`ws.allowed_origins` resolves to a two-level koanf path. These do
not match. To avoid this mismatch, the helper checks koanf first
(for YAML config), then falls back to reading the environment
variable directly and splitting on commas.

Add the following function after `resolveInt` at the bottom of
`cmd/daemon/daemon.go`:

```go
// resolveAllowedOrigins returns the configured WebSocket origin
// patterns. It checks the koanf YAML path first, then falls back
// to the NOTIFIER_WS_ALLOWED_ORIGINS environment variable (comma-
// separated). If neither is set, returns the default patterns that
// permit local development (REQ-022).
func resolveAllowedOrigins() []string {
	defaultOrigins := []string{"localhost:*", "127.0.0.1:*"}

	// Check koanf for the YAML config path.
	if config.K.Exists("ws.allowed_origins") {
		if origins := config.K.Strings("ws.allowed_origins"); len(origins) > 0 {
			return origins
		}
	}

	// Fall back to the environment variable directly to avoid the
	// koanf underscore-to-dot mapping mismatch (NOTIFIER_WS_ALLOWED_ORIGINS
	// maps to "ws.allowed.origins" in koanf, not "ws.allowed_origins").
	if envVal := os.Getenv("NOTIFIER_WS_ALLOWED_ORIGINS"); envVal != "" {
		var origins []string
		for _, o := range strings.Split(envVal, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) > 0 {
			return origins
		}
	}

	return defaultOrigins
}
```

**Step 2: Call the helper and pass origins to HandleWS**

In the `RunServer` function, after the existing flag resolution
block (after line 63), add:

```go
	allowedOrigins := resolveAllowedOrigins()
```

Then update the mux registration (line 203) from:

```go
	mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx))
```

to:

```go
	mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx, allowedOrigins))
```

**Step 3: Verify the daemon compiles**

Run: `go build ./cmd/daemon/...`
Expected: FAIL -- the test file `internal/infra/ws/handler_test.go` still uses the old two-argument signature. The daemon package itself compiles.

**Step 4: Commit**

`fix(ws): add origin validation to WebSocket upgrade endpoint`

This commit includes Tasks 1 and 2 together since neither compiles
independently (the call site and the signature must change together).

### Task 3: Update handler tests and add origin rejection test

**Files:**
- Modify: `internal/infra/ws/handler_test.go`

**Step 1: Write the origin rejection test**

Add the following test at the end of `handler_test.go`:

```go
func TestHandleWSRejectsDisallowedOrigin(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Only allow example.com -- the test server runs on 127.0.0.1
	// so the Origin header will not match.
	srv := httptest.NewServer(HandleWS(hub, ctx, []string{"example.com"}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	_, _, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail for disallowed origin")
	}
}
```

**Step 2: Update existing tests to pass allowed origins**

All four existing tests use `httptest.NewServer` which binds to
`127.0.0.1` with a random port. The `coder/websocket` client sends
an `Origin` header matching the server's address. Pass
`[]string{"127.0.0.1:*"}` so these connections are accepted.

In `TestHandleWSAcceptsConnection` (line 20), change:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx))
```

to:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx, []string{"127.0.0.1:*"}))
```

In `TestHandleWSClientDisconnect` (line 49), change:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx))
```

to:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx, []string{"127.0.0.1:*"}))
```

In `TestHandleWSMultipleClients` (line 75), change:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx))
```

to:

```go
	srv := httptest.NewServer(HandleWS(hub, ctx, []string{"127.0.0.1:*"}))
```

In `TestHandleWSNonUpgradeRequest` (line 119), change:

```go
	handler := HandleWS(hub, ctx)
```

to:

```go
	handler := HandleWS(hub, ctx, []string{"127.0.0.1:*"})
```

**Step 3: Run tests to verify all pass**

Run: `go test -run TestHandleWS ./internal/infra/ws/...`
Expected: PASS (all five tests, including the new rejection test).

**Step 4: Run the full test suite**

Run: `mise run test:unit`
Expected: PASS -- no other files reference `HandleWS` besides
`daemon.go` (updated in Task 2) and `handler_test.go` (updated
above).

**Step 5: Commit**

`test(ws): update handler tests for origin validation`

### Task 4: Mark future work #11 complete

**Files:**
- Modify: `docs/future-work.md:125-131`

**Step 1: Confirm the item is already marked done**

Future work #11 already has the checkmark suffix. No change needed
if the line reads:

```
## #11 — Security: WebSocket Origin Validation (HIGH) ✅
```

Verify by reading the file. If it already has the checkmark, skip
this task. If not, add ` ✅` to the heading.

**Step 2: Commit (if changed)**

`docs: mark future work #11 as complete`

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings
- [ ] `mise run test:unit` passes -- all existing tests plus the new
      origin rejection test
- [ ] `mise run lint:go` produces no warnings
- [ ] Manual smoke test: start the daemon without config, open the
      dashboard at `localhost:8080`, confirm the WebSocket connects
      (the default `["localhost:*", "127.0.0.1:*"]` permits it)
- [ ] Manual smoke test: set `NOTIFIER_WS_ALLOWED_ORIGINS=example.com`,
      restart daemon, open the dashboard at `localhost:8080`, confirm
      the WebSocket connection is rejected (browser console shows a
      failed upgrade)
