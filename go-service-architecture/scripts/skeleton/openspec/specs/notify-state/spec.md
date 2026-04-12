# Notification State Machine

## Purpose

Each notification has a state machine that tracks its lifecycle from creation through delivery or failure. The state machine uses the `qmuntal/stateless` library with external storage, persisting state in the database. All state transitions are logged to an audit table for traceability.

## Requirements

- REQ-1: The system SHALL model each notification with a state machine having the states: `pending`, `sending`, `delivered`, `failed`, and `not_sent`.
- REQ-2: The system SHALL use `qmuntal/stateless` with external storage (accessor/mutator pattern) for the state machine implementation.
- REQ-3: The state machine SHALL enforce the following transitions:
  - `pending` to `sending` (trigger: send initiated)
  - `sending` to `delivered` (trigger: SMTP success)
  - `sending` to `failed` (trigger: permanent SMTP failure or `@example.com` domain)
  - `sending` to `not_sent` (trigger: transient/soft SMTP failure)
  - `not_sent` to `sending` (trigger: automatic retry via goqite visibility timeout)
  - `pending` to `failed` (trigger: `@example.com` domain detected before send)
- REQ-4: The state machine SHALL reject any transition not explicitly permitted, returning an error.
- REQ-5: The state machine configuration SHALL live in the domain layer, with accessor and mutator functions provided by the infra layer.
- REQ-6: The system SHALL persist the current state of each notification in the database (SQLite and PostgreSQL).
- REQ-7: The system SHALL log every state transition to a `state_transitions` audit table, recording: entity type, entity ID, from-state, to-state, trigger, and timestamp.
- REQ-8: The system SHALL transition `@example.com` email notifications to `failed` state without passing through `sending`.
- REQ-9: The state machine states and triggers SHALL be defined as iota enums in the domain package.
- REQ-10: The system SHALL support a `reset` trigger that transitions from any terminal state (`delivered`, `failed`, or `not_sent`) back to `pending`.
- REQ-11: The system SHALL maintain a retry count on each notification record, incremented each time the notification transitions from `not_sent` to `sending`.
- REQ-12: The system SHALL enforce a configurable retry limit (default 3). When the retry count reaches the retry limit, the notification SHALL transition from `not_sent` to `failed` permanently instead of retrying.
- REQ-13: The retry count and retry limit SHALL be visible in API responses and the dashboard.

## Scenarios

#### Scenario: Normal delivery lifecycle
- GIVEN a notification for "user@real.com" is created in `pending` state
- WHEN the background worker picks up the job
- THEN the state SHALL transition from `pending` to `sending`
- AND when SMTP delivery succeeds, the state SHALL transition from `sending` to `delivered`
- AND both transitions SHALL be recorded in the audit table

#### Scenario: Failed delivery lifecycle
- GIVEN a notification for "user@real.com" is in `sending` state
- WHEN the SMTP delivery fails
- THEN the state SHALL transition from `sending` to `failed`
- AND the transition SHALL be recorded in the audit table

#### Scenario: Example.com fast-fail
- GIVEN a notification for "test@example.com" is created in `pending` state
- WHEN the background worker processes the job
- THEN the state SHALL transition directly from `pending` to `failed`
- AND the state SHALL NOT pass through `sending`
- AND the transition SHALL be recorded in the audit table

#### Scenario: Invalid transition rejected
- GIVEN a notification is in `delivered` state
- WHEN a `send` trigger is fired
- THEN the state machine SHALL return an error
- AND the state SHALL remain `delivered`

#### Scenario: Transition audit trail
- GIVEN a notification has transitioned through `pending` to `sending` to `delivered`
- WHEN the audit table is queried for that notification ID
- THEN it SHALL contain exactly two transition records
- AND each record SHALL include the from-state, to-state, trigger, and timestamp

#### Scenario: Reset from terminal state
- GIVEN a notification is in `delivered` or `failed` state
- WHEN the `reset` trigger is fired
- THEN the state SHALL transition back to `pending`
- AND the transition SHALL be recorded in the audit table

#### Scenario: Soft failure transitions to not_sent
- GIVEN a notification for "user@flaky-server.com" is in `sending` state
- WHEN the SMTP delivery encounters a transient failure
- THEN the state SHALL transition from `sending` to `not_sent`
- AND the transition SHALL be recorded in the audit table

#### Scenario: Automatic retry from not_sent
- GIVEN a notification is in `not_sent` state with retry count below the retry limit
- WHEN the goqite visibility timeout expires and the job becomes visible again
- THEN the state SHALL transition from `not_sent` to `sending`
- AND the retry count SHALL be incremented by 1
- AND both the transition and updated retry count SHALL be recorded

#### Scenario: Retry limit exceeded transitions to failed
- GIVEN a notification is in `not_sent` state with retry count equal to the retry limit (default 3)
- WHEN the next retry attempt is triggered
- THEN the state SHALL transition from `not_sent` to `failed` permanently
- AND the notification SHALL NOT be retried again
- AND the transition SHALL be recorded in the audit table

#### Scenario: Reset from not_sent state
- GIVEN a notification is in `not_sent` state
- WHEN the `reset` trigger is fired
- THEN the state SHALL transition back to `pending`
- AND the retry count SHALL be reset to zero
- AND the transition SHALL be recorded in the audit table
