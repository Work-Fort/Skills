# Notification Reset

## Purpose

The notification reset capability allows a previously notified email address to receive another notification. It exposes a REST endpoint that resets the notification record, transitioning the state machine back to `pending` so the email can be re-sent. This supports the dashboard's "Resend" workflow.

## Requirements

- REQ-1: The system SHALL expose a POST `/v1/notify/reset` endpoint that accepts a JSON body containing an `email` field.
- REQ-2: The system SHALL validate that the `email` field is present and non-empty, returning HTTP 422 if validation fails.
- REQ-3: The system SHALL return HTTP 404 if no notification record exists for the given email address. The store layer SHALL return an `ErrNotFound` domain error sentinel, which the HTTP handler maps to 404.
- REQ-4: The system SHALL fire the `reset` trigger on the notification's state machine, transitioning it back to `pending`.
- REQ-5: The system SHALL return HTTP 200 on successful reset with the updated notification record in the response body.
- REQ-6: The system SHALL clear the delivery result so the notification can be re-sent via a subsequent POST to `/v1/notify`.
- REQ-7: After a successful reset, a POST to `/v1/notify` for the same email SHALL be accepted (not rejected as "already notified").
- REQ-8: The state machine transition caused by reset SHALL be recorded in the audit table.
- REQ-9: The system SHALL accept reset from the `not_sent` state in addition to `delivered` and `failed`, transitioning the notification back to `pending` and resetting the retry count to zero.

## Scenarios

#### Scenario: Successful reset of a delivered notification
- GIVEN a notification for "user@real.com" exists in `delivered` state
- WHEN a POST request is made to `/v1/notify/reset` with body `{"email": "user@real.com"}`
- THEN the system SHALL return HTTP 200
- AND the notification state SHALL be `pending`
- AND the state transition SHALL be recorded in the audit table

#### Scenario: Successful reset of a failed notification
- GIVEN a notification for "bad@somewhere.com" exists in `failed` state
- WHEN a POST request is made to `/v1/notify/reset` with body `{"email": "bad@somewhere.com"}`
- THEN the system SHALL return HTTP 200
- AND the notification state SHALL be `pending`

#### Scenario: Successful reset of a not_sent notification
- GIVEN a notification for "user@flaky.com" exists in `not_sent` state with retry count 2
- WHEN a POST request is made to `/v1/notify/reset` with body `{"email": "user@flaky.com"}`
- THEN the system SHALL return HTTP 200
- AND the notification state SHALL be `pending`
- AND the retry count SHALL be reset to zero

#### Scenario: Re-notify after reset
- GIVEN a notification for "user@real.com" has been reset to `pending` state
- WHEN a POST request is made to `/v1/notify` with body `{"email": "user@real.com"}`
- THEN the system SHALL return HTTP 202
- AND a new background job SHALL be enqueued for email delivery

#### Scenario: Reset for nonexistent email returns ErrNotFound
- GIVEN no notification record exists for "unknown@nowhere.com"
- WHEN a POST request is made to `/v1/notify/reset` with body `{"email": "unknown@nowhere.com"}`
- THEN the store SHALL return an `ErrNotFound` domain error
- AND the system SHALL return HTTP 404

#### Scenario: Missing email field on reset
- GIVEN the system is running
- WHEN a POST request is made to `/v1/notify/reset` with an empty body or missing `email` field
- THEN the system SHALL return HTTP 422
