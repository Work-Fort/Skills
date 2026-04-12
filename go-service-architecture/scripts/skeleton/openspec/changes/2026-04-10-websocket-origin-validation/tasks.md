# WebSocket Origin Validation -- Tasks

## Task Breakdown

1. Update `HandleWS` to accept and forward allowed origins
   - Files: `internal/infra/ws/handler.go`
   - Change function signature from `HandleWS(hub *Hub, connCtx context.Context)` to `HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string)`
   - Replace `websocket.Accept(w, r, nil)` with `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: allowedOrigins})`
   - Verification: `go build ./internal/infra/ws/...` compiles. Existing tests will fail until updated (expected).

2. Wire allowed origins through daemon configuration
   - Files: `cmd/daemon/daemon.go`
   - Read `ws.allowed-origins` from koanf (supports YAML list and env var `NOTIFIER_WS_ALLOWED_ORIGINS`)
   - Default to `[]string{"localhost:*"}` when not configured
   - Pass the resolved list to `ws.HandleWS(hub, hubCtx, allowedOrigins)` in the mux registration
   - Verification: start daemon without config, verify default applies. Set env var, verify override applies.

3. Update handler tests for new signature
   - Files: `internal/infra/ws/handler_test.go`
   - Update all `HandleWS(hub, ctx)` calls to `HandleWS(hub, ctx, allowedOrigins)` with appropriate test origins
   - Add a test case that verifies a connection from a disallowed origin is rejected
   - Verification: `go test ./internal/infra/ws/...` passes.

4. Add integration/E2E verification for origin rejection
   - Files: `cmd/daemon/daemon_test.go` (or new test file)
   - Test that a WebSocket dial with a mismatched `Origin` header is rejected
   - Test that a WebSocket dial with a matching `Origin` header succeeds
   - Verification: `go test ./cmd/daemon/...` passes.
