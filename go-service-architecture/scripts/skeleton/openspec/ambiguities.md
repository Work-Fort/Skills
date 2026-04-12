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

**Source says:** "The email templates extract the brand palette from a shared config and apply it via Maizzle build-time CSS inlining." And "Email templates share the same brand colors and styling as the dashboard."
**Ambiguity:** What is the exact mechanism for sharing colors between the Go email renderer and the React frontend? Is it a shared JSON file, a Go template variable, CSS custom properties, or a Tailwind config export?
**Decision made:** Shared `brand.json` file at the project root. Tailwind config imports it for dashboard color tokens. Maizzle build imports it to resolve brand colors into inlined CSS at compile time. At runtime, Go only injects dynamic values into pre-compiled email HTML via `html/template` — no runtime CSS processing. Updated in notification-delivery spec (REQ-020, REQ-021) and frontend-dashboard spec (REQ-024).
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

## release:qa Build Flags -- OPEN

**Source says:** Future work item #9 says "Add `.mise/tasks/release/qa` that builds with `-tags spa,qa` and includes the seed data, matching the pattern of `release:dev` and `release:production`." The existing `release:production` uses `CGO_ENABLED=0`, `-ldflags="-s -w -X main.Version=${VERSION}"`, and `-trimpath`. The existing `release:dev` uses only `-race` with no tags.
**Ambiguity:** Should `release:qa` apply production-style flags (`CGO_ENABLED=0`, `-ldflags`, `-trimpath`) or be a simpler debug-friendly build? The source says "matching the pattern" but the two existing tasks have very different flag sets.
**Decision made:** The spec (REQ-025) requires `-tags spa,qa` and output to `build/notifier` but does not mandate production stripping flags or the race detector. The implementer should decide based on usage context (QA testing vs demo distribution).
**Alternative interpretation:** `release:qa` could mirror `release:production` exactly (with `-ldflags`, `-trimpath`, `CGO_ENABLED=0`) but with the additional `qa` tag. Or it could be a debug build with `-race` like `release:dev` but with `-tags spa,qa`.
**Impact if wrong:** If production flags are used, QA builds lose debug symbols and race detection, making QA-discovered bugs harder to diagnose. If debug flags are used, QA builds are larger and slower, which matters for distribution to external testers.

## release:qa Dependency on build:email -- RESOLVED

**Source says:** The existing `release:production` task depends on both `build:web` and `build:email`. Future work item #9 does not mention email template compilation.
**Ambiguity:** Should `release:qa` depend on `build:email` in addition to `build:web`? The QA build embeds the SPA (requiring `build:web`) and sends emails (requiring compiled templates from `build:email`). But the future work item only says "matching the pattern" without listing dependencies explicitly.
**Decision made:** The `release:qa` task DOES need `build:email` as a dependency. Although the `ConsoleSender` (service-build REQ-026) logs email content to the console instead of sending via SMTP, the worker calls `RenderNotification` unconditionally in all builds before passing rendered content to `sender.Send`. The email templates in `internal/infra/email/dist/` are embedded via `//go:embed dist/*.html` and `//go:embed dist/*.txt` in `template.go`, which is compiled into all builds regardless of build tags. Without `build:email`, the `dist/` directory is empty and the `//go:embed` directives fail at compile time. This is a compile-time embedding constraint, not a sender behavior concern.
**Alternative interpretation:** If the `//go:embed` directives were also build-tag-gated or if the worker conditionally skipped template rendering in QA, the dependency could be removed. But this would require invasive changes to unrelated code for minimal benefit.
**Impact if wrong:** If `build:email` is removed from `release:qa`, the QA build fails at compile time with an `//go:embed` pattern match error because no files exist in `dist/`.

## ConsoleSender File Organization -- OPEN

**Source says:** Future work item #7 says "Use the same `//go:build qa` / `//go:build !qa` pattern" and references the seed data pattern (`seed_qa.go` / `seed_default.go`). The current SMTP sender lives in `internal/infra/email/sender.go` with no build tag.
**Ambiguity:** How should the files be organized? The seed pattern uses two files in the same package: `seed_qa.go` (QA implementation) and `seed_default.go` (no-op for non-QA). For the email sender, the non-QA build is not a no-op -- it is the full SMTP sender. Should the file split be `sender_qa.go` (ConsoleSender) / `sender.go` (SMTPSender with `//go:build !qa` added), or should there be a separate factory function file?
**Decision made:** The spec (service-build REQ-026) requires the `seed_qa.go` / `seed_default.go` file pair pattern. This means: `sender_qa.go` with `//go:build qa` containing `ConsoleSender` and a factory function, and `sender_default.go` with `//go:build !qa` containing `SMTPSender` and its factory function. The existing `sender.go` is split: shared types (`ErrExampleDomain`) move to a build-tag-free file, and each sender gets its own build-tagged file.
**Alternative interpretation:** A single `sender.go` could remain build-tag-free with both implementations, using a factory function in build-tagged files to select which one to return. Or the `ConsoleSender` could live in a separate sub-package.
**Impact if wrong:** If the file split is wrong, both senders could be compiled into the same binary, or the factory function might not resolve correctly. The `//go:build` constraint must ensure exactly one `NewEmailSender` factory is available per build.

## ConsoleSender SMTP Configuration in QA Builds -- OPEN

**Source says:** Future work item #7 says "QA builds should be fully standalone -- no Mailpit, no SMTP server." The current `daemon.go` unconditionally calls `email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)` which requires SMTP config values.
**Ambiguity:** If QA builds use `ConsoleSender`, does the daemon still require SMTP configuration values (host, port, from address) at startup? The `ConsoleSender` does not need them, but the daemon's wiring code currently reads these from config unconditionally.
**Decision made:** The spec (service-build REQ-032) says QA builds require no external dependencies. The daemon wiring code must use a build-tag-gated factory function (e.g., `email.NewSender(...)`) that accepts SMTP config in non-QA builds and ignores it in QA builds. QA builds SHALL NOT fail if SMTP configuration is absent.
**Alternative interpretation:** The daemon could still require SMTP config values in QA builds but simply not use them. This keeps the config schema identical across all builds but violates the "no external dependencies" requirement if the SMTP host is validated or connected at startup.
**Impact if wrong:** If SMTP config is required but absent, the QA binary fails at startup with a config validation error, defeating the standalone goal. If SMTP config is optional globally, production builds could accidentally start without SMTP config and silently fail to send emails.

## @fail.com Timeout Error Type -- OPEN

**Source says:** Future work item #8 says `@fail.com` produces a "simulated timeout." The notification-state-machine spec classifies errors into permanent (skip retry, go to `failed`) and transient (go to `not_sent`, eligible for retry).
**Ambiguity:** What Go error type should the `@fail.com` timeout return? The worker uses `errors.Is(err, email.ErrExampleDomain)` to detect permanent failures. A timeout needs to be recognized as a transient error so the notification goes to `not_sent` rather than `failed`. Should it wrap `context.DeadlineExceeded`, define a new sentinel error like `ErrSimulatedTimeout`, or return a generic `errors.New("simulated timeout")`?
**Decision made:** The spec (service-build REQ-029) says `@fail.com` returns "a timeout error (wrapping a `context.DeadlineExceeded`-style error)." This is intentionally imprecise -- the key requirement is that the error is NOT classified as permanent (not wrapping `ErrExampleDomain`), so the worker transitions to `not_sent` and retries.
**Alternative interpretation:** A new sentinel `ErrSimulatedTimeout` would be more explicit and testable. Wrapping `context.DeadlineExceeded` could confuse error handling that checks for real context cancellation elsewhere in the call stack.
**Impact if wrong:** If the error wraps `ErrExampleDomain`, the notification goes directly to `failed` instead of `not_sent`, defeating the purpose of simulating a retryable timeout.

## @slow.com Delay Interaction with Existing 6-Second Delay -- OPEN

**Source says:** Future work item #8 says `@slow.com` applies "extra-long delay (e.g., 30s)." REQ-016 requires all email sends to include a 6-second artificial delay.
**Ambiguity:** Does the 30-second `@slow.com` delay replace the standard 6-second delay or stack on top of it? If stacked, total delay is 36 seconds. If replaced, total delay is 30 seconds.
**Decision made:** The spec (service-build REQ-029) says `@slow.com` applies "a 30-second delay before returning success." This is the total delay for `@slow.com` recipients -- it replaces the standard 6-second delay rather than adding to it. The 30-second delay is already long enough to demonstrate slow delivery behavior.
**Alternative interpretation:** The 6-second delay (REQ-016) could apply universally as a first step, then `@slow.com` adds its own 30 seconds on top, for 36 seconds total. This preserves the invariant that every send has at least the base delay.
**Impact if wrong:** If stacked, the delay is 36 seconds which may exceed the goqite visibility timeout (30 seconds by default in seed config), causing the job to be re-delivered while still processing. If replaced, the base delay contract (REQ-016) is technically violated for `@slow.com` addresses, though the spirit (making state transitions visible) is preserved.

## Simulated Failure Domain Map Extensibility -- OPEN

**Source says:** Future work item #8 lists three specific domains: `@example.com`, `@fail.com`, `@slow.com`. It says "extend the QA sender with a map."
**Ambiguity:** Is the domain map hardcoded to exactly these three domains, or should it be configurable (e.g., via a config file or additional map entries)? The word "map" suggests a data structure, but the future work item only lists three fixed entries.
**Decision made:** The spec (service-build REQ-029) defines exactly three hardcoded domains. The map is a compile-time constant, not runtime-configurable. This matches the QA build tag philosophy: QA behaviors are baked into the binary, not configured at runtime.
**Alternative interpretation:** The map could be loaded from a config file or extended via additional build-tagged files, allowing QA testers to add custom simulated domains without recompiling.
**Impact if wrong:** If hardcoded and a QA tester needs a new simulated domain, they must modify source code and rebuild. If configurable, there is additional config surface area that could be misconfigured.

## MCP Error Handling -- Which Errors Are "Safe" for Clients -- OPEN

**Source says:** Future work item #10 says "MCP tools should follow the same pattern as HTTP handlers -- return a generic error message to the client, log the real error." The current code exposes `err.Error()` for CreateNotification, Enqueue, GetNotificationByEmail, state machine FireCtx, and UpdateNotification failures. However, it also returns domain validation errors (from `domain.ValidateEmail`) and domain sentinel texts (`"already notified"`, `"not found"`) directly to clients.
**Ambiguity:** Should state machine errors from `sm.FireCtx` (e.g., "cannot transition from pending via reset") be treated as safe client-facing errors or internal errors? These errors reveal state machine internals but also tell the client why the operation failed. Similarly, the `"reset failed: "` prefix on state machine errors implies the operation type -- should the generic error for reset be `"internal error"` or `"reset failed"`?
**Decision made:** The spec (mcp-integration REQ-014) treats all non-domain errors as internal. State machine `FireCtx` errors are classified as internal errors because they may contain implementation details about the state machine library. The client receives `"internal error"` and the real error is logged server-side. Domain sentinels (`ErrAlreadyNotified`, `ErrNotFound`) and validation errors remain client-facing (REQ-016).
**Alternative interpretation:** State machine errors could be sanitized but still specific (e.g., `"reset not permitted from current state"`) to give the client actionable information without leaking internals.
**Impact if wrong:** If state machine errors are hidden, MCP clients cannot distinguish "the notification is in a state that does not allow reset" from "the database is down." If exposed, state machine library internals (transition names, guard conditions) may leak.

## WebSocket Connection Limit -- Rejection Behavior -- OPEN

**Source says:** Future work item #14 says "track connection count in the hub's `Run` loop, reject registrations above a configurable limit (e.g., 1000)." The current hub registration is a channel send (`h.register <- c`) processed inside the `Run` loop.
**Ambiguity:** What happens to the rejected client's WebSocket connection? The hub can close the send channel, but the underlying `*websocket.Conn` is owned by the HTTP handler goroutine (via `WritePump` and `ReadPump`). Closing the send channel causes `WritePump` to exit and close the connection, but `ReadPump` is blocking in the handler goroutine. Should the hub also close the WebSocket connection directly, or rely on the send channel closure to cascade through the pumps?
**Decision made:** The spec (notification-realtime REQ-018) says the hub closes the rejected client's send channel. This causes `WritePump` to exit (it returns when the send channel is closed), which calls `conn.Close(websocket.StatusNormalClosure, ...)`. The `ReadPump` will then get a read error on the closed connection and exit. This matches the existing slow-client drop pattern (REQ-012) and avoids the hub directly touching the connection.
**Alternative interpretation:** The hub could close the WebSocket connection with a specific close code (e.g., `websocket.StatusTryAgainLater` or `websocket.StatusPolicyViolation`) to give the client a clear signal that the server is at capacity. This would require the hub to access `client.Conn` directly.
**Impact if wrong:** If the hub only closes the send channel but `WritePump` has not started yet (race between registration and goroutine scheduling), the connection could leak. If the hub closes the connection directly, it introduces a second closer for the same resource, risking a double-close panic.

## WebSocket Connection Limit -- Configuration Mechanism -- OPEN

**Source says:** Future work item #14 says "configurable limit (e.g., 1000)" but does not specify how the limit is configured.
**Ambiguity:** How is the limit configured? Via koanf config file (`ws.max_connections`), an environment variable (`NOTIFIER_WS_MAX_CONNECTIONS`), a CLI flag, or a compile-time constant? The existing service uses koanf for config with env var overrides.
**Decision made:** The spec (notification-realtime REQ-019) says the limit is passed to `NewHub(maxConns int)` at construction time, leaving the config source to the daemon wiring layer. The default is 1000.
**Alternative interpretation:** The limit could be a top-level config field or a nested `ws.max_connections` field. It could also be a hard-coded constant if the value rarely changes.
**Impact if wrong:** If the config path is wrong, the limit cannot be overridden at deploy time. If hard-coded, operators cannot adjust it without rebuilding.

## MaxBytesReader Error Response Format -- OPEN

**Source says:** Future work item #13 says "add `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` (1 MB) at the top of each POST handler." The Go standard library's `MaxBytesReader` causes `json.Decoder.Decode` to return a `*http.MaxBytesError` when the limit is exceeded.
**Ambiguity:** What HTTP status code and response body should be returned when the body exceeds 1 MB? The `MaxBytesReader` itself does not write a response -- it causes the subsequent `json.NewDecoder(r.Body).Decode()` to fail. The existing error handling for decode failures returns `400 Bad Request` with `{"error": "invalid JSON body"}`. Should the oversized body produce the same 400 with the same message, a 400 with a different message (e.g., `"request body too large"`), or HTTP 413 Request Entity Too Large?
**Decision made:** The spec says HTTP 400 Bad Request. Since `MaxBytesReader` is applied before `json.Decode`, the decode call will fail with a `MaxBytesError`, which falls through to the existing `"invalid JSON body"` error path. This is acceptable for the initial fix -- the body is invalid from the decoder's perspective. A follow-up could check for `*http.MaxBytesError` specifically and return 413.
**Alternative interpretation:** HTTP 413 Request Entity Too Large is the semantically correct status code for this case. The handler could check `errors.As(err, &maxBytesErr)` before falling through to the generic decode error.
**Impact if wrong:** Clients receiving 400 instead of 413 cannot distinguish "malformed JSON" from "body too large." This matters for clients that want to retry with a smaller payload. However, for this service (where the only POST body is a small JSON with an email field), the distinction is unlikely to matter in practice.

## WebSocket Origin Validation -- Default Origin Pattern -- OPEN

**Source says:** Future work item #11 says "pass `&websocket.AcceptOptions{OriginPatterns: [...]}` with allowed origins" but does not specify what the default allowed origins should be or what happens when no origins are configured.
**Ambiguity:** What is the safe default when the operator does not configure `ws.allowed-origins`? Options: (a) reject all connections (secure but breaks out-of-the-box development), (b) allow `localhost:*` (permits local dev, rejects cross-origin in production), (c) allow all origins (insecure, equivalent to current behavior).
**Decision made:** Default to `["localhost:*"]`. This permits the development workflow (dashboard at `localhost:8080` connecting to the WebSocket) without explicit configuration, while rejecting connections from non-localhost origins in production deployments that neglect to configure origins.
**Alternative interpretation:** The default could be an empty list that rejects all WebSocket connections until the operator explicitly configures origins. This is more secure but breaks the zero-configuration development experience that the service currently provides.
**Impact if wrong:** If the default is too permissive (e.g., allow all), the fix provides no protection unless the operator actively configures it, which defeats the purpose. If the default is too restrictive (reject all), developers must add configuration before WebSocket works at all, which is a regression in developer experience.

## WebSocket Origin Validation -- Configuration Key Format for List Values -- OPEN

**Source says:** The existing koanf configuration uses `env.Provider("NOTIFIER_", ".", ...)` which converts `NOTIFIER_WS_ALLOWED_ORIGINS` to the koanf key `ws.allowed.origins` (underscores become dots). The YAML config would use a list: `ws:\n  allowed-origins:\n    - "localhost:*"`.
**Ambiguity:** How does a list value work through the environment variable override? Koanf's env provider treats env var values as strings, not lists. `NOTIFIER_WS_ALLOWED_ORIGINS=localhost:*,example.com` would need custom parsing (e.g., comma-separated splitting). Additionally, the underscore-to-dot conversion produces `ws.allowed.origins` (three levels deep) but the YAML key `allowed-origins` is a single hyphenated key at the second level, producing `ws.allowed-origins`. These do not match.
**Decision made:** The spec uses `ws.allowed-origins` as the configuration key. The implementer must resolve the koanf key mapping so that both the YAML key and the environment variable resolve to the same configuration path. For the environment variable, comma-separated values split into a list is the conventional approach.
**Alternative interpretation:** The config key could be `ws.origins` (simpler, avoids the hyphen-vs-dot mapping issue). Or the environment variable could use a different separator (semicolon, space) or a JSON-encoded list.
**Impact if wrong:** If the YAML key and env var resolve to different koanf paths, the env var override silently fails -- the operator thinks they configured origins via environment but the code reads the YAML default. This would leave the WebSocket endpoint unprotected in container deployments that rely on env vars.

## WebSocket Origin Validation -- 127.0.0.1 vs localhost in Test Environments -- OPEN

**Source says:** The existing handler tests use `httptest.NewServer` which binds to `127.0.0.1` with a random port. The `coder/websocket` library checks the `Origin` header against `OriginPatterns`.
**Ambiguity:** Does `localhost:*` match connections where the `Origin` header is `http://127.0.0.1:<port>`? The `coder/websocket` library may treat `localhost` and `127.0.0.1` as distinct hostnames. If so, tests using `httptest.NewServer` would fail with `["localhost:*"]` as the allowed origins.
**Decision made:** The spec does not mandate whether `localhost` and `127.0.0.1` are treated as equivalent -- it defers to the `coder/websocket` library's pattern matching semantics (REQ-023). The implementer must verify behavior against the library and adjust test origin patterns accordingly (e.g., `["localhost:*", "127.0.0.1:*"]` if they are distinct).
**Alternative interpretation:** The default could include both `localhost:*` and `127.0.0.1:*` to cover both cases. This is slightly more permissive but avoids a subtle test-vs-production discrepancy.
**Impact if wrong:** If `localhost:*` does not match `127.0.0.1`, all existing handler tests break after the change. If the default includes `127.0.0.1:*`, it is slightly more permissive than intended but unlikely to be exploitable since `127.0.0.1` is a loopback address.

## Semantic Token CSS Architecture -- RESOLVED

**Source says:** Future work #4 states "The status badge colors and button colors should come from the same semantic token set so they're always in sync."
**Ambiguity:** The mechanism for sharing tokens is unspecified. Options include: (a) CSS custom properties in `@theme` with `.dark` overrides, (b) a separate `tokens.css` file imported by both components, (c) Tailwind plugin that reads brand.json directly.
**Decision made:** CSS custom properties declared in the `@theme` block of `index.css` with dark mode overrides in a `.dark` selector. This is the simplest approach that works with Tailwind v4's CSS-first configuration and requires no additional build tooling.
**Alternative interpretation:** A Tailwind plugin or separate tokens file could provide more structured token management.
**Impact if wrong:** If a plugin approach is preferred, the `index.css` changes would need to be moved to a plugin file. The token names and values would remain the same.

## Button Variant Hover States -- RESOLVED

**Source says:** Future work #4 mentions button color variants but does not specify hover behavior.
**Ambiguity:** How should semantic-colored button hover states work? Options: (a) darken background via Tailwind opacity modifier (e.g., `hover:bg-semantic-success-bg/80`), (b) a dedicated hover token in brand.json, (c) use `filter: brightness()`.
**Decision made:** Use a slightly more opaque/saturated background via a dedicated `hover` sub-key would add complexity. Instead, semantic button variants use the semantic text color as the background and white text, with a hover state that applies opacity. This provides strong contrast and clear affordance as interactive elements, while badges use the light background/dark text pattern for non-interactive display.
**Alternative interpretation:** Each semantic color could have a dedicated hover background color defined in brand.json.
**Impact if wrong:** If dedicated hover colors are preferred, brand.json would need additional keys and the CSS custom properties would need corresponding hover variants. Visual difference would be subtle.

## Neutral vs Yellow Naming -- RESOLVED

**Source says:** Future work #3 specifies `pending=yellow`. The semantic token system uses semantic names.
**Ambiguity:** Should the yellow semantic token be called `neutral` (semantic meaning) or `yellow` (visual description)? Components map `pending` -> `neutral`, which is one more level of indirection.
**Decision made:** Use `neutral` as the semantic name. The token name describes intent, not color. This allows the actual color to change without renaming the token.
**Alternative interpretation:** Use `caution` or `pending` as the semantic name to be more descriptive of the use case.
**Impact if wrong:** Only naming. Functionally identical. A rename would require updating CSS properties and component class references.

## Font Stack Runtime Behavior -- RESOLVED

**Source says:** Future work #1 mentions "Inter" and "JetBrains Mono" specifically but notes "web fonts are unreliable in email clients."
**Ambiguity:** Should the dashboard load Inter and JetBrains Mono via `@font-face` or Google Fonts, or rely purely on the fallback stack?
**Decision made:** The font stacks include these as first-choice fonts but no `@font-face` or CDN import is added. Users with these fonts installed locally will see them; others get `ui-sans-serif` / `ui-monospace` fallbacks. Email always uses the fallback stack.
**Alternative interpretation:** A `@font-face` declaration or Google Fonts `<link>` could ensure consistent rendering.
**Impact if wrong:** If web font loading is expected, a font loading strategy would need to be added to `index.html`. This is additive and does not affect the token architecture.
