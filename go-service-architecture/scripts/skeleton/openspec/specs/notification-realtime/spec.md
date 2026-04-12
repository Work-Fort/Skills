# Notification Realtime

## Purpose

Provides WebSocket-based real-time push updates to connected browser clients. When a notification's state changes, the update is broadcast to all connected dashboard sessions so users see state transitions live without polling.

## Requirements

### WebSocket Endpoint

- REQ-001: The service SHALL expose `GET /v1/ws` as a WebSocket upgrade endpoint.
- REQ-002: The WebSocket implementation SHALL use `coder/websocket` (ISC license, zero external dependencies). Import path: `github.com/coder/websocket`.
- REQ-003: The WebSocket endpoint SHALL accept connections via `websocket.Accept(w, r, nil)`.

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

### Broadcast

- REQ-013: When a notification state changes, the service layer SHALL broadcast a JSON message to the hub containing at minimum the notification `id` and new `state`.
- REQ-014: The hub SHALL deliver the broadcast message to all connected clients.

### Graceful Shutdown

- REQ-015: When the server shuts down, the hub's context SHALL be cancelled, causing the run loop to exit and write pumps to close connections with `websocket.StatusNormalClosure`.

## Scenarios

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
