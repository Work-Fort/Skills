# Notification State Machine

## Purpose

Manages the lifecycle of a notification through a state machine implemented with `qmuntal/stateless`. Tracks transitions from creation through delivery or failure, supports automatic retries for transient failures, and maintains a complete audit log of all state transitions.

## Requirements

### States

- REQ-001: The notification state machine SHALL define the following states: `pending`, `sending`, `delivered`, `failed`, `not_sent`.
- REQ-002: States and triggers SHALL be defined as iota enums in the domain layer.

### Permitted Transitions

- REQ-003: The transition `pending -> sending` SHALL be permitted (triggered when the background worker picks up the job).
- REQ-004: The transition `sending -> delivered` SHALL be permitted (triggered when SMTP accepts the message).
- REQ-005: The transition `sending -> failed` SHALL be permitted (triggered on permanent failure, e.g., `@example.com` addresses).
- REQ-006: The transition `sending -> not_sent` SHALL be permitted (triggered on transient/soft failure).
- REQ-007: The transition `not_sent -> sending` SHALL be permitted (triggered on automatic retry via goqite visibility timeout).
- REQ-008: The transitions `delivered -> pending`, `failed -> pending`, and `not_sent -> pending` SHALL be permitted (triggered by manual reset via `/v1/notify/reset`). These are the "any terminal -> pending" reset transitions.

### Transitions That SHALL NOT Occur

- REQ-009: The transition `pending -> failed` SHALL NOT be permitted. A notification SHALL NOT skip the `sending` state to reach `failed` directly.
- REQ-010: The transition `pending -> delivered` SHALL NOT be permitted.
- REQ-011: The transition `delivered -> sending` SHALL NOT be permitted.
- REQ-012: The transition `failed -> sending` SHALL NOT be permitted. A failed notification must be reset to `pending` before re-sending.

### stateless Integration

- REQ-013: The state machine SHALL be implemented using `qmuntal/stateless` (Tier 2) with external storage via `accessor` and `mutator` functions.
- REQ-014: The state machine configuration function SHALL live in the domain layer (`internal/domain/`). It SHALL accept `accessor` and `mutator` functions as parameters and SHALL NOT import any infrastructure packages.
- REQ-015: The `accessor` function SHALL read the current state from the database. The `mutator` function SHALL write the new state to the database. Both SHALL be provided by the infra/store layer.
- REQ-016: The state machine SHALL use `stateless.FiringQueued` mode.

### Retry Logic

- REQ-017: Each notification record SHALL track a `retry_count` (number of attempts so far) and the configured `retry_limit`.
- REQ-018: The default retry limit SHALL be 3. This means 3 retries plus 1 initial attempt = 4 total delivery attempts. The initial attempt is NOT counted as a retry.
- REQ-019: Each `sending -> not_sent` transition SHALL increment the `retry_count` on the notification record.
- REQ-020: When `retry_count` reaches the `retry_limit`, the next attempt SHALL transition the notification to `failed` permanently instead of `not_sent`.
- REQ-021: The `retry_count` and `retry_limit` SHALL be visible in the API response and in the dashboard.

### Audit Log

- REQ-022: Every state transition SHALL be recorded in a `state_transitions` audit log table.
- REQ-023: Each audit log entry SHALL contain: `entity_type`, `entity_id`, `from_state`, `to_state`, `trigger`, and `created_at`.

### Terminal States

- REQ-024: The states `delivered` and `failed` SHALL be terminal states (no automatic transitions out).
- REQ-025: The state `not_sent` SHALL NOT be a terminal state; it is a retryable soft-fail state with automatic retry via the queue.

## Scenarios

### Scenario: Successful delivery path

- **Given** a notification exists with state `pending`
- **When** the background worker picks up the job and SMTP accepts the message
- **Then** the state SHALL transition `pending -> sending -> delivered`
- **And** two audit log entries SHALL be recorded (pending->sending and sending->delivered)

### Scenario: Permanent failure path (example.com)

- **Given** a notification exists for `test@example.com` with state `pending`
- **When** the background worker picks up the job
- **Then** the state SHALL transition `pending -> sending -> failed`
- **And** the state SHALL NOT pass through `not_sent`

### Scenario: Transient failure with retry

- **Given** a notification exists with state `pending` and `retry_count` of 0
- **When** the background worker picks up the job and SMTP returns a transient error
- **Then** the state SHALL transition `pending -> sending -> not_sent`
- **And** the `retry_count` SHALL be incremented to 1
- **And** the goqite visibility timeout SHALL make the job available for retry

### Scenario: Retry limit exhausted

- **Given** a notification exists with `retry_count` of 2 and `retry_limit` of 3
- **When** the background worker picks up the job and SMTP returns a transient error
- **Then** the `retry_count` SHALL be incremented to 3
- **And** the state SHALL transition to `failed` (not `not_sent`)
- **And** the notification SHALL NOT be retried again

### Scenario: Automatic retry from not_sent

- **Given** a notification exists with state `not_sent` and `retry_count` of 1
- **When** goqite's visibility timeout expires and the worker picks up the job
- **Then** the state SHALL transition `not_sent -> sending`
- **And** if delivery succeeds, the state SHALL transition `sending -> delivered`

### Scenario: Direct pending-to-failed is rejected

- **Given** a notification exists with state `pending`
- **When** an attempt is made to transition directly to `failed`
- **Then** the state machine SHALL reject the transition
- **And** the state SHALL remain `pending`

### Scenario: Failed notification cannot re-send without reset

- **Given** a notification exists with state `failed`
- **When** an attempt is made to transition to `sending`
- **Then** the state machine SHALL reject the transition
- **And** the state SHALL remain `failed`

### Scenario: Audit log records every transition

- **Given** a notification goes through `pending -> sending -> not_sent -> sending -> delivered`
- **When** the full lifecycle completes
- **Then** the `state_transitions` table SHALL contain exactly 4 entries for this notification
- **And** each entry SHALL have `entity_type` of `"notification"` and the correct `from_state` and `to_state`
