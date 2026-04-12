---
type: plan
step: "10"
title: "QA Build Isolation"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "10"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-3-notification-delivery
  - step-9-release-qa
---

# Step 10: QA Build Isolation

## Overview

Split the email sender into build-tag-gated file pairs so QA builds
use a standalone `ConsoleSender` (logs to slog, no network) while dev
and production builds use the real `SMTPSender`. This removes the need
for Mailpit or any SMTP server in QA, makes the QA binary fully
self-contained, and moves the `@example.com` rejection logic out of
the production code path entirely.

The `ConsoleSender` includes a hardcoded simulated failure domain map
(`@example.com` = permanent failure, `@fail.com` = timeout,
`@slow.com` = 30-second delay) so QA can exercise every state machine
path without external dependencies.

This step also ensures the daemon handles missing SMTP config
gracefully in QA builds. The `build:email` dependency in `release:qa`
is retained because `template.go` embeds compiled templates via
`//go:embed` in all builds (the worker renders templates before
passing content to the sender).

Addresses future work items #6, #7, and #8. Satisfies service-build
REQ-026 through REQ-032 and notification-delivery REQ-017, REQ-029.

## Prerequisites

- Step 3 completed: `sender.go`, `sender_test.go`, worker, and
  `domain.EmailSender` interface exist.
- Step 4 completed: state machine and retry logic are in place.
- Step 9 completed: `release:qa` mise task exists.

## Tasks

### Task 1: Create sender_default.go (production/dev SMTP sender)

**Files:**
- Create: `internal/infra/email/sender_default.go`
- Modify: `internal/infra/email/sender.go`

**Step 1: Create `internal/infra/email/sender_default.go`**

Move the `SMTPSender` and related code from `sender.go` into a new
file gated with `//go:build !qa`. The `@example.com` rejection logic
is removed -- that behavior now lives exclusively in the QA sender's
domain map (service-build REQ-030, notification-delivery REQ-017).

```go
//go:build !qa

package email

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/workfort/notifier/internal/domain"
)

// SMTPSender implements domain.EmailSender via go-mail SMTP.
type SMTPSender struct {
	client *gomail.Client
	from   string
}

// NewSMTPSender creates a new SMTP sender. For Mailpit (local dev),
// use port 1025 with no auth and TLSOpportunistic.
func NewSMTPSender(host string, port int, from string) (*SMTPSender, error) {
	c, err := gomail.NewClient(host,
		gomail.WithPort(port),
		gomail.WithTLSPolicy(gomail.TLSOpportunistic),
	)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return &SMTPSender{client: c, from: from}, nil
}

// Send delivers an email message via SMTP. It enforces the 6-second
// delay (notification-delivery REQ-016) and propagates the
// X-Request-ID header (REQ-023).
//
// The @example.com rejection logic is NOT present in production/dev
// builds (notification-delivery REQ-017). That behavior is compiled
// only into QA builds via the ConsoleSender's domain map.
func (s *SMTPSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
	// REQ-016: artificial delay to simulate real delivery latency.
	slog.Info("email send delay starting", "delay", sendDelay)
	select {
	case <-time.After(sendDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	m := gomail.NewMsg()
	if err := m.From(s.from); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := m.To(msg.To...); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(gomail.TypeTextHTML, msg.HTML)
	m.AddAlternativeString(gomail.TypeTextPlain, msg.Text)

	// REQ-023: propagate request ID into email header.
	if reqID := RequestIDFromMessage(msg); reqID != "" {
		m.SetGenHeader(gomail.Header("X-Request-ID"), reqID)
	}

	if err := s.client.DialAndSend(m); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
```

Satisfies: notification-delivery REQ-014, REQ-015, REQ-016, REQ-017,
REQ-023. Satisfies: service-build REQ-030 (no simulated failure logic
in non-QA builds).

**Step 2: Reduce `sender.go` to shared utilities only**

Replace the entire contents of `internal/infra/email/sender.go` with
the shared `ErrExampleDomain` sentinel and `RequestIDFromMessage`
helper. These have no build tag constraint because both senders and
the worker reference them.

```go
package email

import (
	"errors"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// sendDelay is the artificial delay before sending an email, making
// state transitions visible in the dashboard. Extracted as a package
// variable so tests can override it. Shared across both the SMTP
// sender (production/dev) and the ConsoleSender (QA).
var sendDelay = 6 * time.Second

// ErrExampleDomain is returned when the recipient address ends in
// @example.com, simulating an undeliverable address. Only the QA
// ConsoleSender returns this error; it is never returned in
// production/dev builds. It lives in the shared file because the
// worker (which is not build-tag-gated) checks errors.Is to
// distinguish permanent from transient failures in all builds.
var ErrExampleDomain = errors.New("example.com: permanent delivery failure (simulated)")

// RequestIDFromMessage extracts the request ID from the email message.
func RequestIDFromMessage(msg *domain.EmailMessage) string {
	return msg.RequestID
}
```

**Step 3: Verify the project compiles without `qa` tag**

Run: `go build ./...`

Expected: BUILD SUCCESS. The `!qa` file is selected, `SMTPSender` and
`NewSMTPSender` are defined, and the daemon compiles.

**Step 4: Commit**

`refactor(email): split sender into build-tagged file pair`

### Task 2: Create sender_qa.go (QA ConsoleSender)

**Depends on:** Task 1

**Files:**
- Create: `internal/infra/email/sender_qa.go`

**Step 1: Create `internal/infra/email/sender_qa.go`**

```go
//go:build qa

package email

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// domainAction describes a simulated behavior for a recipient domain.
type domainAction struct {
	delay time.Duration
	err   error
	label string
}

// simulatedDomains maps recipient domains to simulated behaviors
// (service-build REQ-029). This map is compiled only into QA builds
// (REQ-030).
var simulatedDomains = map[string]domainAction{
	"@example.com": {
		err:   ErrExampleDomain,
		label: "simulated permanent failure for @example.com",
	},
	"@fail.com": {
		err:   fmt.Errorf("simulated timeout: %w", context.DeadlineExceeded),
		label: "simulated timeout for @fail.com",
	},
	"@slow.com": {
		delay: 30 * time.Second,
		label: "simulated slow delivery for @slow.com",
	},
}

// matchDomain returns the domainAction for the first recipient whose
// address matches a simulated domain, or nil if no match.
func matchDomain(recipients []string) *domainAction {
	for _, addr := range recipients {
		lower := strings.ToLower(addr)
		for suffix, action := range simulatedDomains {
			if strings.HasSuffix(lower, suffix) {
				a := action // copy to avoid loop variable capture
				return &a
			}
		}
	}
	return nil
}

// ConsoleSender implements domain.EmailSender by logging to slog
// instead of sending over SMTP. It requires no external dependencies
// (service-build REQ-032).
type ConsoleSender struct{}

// NewSMTPSender returns a ConsoleSender in QA builds. The function
// name matches the production signature so the daemon compiles
// without build-tag-conditional wiring. The SMTP parameters are
// accepted but ignored (service-build REQ-032).
func NewSMTPSender(_ string, _ int, _ string) (*ConsoleSender, error) {
	slog.Info("QA build: using ConsoleSender (no SMTP)")
	return &ConsoleSender{}, nil
}

// Send logs the email to slog and applies simulated domain behaviors.
//
// Flow:
//  1. Check recipients against the simulated domain map (REQ-029).
//     If matched, log and apply the simulated action (REQ-031).
//  2. Apply the standard 6-second delay (REQ-028).
//  3. Log the email content (REQ-027) and return nil.
func (s *ConsoleSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
	// REQ-029: check simulated failure domain map.
	if action := matchDomain(msg.To); action != nil {
		// REQ-031: log the simulated action.
		slog.Info(action.label, "to", msg.To)

		// Domain-specific delay (e.g., @slow.com 30s) replaces the
		// standard 6s delay -- the total delay is the domain delay,
		// not domain delay + 6s.
		if action.delay > 0 {
			slog.Info("email send delay starting", "delay", action.delay)
			select {
			case <-time.After(action.delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if action.err != nil {
			return fmt.Errorf("send to %s: %w", msg.To[0], action.err)
		}

		// Domain matched with delay but no error (e.g., @slow.com):
		// log and return success after the delay.
		slog.Info("email sent (console)",
			"to", msg.To,
			"subject", msg.Subject,
			"body", msg.Text,
		)
		return nil
	}

	// REQ-028: standard 6-second artificial delay.
	slog.Info("email send delay starting", "delay", sendDelay)
	select {
	case <-time.After(sendDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	// REQ-027: log the email content.
	slog.Info("email sent (console)",
		"to", msg.To,
		"subject", msg.Subject,
		"body", msg.Text,
	)
	return nil
}
```

Design decisions:

- **`NewSMTPSender` name reused** -- the daemon calls
  `email.NewSMTPSender(...)` unconditionally. By keeping the same
  function name in both files, the daemon compiles without any
  build-tag-conditional wiring. In QA, the function ignores its
  parameters and returns a `ConsoleSender`.
- **`@slow.com` delay replaces, not adds** -- per resolved
  ambiguity #4, the 30-second domain delay replaces the standard
  6-second delay. The total wait for `@slow.com` is 30 seconds.
- **`@fail.com` wraps `context.DeadlineExceeded`** -- per
  service-build REQ-029, the error wraps `context.DeadlineExceeded`
  to simulate a timeout. The worker's `errors.Is(err,
  ErrExampleDomain)` check does not match, so the error is treated
  as transient and triggers retry via `TriggerSoftFail`.
- **Domain map is hardcoded** -- per resolved ambiguity #5, three
  domains, no config surface.

Satisfies: service-build REQ-026, REQ-027, REQ-028, REQ-029, REQ-030,
REQ-031, REQ-032. Satisfies: notification-delivery REQ-029.

**Step 2: Verify the project compiles with `qa` tag**

Run: `go build -tags qa ./...`

Expected: BUILD SUCCESS. The `qa` file is selected, `ConsoleSender`
and `NewSMTPSender` (returning `*ConsoleSender`) are defined.

**Step 3: Commit**

`feat(email): add QA ConsoleSender with simulated failure domains`

### Task 3: Update sender_test.go for build-tag split

**Depends on:** Task 1, Task 2

**Files:**
- Modify: `internal/infra/email/sender_test.go`
- Create: `internal/infra/email/sender_qa_test.go`

**Step 1: Write the QA sender tests**

Create `internal/infra/email/sender_qa_test.go` with `//go:build qa`:

```go
//go:build qa

package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

func init() {
	// Override the 6-second delay for fast tests.
	sendDelay = 1 * time.Millisecond
}

func TestConsoleSenderImplementsInterface(t *testing.T) {
	// Compile-time check that ConsoleSender implements domain.EmailSender.
	var _ domain.EmailSender = (*ConsoleSender)(nil)
}

func TestConsoleSenderSuccess(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"user@company.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestConsoleSenderExampleComPermanentFailure(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(context.Background(), msg)
	if sendErr == nil {
		t.Fatal("expected error for @example.com, got nil")
	}
	if !errors.Is(sendErr, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain, got: %v", sendErr)
	}
}

func TestConsoleSenderFailComTimeout(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@fail.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(context.Background(), msg)
	if sendErr == nil {
		t.Fatal("expected error for @fail.com, got nil")
	}
	// Must NOT be ErrExampleDomain so the worker treats it as transient.
	if errors.Is(sendErr, ErrExampleDomain) {
		t.Error("@fail.com error should not be ErrExampleDomain")
	}
	// Must wrap context.DeadlineExceeded per service-build REQ-029.
	if !errors.Is(sendErr, context.DeadlineExceeded) {
		t.Error("@fail.com error should wrap context.DeadlineExceeded")
	}
}

func TestConsoleSenderSlowComDelay(t *testing.T) {
	// Override the @slow.com delay for testing.
	orig := simulatedDomains["@slow.com"]
	simulatedDomains["@slow.com"] = domainAction{
		delay: 1 * time.Millisecond,
		label: orig.label,
	}
	defer func() { simulatedDomains["@slow.com"] = orig }()

	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	msg := &domain.EmailMessage{
		To:      []string{"test@slow.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	if sendErr := sender.Send(context.Background(), msg); sendErr != nil {
		t.Fatalf("Send() error = %v, want nil (slow but success)", sendErr)
	}
}

func TestConsoleSenderSlowComCancellation(t *testing.T) {
	sender, err := NewSMTPSender("", 0, "")
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	msg := &domain.EmailMessage{
		To:      []string{"test@slow.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	sendErr := sender.Send(ctx, msg)
	if sendErr == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestRequestIDExtraction(t *testing.T) {
	msg := &domain.EmailMessage{
		To:        []string{"user@company.com"},
		Subject:   "Test",
		HTML:      "<p>test</p>",
		Text:      "test",
		RequestID: "req_abc-123",
	}
	got := RequestIDFromMessage(msg)
	if got != "req_abc-123" {
		t.Errorf("RequestIDFromMessage() = %q, want %q", got, "req_abc-123")
	}
}
```

**Step 2: Update `sender_test.go` with `!qa` build tag**

Replace the contents of `internal/infra/email/sender_test.go` with a
`//go:build !qa` constraint. The existing tests for `SMTPSender` and
`@example.com` stay here since they exercise production-only code:

```go
//go:build !qa

package email

import (
	"testing"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

func init() {
	// Override the 6-second delay for fast tests.
	sendDelay = 1 * time.Millisecond
}

func TestSMTPSenderImplementsInterface(t *testing.T) {
	// Compile-time check that SMTPSender implements domain.EmailSender.
	var _ domain.EmailSender = (*SMTPSender)(nil)
}

func TestRequestIDExtraction(t *testing.T) {
	msg := &domain.EmailMessage{
		To:        []string{"user@company.com"},
		Subject:   "Test",
		HTML:      "<p>test</p>",
		Text:      "test",
		RequestID: "req_abc-123",
	}
	got := RequestIDFromMessage(msg)
	if got != "req_abc-123" {
		t.Errorf("RequestIDFromMessage() = %q, want %q", got, "req_abc-123")
	}
}
```

The two `@example.com` tests (`TestExampleComAutoFail`,
`TestExampleComCheckMultipleRecipients`) are removed. The `@example.com`
rejection is no longer in the production sender -- it lives
exclusively in the QA `ConsoleSender`. The QA test file covers it via
`TestConsoleSenderExampleComPermanentFailure`.

**Step 3: Run tests without `qa` tag**

Run: `go test ./internal/infra/email/...`

Expected: PASS. The `!qa` test file runs, `TestSMTPSenderImplementsInterface`
and `TestRequestIDExtraction` pass.

**Step 4: Run tests with `qa` tag**

Run: `go test -tags qa ./internal/infra/email/...`

Expected: PASS. The `qa` test file runs, all `ConsoleSender` tests pass.

**Step 5: Commit**

`test(email): add ConsoleSender tests, gate sender tests by build tag`

### Task 4: Update daemon to handle missing SMTP config in QA builds

**Depends on:** Task 2

**Files:**
- Modify: `cmd/daemon/daemon.go:143-147`

**Step 1: Verify current daemon behavior**

The daemon currently calls `email.NewSMTPSender(cfg.SMTPHost,
cfg.SMTPPort, cfg.SMTPFrom)` unconditionally at line 143. In QA
builds, `NewSMTPSender` returns a `*ConsoleSender` and ignores the
parameters, so no change is needed to the call site itself.

Verify that the QA build of the daemon compiles and starts without
SMTP configuration by reviewing the `NewSMTPSender` signature in
`sender_qa.go` -- it accepts the same parameters but ignores them and
always returns `nil` error. The daemon's existing error check on
line 144-146 handles this correctly.

No code change is required. The build-tag split in Tasks 1 and 2
already satisfies this requirement because `NewSMTPSender` in QA
builds ignores SMTP parameters and never errors on missing config.

Satisfies: service-build REQ-032 (QA builds require no external
dependencies for email delivery).

**Step 2: Verify daemon compiles with QA tag**

Run: `go build -tags qa ./cmd/...`

Expected: BUILD SUCCESS. The daemon compiles with the QA sender.

**Step 3: Commit**

No commit needed -- no code changed. Verification only.

### Task 5: Verify build:email dependency in release:qa

**Files:**
- Modify: `openspec/ambiguities.md`

**Step 1: Verify the release:qa task retains build:email**

The `ConsoleSender` logs raw email content to slog and never calls
`RenderNotification`. However, the worker (which is not
build-tag-gated) calls `RenderNotification` unconditionally before
passing the rendered content to `sender.Send`. The email templates
(`internal/infra/email/dist/`) are embedded via `//go:embed` in
`template.go`, which is compiled into all builds.

This means `build:email` IS still needed -- without the compiled
templates in `internal/infra/email/dist/`, the `//go:embed` directive
fails at compile time. The dependency must stay.

**No change to `.mise/tasks/release/qa`.** The `//go:embed` in
`template.go` requires the compiled templates to exist at build time
regardless of whether the QA sender uses them. The dependency is a
compile-time requirement, not a runtime one.

**Step 2: Correct the ambiguity resolution for build:email**

Update the "release:qa Dependency on build:email" entry in
`openspec/ambiguities.md` to reflect the correct analysis: `release:qa`
DOES need `build:email` because `template.go` has `//go:embed
dist/*.html` and `//go:embed dist/*.txt` directives that fail at
compile time if the `dist/` directory is empty. The worker calls
`RenderNotification` unconditionally in all builds, so the compiled
templates must be present. This is a compile-time embedding
constraint, not a sender behavior concern.

**Step 3: Commit**

`docs: correct ambiguity resolution for release:qa build:email dependency`

### Task 6: Update future-work.md to mark items resolved

**Files:**
- Modify: `docs/future-work.md`

**Step 1: Mark future work items #6, #7, and #8 as resolved**

Add a checkmark to the headings for items #6, #7, and #8, matching
the pattern used for item #9:

Change `## #6 — @example.com Auto-Fail Should Be Dev/QA Only` to
`## #6 — @example.com Auto-Fail Should Be Dev/QA Only ✅`

Change `## #7 — QA Build: Console-Only Email, No Mailpit Required` to
`## #7 — QA Build: Console-Only Email, No Mailpit Required ✅`

Change `## #8 — QA Build: Simulated Failure Domains` to
`## #8 — QA Build: Simulated Failure Domains ✅`

**Step 2: Commit**

`docs: mark future work items #6, #7, #8 as resolved`

### Task 7: Full verification

**Depends on:** Task 1, Task 2, Task 3, Task 4, Task 5, Task 6

**Step 1: Run the full test suite without QA tag**

Run: `mise run test:unit`

Expected: PASS. All tests pass. The `!qa` sender tests exercise the
`SMTPSender`. No `@example.com` rejection in production code.

**Step 2: Run the full test suite with QA tag**

Run: `go test -tags qa ./...`

Expected: PASS. All tests pass. The `qa` sender tests exercise the
`ConsoleSender` and simulated domain map.

**Step 3: Build the QA binary**

Run: `mise run release:qa`

Expected: exits 0, produces `build/notifier`.

**Step 4: Build the production binary**

Run: `mise run release:production`

Expected: exits 0. Production binary does not contain simulated
failure logic.

**Step 5: Run the linter**

Run: `mise run lint:go`

Expected: no warnings or errors.

## Verification Checklist

- [ ] `internal/infra/email/sender.go` contains only `ErrExampleDomain`
      and `RequestIDFromMessage` (shared, no build tag)
- [ ] `internal/infra/email/sender_default.go` has `//go:build !qa`
      and contains `SMTPSender` with no `@example.com` rejection
- [ ] `internal/infra/email/sender_qa.go` has `//go:build qa` and
      contains `ConsoleSender` with simulated domain map
- [ ] `ConsoleSender` logs recipient, subject, and body via slog (REQ-027)
- [ ] `ConsoleSender` applies 6-second delay for normal domains (REQ-028)
- [ ] `@example.com` returns error wrapping `ErrExampleDomain` (REQ-029)
- [ ] `@fail.com` returns error wrapping `context.DeadlineExceeded` --
      worker treats as transient (REQ-029)
- [ ] `@slow.com` applies 30-second delay then returns success (REQ-029)
- [ ] `@slow.com` delay replaces (not adds to) the 6-second delay
- [ ] Domain map exists only in `//go:build qa` file (REQ-030)
- [ ] Simulated actions are logged before returning (REQ-031)
- [ ] QA binary starts without SMTP config or Mailpit (REQ-032)
- [ ] `NewSMTPSender` in QA returns `*ConsoleSender`, same function
      name as production -- daemon compiles without conditional wiring
- [ ] `sendDelay` declared once in shared `sender.go`, not in
      build-tagged files
- [ ] `ErrExampleDomain` comment notes it is only returned in QA builds
- [ ] `sender_test.go` has `//go:build !qa` tag
- [ ] `sender_qa_test.go` has `//go:build qa` tag
- [ ] `TestRequestIDExtraction` present in both test files
- [ ] `go test ./internal/infra/email/...` passes (production tests)
- [ ] `go test -tags qa ./internal/infra/email/...` passes (QA tests)
- [ ] `mise run test:unit` passes
- [ ] `mise run release:qa` builds successfully
- [ ] `mise run release:production` builds successfully
- [ ] `mise run lint:go` passes with no warnings
- [ ] `ambiguities.md` build:email resolution corrected
- [ ] Future work items #6, #7, #8 marked resolved
