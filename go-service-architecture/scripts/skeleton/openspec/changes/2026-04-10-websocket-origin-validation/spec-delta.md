# WebSocket Origin Validation -- Spec Delta

## notification-realtime/spec.md

### Requirements Changed

- REQ-003: The WebSocket endpoint SHALL accept connections via `websocket.Accept(w, r, nil)`.
+ REQ-003: The WebSocket endpoint SHALL accept connections via `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: origins})` where `origins` is the list of allowed origin patterns. Passing `nil` options is prohibited because it disables origin checking, enabling cross-site WebSocket hijacking.

### Requirements Added

+ REQ-020: The `HandleWS` function SHALL accept an `allowedOrigins []string` parameter that is forwarded to `websocket.AcceptOptions{OriginPatterns: allowedOrigins}`. The function signature SHALL be `HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string) http.HandlerFunc`.
+ REQ-021: The daemon SHALL read allowed WebSocket origins from configuration key `ws.allowed-origins` (env: `NOTIFIER_WS_ALLOWED_ORIGINS`). The value SHALL be a YAML list of origin patterns (e.g., `["localhost:*", "example.com"]`).
+ REQ-022: When `ws.allowed-origins` is not configured, the default SHALL be `["localhost:*"]`. This permits local development without explicit configuration while rejecting cross-origin connections in production if the operator forgets to configure origins.
+ REQ-023: The `coder/websocket` library's `OriginPatterns` field supports exact hostnames and `*` wildcards in the port position (e.g., `localhost:*` matches any port on localhost). The spec defers to the library's pattern matching semantics.

### Scenarios Added

+ **Connection accepted from allowed origin**
+   Given the server is configured with `ws.allowed-origins: ["app.example.com"]`
+   When a browser client at `https://app.example.com` sends a WebSocket upgrade request to `/v1/ws`
+   Then the server SHALL accept the upgrade
+   And the client SHALL be registered with the hub

+ **Connection rejected from disallowed origin**
+   Given the server is configured with `ws.allowed-origins: ["app.example.com"]`
+   When a browser client at `https://evil.attacker.com` sends a WebSocket upgrade request to `/v1/ws`
+   Then `websocket.Accept` SHALL reject the upgrade with an HTTP error response
+   And the client SHALL NOT be registered with the hub

+ **Localhost allowed by default when no origins configured**
+   Given the `ws.allowed-origins` configuration key is absent
+   When a browser client at `http://localhost:8080` sends a WebSocket upgrade request to `/v1/ws`
+   Then the server SHALL accept the upgrade using the default origin pattern `localhost:*`

+ **Non-localhost rejected by default when no origins configured**
+   Given the `ws.allowed-origins` configuration key is absent
+   When a browser client at `https://external-site.com` sends a WebSocket upgrade request to `/v1/ws`
+   Then `websocket.Accept` SHALL reject the upgrade with an HTTP error response
