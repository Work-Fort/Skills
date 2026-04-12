# Service Health

## Purpose

The service health capability provides a lightweight liveness and readiness check for the notification service. It exposes an HTTP endpoint that verifies database connectivity, enabling load balancers and orchestrators to determine whether the service can accept traffic.

## Requirements

- REQ-1: The system SHALL expose a GET `/v1/health` endpoint.
- REQ-2: The health endpoint SHALL check database connectivity by calling the `Ping` method on the store.
- REQ-3: The health endpoint SHALL return HTTP 200 with `{"status": "healthy"}` when the database is reachable.
- REQ-4: The health endpoint SHALL return HTTP 503 with `{"status": "unhealthy"}` when the database is unreachable.

## Scenarios

#### Scenario: Healthy database
- GIVEN the service is running and the database is accessible
- WHEN a GET request is made to `/v1/health`
- THEN the system SHALL return HTTP 200
- AND the response body SHALL be `{"status": "healthy"}`

#### Scenario: Unhealthy database
- GIVEN the service is running but the database is unreachable
- WHEN a GET request is made to `/v1/health`
- THEN the system SHALL return HTTP 503
- AND the response body SHALL be `{"status": "unhealthy"}`
