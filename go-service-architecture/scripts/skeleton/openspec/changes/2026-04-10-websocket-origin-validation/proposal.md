# WebSocket Origin Validation

## Summary

Add origin validation to the WebSocket upgrade endpoint to prevent cross-site WebSocket hijacking. Currently `websocket.Accept` is called with `nil` options, which disables all origin checks.

## Motivation

Any webpage on any domain can currently open a WebSocket connection to the service and receive all broadcast state-change events. This is a cross-site WebSocket hijacking vulnerability (future work #11, classified HIGH). Browsers send cookies and ambient credentials automatically on WebSocket upgrades, so a malicious page can silently connect and exfiltrate real-time notification data.

## Affected Specs

- `openspec/specs/notification-realtime/spec.md` -- REQ-003 updated, REQ-020 through REQ-023 added, four new scenarios added.

## Scope

**In scope:**
- Passing `websocket.AcceptOptions{OriginPatterns: [...]}` instead of `nil` in `HandleWS`.
- Adding a configuration key (`ws.allowed-origins`) with env var override for allowed origin patterns.
- Defaulting to `localhost:*` when no origins are configured.
- Updating `HandleWS` function signature to accept allowed origins.

**Out of scope:**
- Authentication or authorization on the WebSocket endpoint (separate concern).
- Per-user filtering of broadcast messages.
- Rate limiting on WebSocket upgrades.
