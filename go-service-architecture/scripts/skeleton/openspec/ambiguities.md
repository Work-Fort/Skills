# Ambiguity Report

## Environment Variable Prefix -- RESOLVED

**Source says:** The architecture reference uses `MYSERVICE_` as the env var prefix in the koanf example. The overview document names the project "Notification Service" but does not specify a prefix.
**Ambiguity:** What is the exact environment variable prefix for configuration? `NOTIFY_`, `NOTIFICATION_`, `NTF_`, or something else?
**Decision made:** `NOTIFIER_`. The service is called "Notifier". Updated in service-cli spec (REQ-007 and scenario).
**Alternative interpretation:** The prefix could be the full service binary name (e.g., `NOTIFYSERVICE_`), or a shorter form like `NTF_`.
**Impact if wrong:** Every configuration example in documentation and deployment scripts would use the wrong prefix. Environment variables would silently fail to load.

## Reset Endpoint HTTP Status Code on Success -- RESOLVED

**Source says:** "A reset endpoint clears the notification record, allowing the address to be notified again." The overview does not specify the success status code.
**Ambiguity:** What HTTP status code does a successful reset return? 200 OK, 202 Accepted, or 204 No Content?
**Decision made:** HTTP 204 No Content with an empty response body. Updated in notification-management spec (REQ-007a and reset scenario). Supersedes the earlier assumption of HTTP 200.
**Alternative interpretation:** HTTP 200 with the updated notification object could be useful for immediate UI updates.
**Impact if wrong:** Clients and E2E tests would assert the wrong status code.

## Reset Side Effects — Which Fields Are Cleared -- RESOLVED

**Source says:** "A reset endpoint clears the notification record, allowing the address to be notified again." The overview mentions retry count but does not enumerate all fields that are reset.
**Ambiguity:** Beyond setting state to `pending`, which fields are cleared? The notification record has at minimum: state, retry_count, delivery results, timestamps (created_at, updated_at). Are there additional fields?
**Decision made:** Confirmed: state to `pending`, `retry_count` to 0, delivery results cleared, `created_at` and ID preserved. Already correct in specs (notification-management REQ-004 through REQ-006).
**Alternative interpretation:** A reset could delete the record entirely and create a new one (new ID). Or it could preserve retry_count as a historical counter.
**Impact if wrong:** If the reset does not clear retry_count, a notification that failed after 3 retries would immediately fail again after reset because it is already at the limit. If the reset deletes the record, audit log foreign keys could break.

## not_sent as Terminal State for Reset -- RESOLVED

**Source says:** The state machine diagram shows "any terminal -> pending (manual reset via /v1/notify/reset)." The overview also says not_sent has automatic retry via the queue.
**Ambiguity:** Is `not_sent` considered a terminal state for purposes of the reset transition? The overview says "any terminal -> pending" but also describes `not_sent` as having automatic retry, which makes it non-terminal.
**Decision made:** Confirmed: `not_sent` is eligible for reset. It is non-terminal (has automatic retry), but `not_sent -> pending` is permitted as an explicit override. Auto-retry can still proceed to `sent` state. Already correct in specs (notification-state-machine REQ-008, REQ-025).
**Alternative interpretation:** `not_sent` could be excluded from the reset path entirely, since automatic retry will eventually resolve it (to either delivered or failed).
**Impact if wrong:** If `not_sent -> pending` is not permitted, users cannot manually intervene on a retrying notification. If it is permitted but the queue still has the job, a race condition could occur between the retry and the reset.

## Retry Limit — When Does the Transition to Failed Happen -- RESOLVED

**Source says:** "After reaching the configured retry limit (default 3), the notification transitions to failed permanently."
**Ambiguity:** Does the transition to `failed` happen when retry_count equals the limit (on the Nth attempt) or when it exceeds the limit (on the N+1th attempt)? With a limit of 3: does the notification get 3 total attempts or 3 retries (4 total attempts)?
**Decision made:** 3 retries + 1 initial = 4 total attempts. The initial attempt is NOT counted as a retry. When `retry_count` reaches `retry_limit` (e.g., 3 == 3), the next failure transitions to `failed` instead of `not_sent`. Updated in notification-state-machine spec (REQ-018) and overview.
**Alternative interpretation:** The limit could mean 3 total attempts (initial + 2 retries). Or the check could happen before the attempt rather than after.
**Impact if wrong:** Off-by-one in retry count. The notification either retries one too many or one too few times. E2E tests for retry exhaustion would fail.

## Goqite Queue Name -- RESOLVED

**Source says:** The architecture reference shows `Name: "callbacks"` in the goqite example. The overview describes email delivery jobs but does not name the queue.
**Ambiguity:** What is the queue name for notification email jobs?
**Decision made:** `"notifications"`. Updated in notification-delivery spec (REQ-010).
**Alternative interpretation:** Could be `"email"`, `"send_email"`, or `"callbacks"`.
**Impact if wrong:** Minimal — the queue name is internal. But if it matters for monitoring or debugging, a consistent name should be chosen.

## Goqite MaxReceive and Timeout Values -- RESOLVED

**Source says:** The architecture reference shows `MaxReceive: 3, Timeout: 10 * time.Second` in the goqite example. The overview says "default 3" for retry limit.
**Ambiguity:** Is the goqite `MaxReceive` the same as the notification retry limit? Should the retry limit be enforced by goqite's MaxReceive or by application logic (checking retry_count)?
**Decision made:** Application-level enforcement. Set goqite `MaxReceive` higher (e.g., 5-10) as a safety net. The job handler checks `retry_count >= retry_limit`, transitions to `failed`, and returns `nil` to acknowledge the message. Updated in notification-delivery spec (REQ-013a).
**Alternative interpretation:** goqite's `MaxReceive` could be set equal to the retry limit, letting goqite enforce the maximum. But then the application would not record the `sending -> failed` transition.
**Impact if wrong:** If MaxReceive is set to 3 and the application also checks retry_count at 3, goqite might drop the message before the application can transition it to `failed`, leaving it stuck in `not_sent` with no future retry.

## Job Runner Configuration Values -- RESOLVED

**Source says:** The architecture reference shows `Limit: 5, PollInterval: 500 * time.Millisecond` for the job runner.
**Ambiguity:** Are these the values for the notification service, or just defaults from the example?
**Decision made:** `Limit: 5`, `PollInterval: 500ms`. These are the values for this service. Updated in notification-delivery spec (REQ-013).
**Alternative interpretation:** The notification service might need different values, especially since each job includes a 6-second artificial delay (a Limit of 5 means up to 5 concurrent 6-second sends).
**Impact if wrong:** If the runner limit is 1, the service processes notifications sequentially at 6 seconds each, which could be too slow for QA demonstrations. If too high, SQLite concurrency limits could cause SQLITE_BUSY errors.

## Request ID — Response Header vs Body -- RESOLVED

**Source says:** "The request ID SHALL be included in the API response (in the response body or a header)." The overview mentions request ID propagation but does not specify the response format.
**Ambiguity:** Is the request ID returned in a response header (e.g., `X-Request-ID`), in the JSON response body, or both?
**Decision made:** `X-Request-ID` response header only, not in body. Updated in service-observability spec (REQ-006).
**Alternative interpretation:** Best practice for APIs is to return it in both a response header and the body. Or it could be only in the header to keep the body focused on domain data.
**Impact if wrong:** Clients that expect the request ID in a specific location will not find it. E2E tests need to know where to look.

## Graceful Shutdown Order -- RESOLVED

**Source says:** The architecture reference shows: shut down HTTP server, then cancel job runner context, then close store. The overview mentions graceful shutdown and connection draining.
**Ambiguity:** Where do the WebSocket hub shutdown and MCP handler shutdown fit in the sequence?
**Decision made:** Confirmed order: HTTP server shutdown, MCP handler shutdown, job runner cancel, hub context cancel, DB close. Already correct in specs.
**Alternative interpretation:** The hub could shut down before the MCP handler, or they could shut down concurrently.
**Impact if wrong:** Shutting down the store before the job runner could cause in-flight jobs to fail with database errors. Shutting down the hub before the HTTP server could cause WebSocket upgrade attempts to fail during drain.

## Brand Color Sharing Mechanism -- RESOLVED

**Source says:** "The email templates extract the brand palette from a shared config and apply it via go-premailer CSS inlining." And "Email templates share the same brand colors and styling as the dashboard."
**Ambiguity:** What is the exact mechanism for sharing colors between the Go email renderer and the React frontend? Is it a shared JSON file, a Go template variable, CSS custom properties, or a Tailwind config export?
**Decision made:** Shared `brand.json` file at the project root. Tailwind config imports it for custom color tokens. Go reads it via `go:embed` and passes values to `html/template`; go-premailer inlines the concrete color values. Updated in notification-delivery spec (REQ-021) and frontend-dashboard spec (REQ-024).
**Alternative interpretation:** Could be Go constants exported to a generated TypeScript file. Could be CSS custom properties duplicated in both.
**Impact if wrong:** If the mechanism is not shared, brand colors will drift between email and dashboard. Manual duplication is error-prone.

## Cursor-Based Pagination Format -- RESOLVED

**Source says:** "Paginated list endpoint — Cursor-based pagination" in the architecture stack table. No further detail on the cursor format.
**Ambiguity:** What is the cursor format? Opaque base64-encoded string? Timestamp-based? ID-based? What are the query parameter names (`cursor`, `after`, `page_token`)?
**Decision made:** Query params `after` (base64-encoded `next_cursor` from a previous response) and `limit`. Response body includes `meta: {has_more: true, next_cursor: "base64"}`. Updated in notification-management spec (REQ-009).
**Alternative interpretation:** The huma framework may have conventions for pagination parameters. The cursor could be the last notification ID, a timestamp, or an encoded composite key.
**Impact if wrong:** Frontend API client and MCP tool would use wrong parameter names. E2E tests would send wrong pagination parameters.

## Email Validation Rules -- RESOLVED

**Source says:** "Email addresses are validated for format before accepting. Invalid addresses return 422 Unprocessable Entity."
**Ambiguity:** What constitutes a valid email format? RFC 5322 full compliance? A simple regex? Go's `mail.ParseAddress`? Does it check for MX records?
**Decision made:** `net/mail.ParseAddress` from the Go standard library. No MX record checking. Updated in notification-delivery spec (REQ-006).
**Alternative interpretation:** Could use a strict RFC 5322 regex or a third-party validation library. MX record checking would add latency and external dependency.
**Impact if wrong:** Over-strict validation rejects valid addresses. Over-lenient validation accepts invalid addresses that will fail at SMTP time. Choosing the wrong library could reject edge cases (plus addressing, international domains).

## Reset Endpoint Response Body -- RESOLVED

**Source says:** The overview describes the reset endpoint's error cases (404 for not found) but not what a successful reset returns.
**Ambiguity:** What is the response body on successful reset? The notification object? An empty body? A confirmation message?
**Decision made:** HTTP 204 No Content, empty body. Updated in notification-management spec (REQ-007a and reset scenario).
**Alternative interpretation:** Could return the updated notification object (consistent with REST conventions) or a simple `{"status": "reset"}` message.
**Impact if wrong:** Frontend resend button may expect the updated notification in the response to update the UI immediately. If the body is empty, the frontend would need to re-fetch.
