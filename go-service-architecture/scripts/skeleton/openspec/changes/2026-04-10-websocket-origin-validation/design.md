# WebSocket Origin Validation -- Design

## Approach

The `coder/websocket` library (v1.8.14) supports origin validation natively via `websocket.AcceptOptions{OriginPatterns: []string{...}}`. The library checks the `Origin` header against the provided patterns and rejects the upgrade if no pattern matches.

The fix threads an `allowedOrigins []string` parameter from the daemon configuration through to `HandleWS`, which passes it to `websocket.Accept`. No custom origin-checking logic is needed.

## Components Affected

- `internal/infra/ws/handler.go` -- Change `HandleWS` signature to accept `allowedOrigins []string`. Replace `websocket.Accept(w, r, nil)` with `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: allowedOrigins})`.
- `cmd/daemon/daemon.go` -- Read `ws.allowed-origins` from koanf config. Pass the value to `HandleWS`. Default to `[]string{"localhost:*"}` when not configured.
- `internal/infra/ws/handler_test.go` -- Update all `HandleWS` call sites to pass an allowed origins parameter. Tests using `httptest.NewServer` originate from `127.0.0.1`, so tests must pass an appropriate pattern or use `InsecureSkipVerify` for test convenience.

## Risks

- **Test breakage.** Existing tests call `HandleWS(hub, ctx)` without origins. All call sites must be updated. Tests that connect via `httptest.NewServer` may need `"localhost:*"` or `"127.0.0.1:*"` in the allowed origins list to continue passing.
- **Operator misconfiguration.** If an operator sets `ws.allowed-origins` to an empty list, all WebSocket connections will be rejected. The default of `["localhost:*"]` mitigates this for development but production deployments must explicitly configure their domain.

## Alternatives Considered

- **Wildcard `*` to accept all origins.** This is what `nil` options currently does. Rejected because it defeats the purpose of origin validation entirely.
- **Middleware-based origin check.** A custom middleware could inspect the `Origin` header before the upgrade. Rejected because the library already provides this capability natively, and duplicating the check adds complexity without benefit.
- **Compile-time origin list via build tags.** Hardcode allowed origins per build type (dev, QA, production). Rejected because origins are deployment-specific (the production domain varies), so they must be configurable at runtime.
