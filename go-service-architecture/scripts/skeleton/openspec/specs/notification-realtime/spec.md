# Notification Realtime

## Purpose

Provides WebSocket-based real-time push updates to connected browser clients. When a notification's state changes, the update is broadcast to all connected dashboard sessions so users see state transitions live without polling.

## Requirements

### WebSocket Endpoint

- REQ-001: The service SHALL expose `GET /v1/ws` as a WebSocket upgrade endpoint.
- REQ-002: The WebSocket implementation SHALL use `coder/websocket` (ISC license, zero external dependencies). Import path: `github.com/coder/websocket`.
- REQ-003: The WebSocket endpoint SHALL accept connections via `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: origins})` where `origins` is the list of allowed origin patterns. Passing `nil` options is prohibited because it disables origin checking, enabling cross-site WebSocket hijacking.

### Origin Validation

- REQ-020: The `HandleWS` function SHALL accept an `allowedOrigins []string` parameter that is forwarded to `websocket.AcceptOptions{OriginPatterns: allowedOrigins}`. The function signature SHALL be `HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string) http.HandlerFunc`.
- REQ-021: The daemon SHALL read allowed WebSocket origins from configuration key `ws.allowed-origins` (env: `NOTIFIER_WS_ALLOWED_ORIGINS`). The value SHALL be a YAML list of origin patterns (e.g., `["localhost:*", "example.com"]`).
- REQ-022: When `ws.allowed-origins` is not configured, the default SHALL be `["localhost:*"]`. This permits local development without explicit configuration while rejecting cross-origin connections in production if the operator forgets to configure origins.
- REQ-023: The `coder/websocket` library's `OriginPatterns` field supports exact hostnames and `*` wildcards in the port position (e.g., `localhost:*` matches any port on localhost). The spec defers to the library's pattern matching semantics.

### Hub Pattern

- REQ-004: The service SHALL implement a hub that manages all connected WebSocket clients.
- REQ-005: The hub SHALL maintain a map of connected clients and provide channels for register, unregister, and broadcast operations.
- REQ-006: The hub's `Run` method SHALL be started as a goroutine in the daemon's `RunE` before the HTTP server starts.
- REQ-007: The hub SHALL accept a `context.Context` and exit its run loop when the context is cancelled.
- REQ-008: The broadcast channel SHALL be buffered (capacity 256).

### Client Management

- REQ-009: Each connected client SHALL have a dedicated send channel (buffered, capacity 256) and separate read/write pump goroutines.
- REQ-010: The write pump SHALL send messages from the client's send channel to the WebSocket connection as `websocket.MessageText`.
- REQ-011: The read pump SHALL detect client disconnects by reading from the connection. When a read error occurs, the client SHALL be unregistered from the hub.
- REQ-012: When a client's send channel is full (non-writable), the hub SHALL drop that client by removing it from the client map and closing its send channel.
- REQ-016: The read pump SHALL call `conn.SetReadLimit(512)` before entering the read loop. Since the server discards all incoming messages, this limits memory consumption from malicious or misbehaving clients sending large frames.

### Connection Limits

- REQ-017: The hub SHALL enforce a maximum number of concurrent client connections. The limit SHALL be configurable and SHALL default to 1000.
- REQ-018: When a registration is received and the number of connected clients is already at the limit, the hub SHALL reject the registration by closing the client's send channel (without adding it to the client map).
- REQ-019: The connection limit SHALL be passed to the hub at construction time (e.g., `NewHub(maxConns int)`).

### Broadcast

- REQ-013: When a notification state changes, the service layer SHALL broadcast a JSON message to the hub containing at minimum the notification `id` and new `state`.
- REQ-014: The hub SHALL deliver the broadcast message to all connected clients.

### Graceful Shutdown

- REQ-015: When the server shuts down, the hub's context SHALL be cancelled, causing the run loop to exit and write pumps to close connections with `websocket.StatusNormalClosure`.

## Scenarios

### Scenario: Connection accepted from allowed origin

- **Given** the server is configured with `ws.allowed-origins: ["app.example.com"]`
- **When** a browser client at `https://app.example.com` sends a WebSocket upgrade request to `/v1/ws`
- **Then** the server SHALL accept the upgrade
- **And** the client SHALL be registered with the hub

### Scenario: Connection rejected from disallowed origin

- **Given** the server is configured with `ws.allowed-origins: ["app.example.com"]`
- **When** a browser client at `https://evil.attacker.com` sends a WebSocket upgrade request to `/v1/ws`
- **Then** `websocket.Accept` SHALL reject the upgrade with an HTTP error response
- **And** the client SHALL NOT be registered with the hub

### Scenario: Localhost allowed by default when no origins configured

- **Given** the `ws.allowed-origins` configuration key is absent
- **When** a browser client at `http://localhost:8080` sends a WebSocket upgrade request to `/v1/ws`
- **Then** the server SHALL accept the upgrade using the default origin pattern `localhost:*`

### Scenario: Non-localhost rejected by default when no origins configured

- **Given** the `ws.allowed-origins` configuration key is absent
- **When** a browser client at `https://external-site.com` sends a WebSocket upgrade request to `/v1/ws`
- **Then** `websocket.Accept` SHALL reject the upgrade with an HTTP error response

### Scenario: Client receives state update in real time

- **Given** a browser client is connected to `/v1/ws`
- **And** a notification for `user@company.com` exists with state `pending`
- **When** the notification transitions to `sending`
- **Then** the WebSocket client SHALL receive a JSON message containing `{"id": "ntf_...", "state": "sending"}`

### Scenario: Multiple clients receive the same broadcast

- **Given** two browser clients are connected to `/v1/ws`
- **When** a notification transitions to `delivered`
- **Then** both clients SHALL receive the state update message

### Scenario: Disconnected client is cleaned up

- **Given** a browser client is connected to `/v1/ws`
- **When** the client closes the WebSocket connection
- **Then** the hub SHALL remove the client from its client map
- **And** the client's send channel SHALL be closed

### Scenario: Slow client is dropped

- **Given** a browser client is connected to `/v1/ws` but is not reading messages
- **When** the client's send channel is full and a new broadcast arrives
- **Then** the hub SHALL remove the slow client
- **And** the hub SHALL close the slow client's send channel
- **And** other connected clients SHALL continue receiving broadcasts

### Scenario: Large incoming frame rejected by read limit

- **Given** a browser client is connected to `/v1/ws`
- **When** the client sends a WebSocket message larger than 512 bytes
- **Then** the `coder/websocket` library SHALL close the connection with a protocol error
- **And** the read pump SHALL detect the read error and unregister the client from the hub

### Scenario: Connection rejected at capacity

- **Given** the hub is configured with a maximum of 1000 connections
- **And** 1000 clients are currently connected
- **When** a new client attempts to register with the hub
- **Then** the hub SHALL NOT add the client to the client map
- **And** the hub SHALL close the rejected client's send channel

### Scenario: Connection accepted below capacity

- **Given** the hub is configured with a maximum of 1000 connections
- **And** 999 clients are currently connected
- **When** a new client attempts to register with the hub
- **Then** the hub SHALL add the client to the client map
- **And** the client SHALL receive subsequent broadcast messages
