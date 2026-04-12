---
type: plan
step: "3"
title: "Notification Delivery"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "3"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-2-cli-and-database
---

# Step 3: Notification Delivery

## Overview

Deliver the core write path of the notification service: accept a
notification request via `POST /v1/notify`, validate the email address,
enforce one-notification-per-email uniqueness, create a notification
record in the database, enqueue an email delivery job via goqite, and
send branded HTML email through SMTP with a 6-second artificial delay.

After this step the service accepts notify requests, queues and delivers
email through a background worker, automatically fails `@example.com`
addresses, includes branded HTML and plaintext email bodies compiled
from Maizzle templates, propagates request IDs into email headers, and
has E2E tests that verify the full flow against Mailpit.

## Prerequisites

- Step 2 completed: Cobra CLI, koanf config, SQLite store with Goose
  migrations, health endpoint, structured JSON logging, request ID
  middleware, graceful shutdown all working
- Go 1.26.0 (pinned in `mise.toml`)
- Node 22 (pinned in `mise.toml`)
- Mailpit installed (pinned in `mise.toml` as `aqua:axllent/mailpit`)
- `mise` CLI available on PATH

## New Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `maragu.dev/goqite` | v0.4.0 | Persistent background job queue (SQLite-backed) |
| `github.com/wneessen/go-mail` | v0.7.2 | SMTP email sending |

NPM (in `email/`):

| Package | Purpose |
|---------|---------|
| `@maizzle/framework` | Email template build (Tailwind CSS inlining) |
| `tailwindcss-preset-email` | Tailwind preset for email-safe CSS |

Existing: `modernc.org/sqlite`, `github.com/google/uuid`,
`github.com/pressly/goose/v3`, `github.com/spf13/cobra`,
`github.com/knadh/koanf/v2` (all from Steps 1-2).

Note: goqite v0.4.0 uses `maragu.dev/goqite` as its module path.
Its `jobs` sub-package is imported as `maragu.dev/goqite/jobs`.
`go mod tidy` resolves exact versions. The versions above are the
targets.

## Spec Traceability

All tasks trace to `openspec/specs/notification-delivery/spec.md`.

## Tasks

### Task 1: Goqite Migration

Satisfies: REQ-010 (goqite queue), REQ-011 (shared `*sql.DB`),
REQ-012 (goqite schema via goose migration).

**Files:**
- Create: `internal/infra/sqlite/migrations/002_goqite.sql`

**Step 1: Create the goqite schema migration**

```sql
-- +goose Up
create table goqite (
  id text primary key default ('m_' || lower(hex(randomblob(16)))),
  created text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  updated text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  queue text not null,
  body blob not null,
  timeout text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  received integer not null default 0,
  priority integer not null default 0
) strict;

create trigger goqite_updated_timestamp after update on goqite begin
  update goqite set updated = strftime('%Y-%m-%dT%H:%M:%fZ') where id = old.id;
end;

create index goqite_queue_priority_created_idx on goqite (queue, priority desc, created);

-- +goose Down
DROP INDEX IF EXISTS goqite_queue_priority_created_idx;
DROP TRIGGER IF EXISTS goqite_updated_timestamp;
DROP TABLE IF EXISTS goqite;
```

This is the exact contents of goqite's `schema_sqlite.sql` wrapped in
goose markers. It must be installed via migration so the table exists
before the queue is initialised.

**Step 2: Verify the migration compiles with the store**

Run: `go build ./internal/infra/sqlite/...`
Expected: exits 0 (the embedded `migrations/*.sql` glob picks up the
new file automatically)

**Step 3: Commit**

`feat(db): add goqite queue schema migration`

---

### Task 2: Email Templates (Maizzle Build)

Satisfies: REQ-019 (html/template with embedded files), REQ-020
(Maizzle build with Tailwind CSS inlining), REQ-021 (brand.json shared
colors), REQ-022 (HTML + plaintext bodies).

This task creates the Maizzle email project, brand.json, and the
notification template. The compiled output in `email/dist/` is what the
Go binary embeds at build time.

**Files:**
- Create: `brand.json`
- Create: `email/package.json`
- Create: `email/config.js`
- Create: `email/config.production.js`
- Create: `email/tailwind.config.js`
- Create: `email/emails/notification.html`
- Create: `email/emails/notification.txt`
- Create: `.mise/tasks/build/email`
- Modify: `.gitignore`

**Step 1: Create brand.json at the project root**

```json
{
  "primary": "#1a1a2e",
  "accent": "#e94560",
  "surface": "#16213e",
  "text": "#eaeaea"
}
```

This is the single source of truth for brand colors. Both the Maizzle
email config and the future frontend Tailwind config import it.

**Step 2: Create email/package.json**

```json
{
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "maizzle serve",
    "build": "maizzle build production"
  },
  "dependencies": {
    "@maizzle/framework": "latest",
    "tailwindcss-preset-email": "latest"
  }
}
```

**Step 3: Create email/config.js**

```javascript
/** @type {import('@maizzle/framework').Config} */
export default {
  build: {
    content: ['emails/**/*.html'],
  },
}
```

**Step 4: Create email/config.production.js**

```javascript
/** @type {import('@maizzle/framework').Config} */
export default {
  build: {
    content: ['emails/**/*.html'],
    output: {
      path: 'dist',
    },
  },
  css: {
    inline: true,
    purge: true,
    shorthand: true,
  },
  prettify: true,
}
```

Brand colors are consumed via `tailwind.config.js` (which imports
`brand.json` and maps them to Tailwind utility classes like
`bg-brand-primary`), not via Maizzle page variables. CSS inlining
happens at build time -- the Go service does zero runtime CSS
processing.

**Step 5: Create email/tailwind.config.js**

```javascript
import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const brand = require('../brand.json')

/** @type {import('tailwindcss').Config} */
export default {
  presets: [
    require('tailwindcss-preset-email'),
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          primary: brand.primary,
          accent: brand.accent,
          surface: brand.surface,
          text: brand.text,
        },
      },
    },
  },
}
```

**Step 6: Create email/emails/notification.html**

```html
---
subject: "You have a new notification"
---

<x-main>
  <table class="w-full">
    <tr>
      <td class="bg-brand-primary p-6">
        <h1 class="text-brand-text text-2xl font-bold m-0">Notifier</h1>
      </td>
    </tr>
    <tr>
      <td class="bg-brand-surface p-6">
        <p class="text-brand-text text-base m-0 mb-4">
          Hello,
        </p>
        <p class="text-brand-text text-base m-0 mb-4">
          A notification has been sent to <strong>@{{ .Email }}</strong>.
        </p>
        <p class="text-brand-text text-base m-0 mb-4">
          Notification ID: <code>@{{ .ID }}</code>
        </p>
      </td>
    </tr>
    <tr>
      <td class="bg-brand-primary p-4">
        <p class="text-brand-text text-xs m-0 text-center">
          Sent by Notifier &bull; Request @{{ .RequestID }}
        </p>
      </td>
    </tr>
  </table>
</x-main>
```

Note: The `@{{ .Email }}`, `@{{ .ID }}`, and `@{{ .RequestID }}` tokens
use Maizzle's escape syntax. Maizzle 5 evaluates all `{{ }}` blocks as
JavaScript expressions, so bare `{{ .Email }}` would be consumed at
build time. The `@` prefix tells Maizzle to skip evaluation and output
the `{{ }}` expression verbatim (without the `@`), preserving the Go
`html/template` tokens in the compiled HTML for runtime injection. The
plaintext template (`notification.txt`) is not processed by Maizzle so
it uses bare `{{ }}` syntax directly.

**Step 7: Create email/emails/notification.txt**

```
Hello,

A notification has been sent to {{ .Email }}.

Notification ID: {{ .ID }}

--
Sent by Notifier | Request {{ .RequestID }}
```

This plaintext template is not processed by Maizzle -- it is copied
directly to `email/dist/` by the build task. The Go binary embeds it
alongside the HTML template.

**Step 8: Add `email/node_modules/` to .gitignore**

Running `npm ci` in the `email/` directory creates `node_modules/` which
must not be committed. Add it to the project `.gitignore`:

```
email/node_modules/
```

Append this line to the existing `.gitignore` (after the existing
`email/dist/` entry in the "Email build output" section).

**Step 9: Create .mise/tasks/build/email**

```bash
#!/usr/bin/env bash
#MISE description="Build email templates (Maizzle + Tailwind CSS inlining)"
set -euo pipefail

cd email
npm ci --silent
npx maizzle build production

# Copy plaintext templates to dist (Maizzle only processes HTML).
cp emails/*.txt dist/
```

**Step 10: Make the build task executable**

Run: `chmod +x .mise/tasks/build/email`
Expected: exits 0

**Step 11: Install npm dependencies and run the email build**

Run: `mise run build:email`
Expected: exits 0, `email/dist/notification.html` and
`email/dist/notification.txt` are created. The HTML file contains
inlined CSS styles (no `<style>` block, styles are on individual
elements).

**Step 12: Commit**

`feat(email): add Maizzle email templates with brand.json styling`

---

### Task 3: Email Template Rendering (Go)

Satisfies: REQ-019 (html/template with go:embed), REQ-021 (dynamic
value injection at runtime), REQ-022 (HTML + plaintext).

**Files:**
- Create: `internal/infra/email/template.go`
- Test: `internal/infra/email/template_test.go`

**Step 1: Write the failing test**

```go
package email

import (
	"strings"
	"testing"
)

func TestRenderNotification(t *testing.T) {
	data := NotificationData{
		Email:     "user@company.com",
		ID:        "ntf_abc-123",
		RequestID: "req_def-456",
	}

	html, text, err := RenderNotification(data)
	if err != nil {
		t.Fatalf("RenderNotification() error: %v", err)
	}

	// HTML body should contain the dynamic values.
	if !strings.Contains(html, "user@company.com") {
		t.Error("HTML does not contain email address")
	}
	if !strings.Contains(html, "ntf_abc-123") {
		t.Error("HTML does not contain notification ID")
	}
	if !strings.Contains(html, "req_def-456") {
		t.Error("HTML does not contain request ID")
	}

	// Plaintext body should contain the dynamic values.
	if !strings.Contains(text, "user@company.com") {
		t.Error("plaintext does not contain email address")
	}
	if !strings.Contains(text, "ntf_abc-123") {
		t.Error("plaintext does not contain notification ID")
	}
}

func TestRenderNotificationHTMLNotEmpty(t *testing.T) {
	data := NotificationData{
		Email:     "a@b.com",
		ID:        "ntf_x",
		RequestID: "req_y",
	}
	html, _, err := RenderNotification(data)
	if err != nil {
		t.Fatalf("RenderNotification() error: %v", err)
	}
	if len(html) < 50 {
		t.Errorf("HTML body suspiciously short: %d bytes", len(html))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestRenderNotification ./internal/infra/email/...`
Expected: FAIL with "undefined: NotificationData"

**Step 3: Write the implementation**

Delete `internal/infra/email/doc.go` and create
`internal/infra/email/template.go`:

```go
package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed dist/*.html
var htmlFS embed.FS

//go:embed dist/*.txt
var textFS embed.FS

var (
	htmlTemplates = template.Must(template.ParseFS(htmlFS, "dist/*.html"))
	textTemplates = texttemplate.Must(texttemplate.ParseFS(textFS, "dist/*.txt"))
)

// NotificationData holds the dynamic values injected into the email
// templates at runtime. The HTML template has pre-inlined CSS from the
// Maizzle build -- no runtime CSS processing occurs here.
type NotificationData struct {
	Email     string
	ID        string
	RequestID string
}

// RenderNotification renders the notification email templates with the
// given data and returns the HTML body and plaintext body.
func RenderNotification(data NotificationData) (string, string, error) {
	var htmlBuf, textBuf bytes.Buffer

	if err := htmlTemplates.ExecuteTemplate(&htmlBuf, "notification.html", data); err != nil {
		return "", "", fmt.Errorf("render html: %w", err)
	}
	if err := textTemplates.ExecuteTemplate(&textBuf, "notification.txt", data); err != nil {
		return "", "", fmt.Errorf("render text: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}
```

Note: The `//go:embed dist/*.html` and `//go:embed dist/*.txt`
directives require the `email/dist/` directory to contain the compiled
templates. The `mise run build:email` task (Task 2) must have been run
before `go build` or `go test` for this package. The `dist/` directory
is committed so that `go build` works without a Node.js toolchain.

**Step 4: Run tests to verify they pass**

Run: `go test -run TestRenderNotification ./internal/infra/email/...`
Expected: PASS (2 tests)

**Step 5: Commit**

`feat(email): add Go template rendering with embedded Maizzle output`

---

### Task 4: SMTP Sender Adapter

Satisfies: REQ-014 (go-mail for SMTP), REQ-015 (implements
domain.EmailSender), REQ-016 (6-second delay), REQ-017 (@example.com
auto-fail), REQ-023 (X-Request-ID header in email).

**Files:**
- Create: `internal/infra/email/sender.go`
- Test: `internal/infra/email/sender_test.go`

**Step 1: Write the failing test**

```go
package email

import (
	"context"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestSMTPSenderImplementsInterface(t *testing.T) {
	// Compile-time check that SMTPSender implements domain.EmailSender.
	var _ domain.EmailSender = (*SMTPSender)(nil)
}

func TestExampleComAutoFail(t *testing.T) {
	sender := &SMTPSender{} // no SMTP client needed for this check
	msg := &domain.EmailMessage{
		To:      []string{"test@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for @example.com, got nil")
	}
	if !errors.Is(err, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain, got: %v", err)
	}
}

func TestExampleComCheckMultipleRecipients(t *testing.T) {
	sender := &SMTPSender{}
	msg := &domain.EmailMessage{
		To:      []string{"real@company.com", "fail@example.com"},
		Subject: "Test",
		HTML:    "<p>test</p>",
		Text:    "test",
	}

	err := sender.Send(context.Background(), msg)
	if !errors.Is(err, ErrExampleDomain) {
		t.Errorf("expected ErrExampleDomain when any recipient is @example.com, got: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestSMTPSender|TestExampleCom" ./internal/infra/email/...`
Expected: FAIL with "undefined: SMTPSender"

**Step 3: Write the implementation**

```go
package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/workfort/notifier/internal/domain"
)

// ErrExampleDomain is returned when the recipient address ends in
// @example.com, simulating an undeliverable address.
var ErrExampleDomain = errors.New("example.com: permanent delivery failure (simulated)")

// sendDelay is the artificial delay before sending an email, making
// state transitions visible in the dashboard. Extracted as a package
// variable so tests can override it.
var sendDelay = 6 * time.Second

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
// delay (REQ-016) and rejects @example.com recipients (REQ-017).
// The X-Request-ID header from the context is added to the email
// (REQ-023).
func (s *SMTPSender) Send(ctx context.Context, msg *domain.EmailMessage) error {
	// REQ-017: reject @example.com.
	for _, addr := range msg.To {
		if strings.HasSuffix(strings.ToLower(addr), "@example.com") {
			return fmt.Errorf("send to %s: %w", addr, ErrExampleDomain)
		}
	}

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

// RequestIDFromMessage extracts the request ID from the email
// message's metadata. The request ID is stored in the Subject field's
// context during job processing -- we pass it as an extra field on
// EmailMessage instead. See the RequestID field added to EmailMessage
// in the port interface update (Task 5).
//
// For now this is a placeholder that returns empty string. Task 5
// adds the RequestID field to EmailMessage and this function reads it.
func RequestIDFromMessage(_ *domain.EmailMessage) string {
	return ""
}
```

**Step 4: Run go mod tidy to fetch go-mail**

Run: `go mod tidy`
Expected: exits 0. `github.com/wneessen/go-mail` v0.7.2 added to
`go.mod`.

**Step 5: Run tests to verify they pass**

Run: `go test -run "TestSMTPSender|TestExampleCom" ./internal/infra/email/...`
Expected: PASS (3 tests)

**Step 6: Commit**

`feat(email): add SMTP sender with 6s delay and example.com auto-fail`

---

### Task 5: Add RequestID to EmailMessage and Wire It

Satisfies: REQ-023 (X-Request-ID in email header), REQ-028
(EmailMessage fields).

The domain `EmailMessage` struct needs a `RequestID` field so the
request ID from the originating API call can propagate to the email
header.

**Files:**
- Modify: `internal/domain/email.go`
- Modify: `internal/infra/email/sender.go`
- Test: `internal/infra/email/sender_test.go`

**Step 1: Write a test for request ID propagation**

Add to `internal/infra/email/sender_test.go`:

```go
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

**Step 2: Run test to verify it fails**

Run: `go test -run TestRequestIDExtraction ./internal/infra/email/...`
Expected: FAIL with "unknown field RequestID" or similar

**Step 3: Add RequestID field to EmailMessage**

Modify `internal/domain/email.go`:

```go
package domain

import "context"

// EmailMessage holds the data needed to send one email.
type EmailMessage struct {
	To        []string
	Subject   string
	HTML      string
	Text      string
	RequestID string
}

// EmailSender sends email messages. Implementations live in infra/.
type EmailSender interface {
	Send(ctx context.Context, msg *EmailMessage) error
}
```

**Step 4: Update RequestIDFromMessage in sender.go**

Replace the placeholder `RequestIDFromMessage` function in
`internal/infra/email/sender.go`:

```go
// RequestIDFromMessage extracts the request ID from the email message.
func RequestIDFromMessage(msg *domain.EmailMessage) string {
	return msg.RequestID
}
```

**Step 5: Run tests to verify they pass**

Run: `go test -run "TestRequestID|TestSMTPSender|TestExampleCom" ./internal/infra/email/...`
Expected: PASS (4 tests)

**Step 6: Commit**

`feat(domain): add RequestID to EmailMessage for X-Request-ID propagation`

---

### Task 6: Input Validation

Satisfies: REQ-006 (net/mail.ParseAddress), REQ-007 (422 for invalid
email).

Input validation is a domain concern -- it lives adjacent to the
domain types and is called by the handler before any store interaction.

**Files:**
- Create: `internal/domain/validate.go`
- Test: `internal/domain/validate_test.go`

**Step 1: Write the failing test**

```go
package domain

import (
	"errors"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid simple", "user@example.com", false},
		{"valid with name", "user@company.co.uk", false},
		{"empty string", "", true},
		{"no at sign", "not-an-email", true},
		{"no domain", "user@", true},
		{"no local part", "@example.com", true},
		{"spaces only", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("expected ErrInvalidEmail, got: %v", err)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestValidateEmail ./internal/domain/...`
Expected: FAIL with "undefined: ValidateEmail"

**Step 3: Write the implementation**

```go
package domain

import (
	"fmt"
	"net/mail"
)

// ValidateEmail checks that the email address is well-formed using
// net/mail.ParseAddress from the Go standard library (REQ-006).
// Returns ErrInvalidEmail if the address is invalid.
func ValidateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%q: %w", email, ErrInvalidEmail)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestValidateEmail ./internal/domain/...`
Expected: PASS (7 tests)

**Step 5: Commit**

`feat(domain): add email validation with net/mail.ParseAddress`

---

### Task 7: Goqite Queue and Job Runner Wiring

Satisfies: REQ-010 (goqite queue named "notifications"), REQ-011
(shared *sql.DB), REQ-013 (jobs.NewRunner with Limit 5, PollInterval
500ms), REQ-013a (MaxReceive as safety net, application-level retry).

This task creates the queue adapter that implements `httpapi.Enqueuer`
and the job runner that processes email delivery jobs. It also defines
the `EmailJobPayload` type that the notify handler (Task 9) will import
when constructing job messages.

**Files:**
- Create: `internal/infra/queue/queue.go`
- Test: `internal/infra/queue/queue_test.go`

**Step 1: Write the failing test**

```go
package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestNotificationQueueEnqueue(t *testing.T) {
	db := setupTestDB(t)
	q, err := NewNotificationQueue(db)
	if err != nil {
		t.Fatalf("NewNotificationQueue() error: %v", err)
	}

	payload := map[string]string{"email": "test@company.com"}
	data, _ := json.Marshal(payload)

	if err := q.Enqueue(context.Background(), data); err != nil {
		t.Fatalf("Enqueue() error: %v", err)
	}

	// Verify a message exists in the goqite table.
	var count int
	err = db.QueryRow("SELECT count(*) FROM goqite WHERE queue = 'notifications'").Scan(&count)
	if err != nil {
		t.Fatalf("query goqite: %v", err)
	}
	if count != 1 {
		t.Errorf("goqite message count = %d, want 1", count)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Create the goqite schema (same as migration 002).
	schema := `
	create table goqite (
		id text primary key default ('m_' || lower(hex(randomblob(16)))),
		created text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		updated text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		queue text not null,
		body blob not null,
		timeout text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		received integer not null default 0,
		priority integer not null default 0
	) strict;

	create trigger goqite_updated_timestamp after update on goqite begin
		update goqite set updated = strftime('%Y-%m-%dT%H:%M:%fZ') where id = old.id;
	end;

	create index goqite_queue_priority_created_idx on goqite (queue, priority desc, created);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create goqite schema: %v", err)
	}
	return db
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestNotificationQueue ./internal/infra/queue/...`
Expected: FAIL with "undefined: NewNotificationQueue"

**Step 3: Write the implementation**

```go
package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

const queueName = "notifications"

// EmailJobPayload is the JSON structure enqueued by the notify handler
// and deserialised by the email worker. Defined in this package as the
// queue owns the job format; the httpapi package imports it.
type EmailJobPayload struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}

// NotificationQueue wraps a goqite queue and implements
// httpapi.Enqueuer so the handler can enqueue jobs without depending
// on goqite directly.
type NotificationQueue struct {
	q *goqite.Queue
}

// NewNotificationQueue creates a goqite queue named "notifications"
// sharing the given *sql.DB (REQ-011). MaxReceive is set to 8 as a
// safety net (REQ-013a) -- application-level retry limits are enforced
// by the job handler in the worker, not by goqite.
func NewNotificationQueue(db *sql.DB) (*NotificationQueue, error) {
	q := goqite.New(goqite.NewOpts{
		DB:         db,
		Name:       queueName,
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	})
	return &NotificationQueue{q: q}, nil
}

// Queue returns the underlying goqite.Queue for use with
// jobs.NewRunner.
func (nq *NotificationQueue) Queue() *goqite.Queue {
	return nq.q
}

// Enqueue serialises the payload and creates a job in the goqite queue
// using the jobs.Create envelope format. The runner dispatches jobs by
// name, so Create must be used instead of raw q.Send(). The returned
// goqite message ID is logged for operational observability.
func (nq *NotificationQueue) Enqueue(ctx context.Context, payload []byte) error {
	msgID, err := jobs.Create(ctx, nq.q, "send_notification", goqite.Message{Body: payload})
	if err != nil {
		return fmt.Errorf("enqueue notification: %w", err)
	}
	// Log the queue message ID to correlate queue messages with
	// notification requests during debugging and operational triage.
	var p EmailJobPayload
	if jsonErr := json.Unmarshal(payload, &p); jsonErr == nil {
		slog.Info("notification job enqueued",
			"queue_message_id", string(msgID),
			"notification_id", p.NotificationID,
		)
	}
	return nil
}

// NewJobRunner creates a jobs.Runner configured per REQ-013:
// Limit 5 (max concurrent jobs), PollInterval 500ms.
func NewJobRunner(q *goqite.Queue) *jobs.Runner {
	return jobs.NewRunner(jobs.NewRunnerOpts{
		Limit:        5,
		Log:          slog.Default(),
		PollInterval: 500 * time.Millisecond,
		Queue:        q,
	})
}
```

**Step 4: Run go mod tidy to fetch goqite**

Run: `go mod tidy`
Expected: exits 0. `maragu.dev/goqite` v0.4.0 added to `go.mod`.

Note: goqite v0.4.0 depends on `github.com/mattn/go-sqlite3` (CGO)
as a direct dependency in its own `go.mod`. However, our project uses
`modernc.org/sqlite` (CGO-free) for the store. Since we pass our own
`*sql.DB` to goqite, and goqite interacts only through `database/sql`
interfaces, the CGO dependency is transitive but unused at runtime.
`go mod tidy` will include it in `go.sum` but it does not need to
compile (CGO_ENABLED=0 builds work because goqite's `go-sqlite3`
import is in test files only, not in the library code).

**Step 5: Run tests to verify they pass**

Run: `go test -run TestNotificationQueue ./internal/infra/queue/...`
Expected: PASS (1 test)

**Step 6: Commit**

`feat(queue): add goqite notification queue with EmailJobPayload and job runner`

---

### Task 8: Email Worker (Job Handler)

Satisfies: REQ-005 (async email sending), REQ-014-REQ-018 (email
delivery via worker), REQ-013a (application-level retry limit).

The worker is registered with the job runner and handles email delivery
jobs. It renders templates, sends via SMTP, and updates notification
status. State machine transitions (pending -> sending -> delivered/failed)
are introduced here in simplified form -- the full stateless integration
comes in Step 4.

**Files:**
- Create: `internal/infra/queue/worker.go`
- Test: `internal/infra/queue/worker_test.go`

**Step 1: Write the failing test**

```go
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// spyEmailSender captures sent messages.
type spyEmailSender struct {
	messages []*domain.EmailMessage
	err      error
}

func (s *spyEmailSender) Send(_ context.Context, msg *domain.EmailMessage) error {
	s.messages = append(s.messages, msg)
	return s.err
}

// spyStore captures notification updates.
type spyStore struct {
	notifications map[string]*domain.Notification
}

func newSpyStore() *spyStore {
	return &spyStore{notifications: make(map[string]*domain.Notification)}
}

func (s *spyStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.ID] = n
	return nil
}

func (s *spyStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	for _, n := range s.notifications {
		if n.Email == email {
			return n, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *spyStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.ID] = n
	return nil
}

func (s *spyStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, nil
}

func TestEmailWorkerSuccess(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_123"] = &domain.Notification{
		ID:         "ntf_123",
		Email:      "user@company.com",
		Status:     domain.StatusPending,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_123",
		Email:          "user@company.com",
		RequestID:      "req_abc",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// Verify email was sent.
	if len(sender.messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.messages))
	}
	if sender.messages[0].RequestID != "req_abc" {
		t.Errorf("RequestID = %q, want %q", sender.messages[0].RequestID, "req_abc")
	}

	// Verify notification status updated to delivered.
	n := store.notifications["ntf_123"]
	if n.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusDelivered)
	}
}

func TestEmailWorkerSendFailure(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_456"] = &domain.Notification{
		ID:         "ntf_456",
		Email:      "user@company.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{err: errors.New("smtp timeout")}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_456",
		Email:          "user@company.com",
		RequestID:      "req_def",
	})

	err := worker.Handle(context.Background(), payload)
	// Should return an error so goqite retries via visibility timeout.
	if err == nil {
		t.Fatal("expected error on send failure, got nil")
	}

	// Verify notification transitioned to not_sent with incremented retry.
	n := store.notifications["ntf_456"]
	if n.Status != domain.StatusNotSent {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusNotSent)
	}
	if n.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", n.RetryCount)
	}
}

func TestEmailWorkerRetryLimitExceeded(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_789"] = &domain.Notification{
		ID:         "ntf_789",
		Email:      "user@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: 3,
	}
	sender := &spyEmailSender{err: errors.New("still failing")}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_789",
		Email:          "user@company.com",
		RequestID:      "req_ghi",
	})

	// REQ-013a: when retry_count >= retry_limit, transition to failed
	// and return nil to acknowledge the message.
	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected nil (ack) when retry limit exceeded, got: %v", err)
	}

	n := store.notifications["ntf_789"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestEmailWorker ./internal/infra/queue/...`
Expected: FAIL with "undefined: NewEmailWorker"

**Step 3: Write the implementation**

```go
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	infraemail "github.com/workfort/notifier/internal/infra/email"

	"github.com/workfort/notifier/internal/domain"
)

// EmailWorker processes email delivery jobs from the goqite queue.
type EmailWorker struct {
	store  domain.NotificationStore
	sender domain.EmailSender
}

// NewEmailWorker creates a new worker with the given store and sender.
func NewEmailWorker(store domain.NotificationStore, sender domain.EmailSender) *EmailWorker {
	return &EmailWorker{store: store, sender: sender}
}

// Handle processes a single email delivery job. It is registered with
// the jobs.Runner via runner.Register("send_notification", worker.Handle).
//
// Flow:
//   1. Deserialise payload
//   2. Load notification from store
//   3. Check retry limit (REQ-013a) -- if exceeded, mark failed and ack
//   4. Update status to sending
//   5. Render email template
//   6. Send via SMTP (includes 6s delay per REQ-016)
//   7. On success: update to delivered, return nil
//   8. On failure: update to not_sent, increment retry_count, return error
//      (goqite retries via visibility timeout per REQ-018)
func (w *EmailWorker) Handle(ctx context.Context, payload []byte) error {
	var job EmailJobPayload
	if err := json.Unmarshal(payload, &job); err != nil {
		slog.Error("unmarshal job payload", "error", err)
		return nil // bad payload, ack to avoid infinite retry
	}

	slog.Info("processing email job",
		"notification_id", job.NotificationID,
		"email", job.Email,
	)

	// Load notification from store.
	n, err := w.store.GetNotificationByEmail(ctx, job.Email)
	if err != nil {
		slog.Error("get notification for job", "error", err, "email", job.Email)
		return fmt.Errorf("get notification: %w", err)
	}

	// REQ-013a: check retry limit before attempting send.
	if n.RetryCount >= n.RetryLimit {
		slog.Info("retry limit reached, marking as failed",
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
			"retry_limit", n.RetryLimit,
		)
		n.Status = domain.StatusFailed
		if err := w.store.UpdateNotification(ctx, n); err != nil {
			return fmt.Errorf("update notification to failed: %w", err)
		}
		return nil // ack message
	}

	// Update to sending.
	n.Status = domain.StatusSending
	if err := w.store.UpdateNotification(ctx, n); err != nil {
		return fmt.Errorf("update notification to sending: %w", err)
	}

	// Render email templates.
	html, text, err := infraemail.RenderNotification(infraemail.NotificationData{
		Email:     job.Email,
		ID:        job.NotificationID,
		RequestID: job.RequestID,
	})
	if err != nil {
		slog.Error("render email template", "error", err)
		return fmt.Errorf("render template: %w", err)
	}

	// Send via SMTP.
	msg := &domain.EmailMessage{
		To:        []string{job.Email},
		Subject:   "You have a new notification",
		HTML:      html,
		Text:      text,
		RequestID: job.RequestID,
	}

	if err := w.sender.Send(ctx, msg); err != nil {
		slog.Warn("email send failed, will retry",
			"error", err,
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
		)

		// Check if this is a permanent failure (@example.com).
		if errors.Is(err, infraemail.ErrExampleDomain) {
			n.Status = domain.StatusFailed
			if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
				slog.Error("update notification to failed", "error", updateErr)
			}
			return nil // ack -- permanent failure, no retry
		}

		// Transient failure: mark as not_sent, increment retry.
		n.Status = domain.StatusNotSent
		n.RetryCount++
		if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
			slog.Error("update notification to not_sent", "error", updateErr)
		}
		return fmt.Errorf("send email: %w", err) // return error for goqite retry
	}

	// Success: mark as delivered.
	n.Status = domain.StatusDelivered
	if err := w.store.UpdateNotification(ctx, n); err != nil {
		slog.Error("update notification to delivered", "error", err)
		return fmt.Errorf("update notification to delivered: %w", err)
	}

	slog.Info("email delivered",
		"notification_id", n.ID,
		"email", job.Email,
	)
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestEmailWorker ./internal/infra/queue/...`
Expected: PASS (3 tests)

**Step 5: Commit**

`feat(queue): add email worker with retry logic and status transitions`

---

### Task 9: Notify Handler

Satisfies: REQ-001 (POST /v1/notify), REQ-002 (202 with notification
ID), REQ-003 (ntf_ prefixed UUID), REQ-004 (create record as pending),
REQ-005 (enqueue job, not synchronous), REQ-006/REQ-007 (validation in
handler), REQ-008/REQ-009 (409 for duplicate), REQ-024 (ErrAlreadyNotified
to 409), REQ-026 (unhandled errors to 500).

**Files:**
- Create: `internal/infra/httpapi/notify.go`
- Test: `internal/infra/httpapi/notify_test.go`

**Step 1: Write the failing test**

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// stubNotificationStore is a minimal in-memory store for handler tests.
type stubNotificationStore struct {
	notifications map[string]*domain.Notification
	enqueued      []string // emails enqueued for delivery
}

func newStubStore() *stubNotificationStore {
	return &stubNotificationStore{
		notifications: make(map[string]*domain.Notification),
	}
}

func (s *stubNotificationStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	if _, exists := s.notifications[n.Email]; exists {
		return domain.ErrAlreadyNotified
	}
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (s *stubNotificationStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, nil
}

// stubEnqueuer captures enqueue calls without a real goqite queue.
type stubEnqueuer struct {
	jobs [][]byte
}

func (e *stubEnqueuer) Enqueue(_ context.Context, payload []byte) error {
	e.jobs = append(e.jobs, payload)
	return nil
}

func TestHandleNotifySuccess(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp["id"], "ntf_") {
		t.Errorf("id = %q, want ntf_ prefix", resp["id"])
	}

	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(enqueuer.jobs))
	}
}

func TestHandleNotifyDuplicate(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		Email: "user@company.com",
	}
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "already notified") {
		t.Errorf("error = %q, want 'already notified'", resp["error"])
	}
}

func TestHandleNotifyInvalidEmail(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "not-an-email"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleNotifyEmptyBody(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// Verify the stub implements the interface used by the handler.
func TestStubStoreImplementsNotificationStore(t *testing.T) {
	var _ domain.NotificationStore = newStubStore()
}

func TestStubEnqueuerImplementsEnqueuer(t *testing.T) {
	var _ Enqueuer = &stubEnqueuer{}
}

func TestHandleNotifyRequestIDPropagation(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Set a request ID in the context (simulates the middleware).
	ctx := context.WithValue(req.Context(), requestIDKey, "req_test-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Verify the enqueued job payload contains the request ID.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(enqueuer.jobs))
	}
	var payload map[string]string
	if err := json.Unmarshal(enqueuer.jobs[0], &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["request_id"] != "req_test-123" {
		t.Errorf("request_id = %q, want %q", payload["request_id"], "req_test-123")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestHandleNotify|TestStub" ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: HandleNotify"

**Step 3: Write the implementation**

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// Enqueuer abstracts the job queue so the handler does not depend on
// goqite directly. The daemon wires the real implementation.
type Enqueuer interface {
	Enqueue(ctx context.Context, payload []byte) error
}

// notifyRequest is the JSON body for POST /v1/notify.
type notifyRequest struct {
	Email string `json:"email"`
}

// notifyResponse is the JSON response for a successful notify.
type notifyResponse struct {
	ID string `json:"id"`
}

// HandleNotify returns an http.HandlerFunc for POST /v1/notify.
// It validates the email, creates a notification record, and enqueues
// a delivery job. Email sending is asynchronous (REQ-005).
func HandleNotify(store domain.NotificationStore, enqueuer Enqueuer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req notifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		// REQ-006/REQ-007: validate email format.
		if err := domain.ValidateEmail(req.Email); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": err.Error(),
			})
			return
		}

		// REQ-003: generate prefixed UUID at infra layer.
		id := domain.NewID("ntf")

		// REQ-004: create notification record as pending.
		n := &domain.Notification{
			ID:         id,
			Email:      req.Email,
			Status:     domain.StatusPending,
			RetryCount: 0,
			RetryLimit: domain.DefaultRetryLimit,
		}

		if err := store.CreateNotification(r.Context(), n); err != nil {
			// REQ-008/REQ-009/REQ-024: duplicate returns 409.
			if errors.Is(err, domain.ErrAlreadyNotified) {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": "already notified",
				})
				return
			}
			// REQ-026: unhandled errors return 500.
			slog.Error("create notification failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-005: enqueue email delivery job (async, not in handler).
		reqID := RequestIDFromContext(r.Context())
		jobPayload := queue.EmailJobPayload{
			NotificationID: id,
			Email:          req.Email,
			RequestID:      reqID,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(r.Context(), payload); err != nil {
			slog.Error("enqueue notification job failed",
				"error", err,
				"notification_id", id,
			)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-002: return 202 with notification ID.
		writeJSON(w, http.StatusAccepted, notifyResponse{ID: id})
	}
}

// writeJSON encodes v as JSON and writes it to w with the given status
// code. Shared helper for all handlers in this package.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	//nolint:errcheck // response write errors are unactionable after WriteHeader
	json.NewEncoder(w).Encode(v)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestHandleNotify|TestStub" ./internal/infra/httpapi/...`
Expected: PASS (7 tests)

**Step 5: Commit**

`feat(httpapi): add POST /v1/notify handler with validation and job enqueue`

---

### Task 10: Wire Everything in the Daemon

Satisfies: REQ-001 (endpoint registration), REQ-005 (async via
goqite), REQ-010/REQ-011 (queue setup), REQ-013 (runner config).

Connect the notify handler, queue, and worker into the daemon startup
so the full flow works end-to-end.

**Files:**
- Modify: `cmd/daemon/daemon.go`
- Modify: `cmd/daemon/daemon_test.go`

**Step 1: Update the daemon to wire queue, worker, and notify handler**

Replace the contents of `cmd/daemon/daemon.go`:

```go
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/email"
	"github.com/workfort/notifier/internal/infra/httpapi"
	"github.com/workfort/notifier/internal/infra/queue"
	"github.com/workfort/notifier/internal/infra/seed"
	"github.com/workfort/notifier/internal/infra/sqlite"
)

// NewCmd creates the daemon subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE:  run,
	}
	cmd.Flags().String("bind", "127.0.0.1", "Bind address")
	cmd.Flags().Int("port", 8080, "Listen port")
	cmd.Flags().String("db", "", "Database DSN (empty = SQLite in XDG state dir)")
	cmd.Flags().String("smtp-host", "127.0.0.1", "SMTP server host")
	cmd.Flags().Int("smtp-port", 1025, "SMTP server port")
	cmd.Flags().String("smtp-from", "notifier@localhost", "Email sender address")
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve flags: koanf (file/env) takes precedence.
	bind := resolveString(cmd, "bind")
	port := resolveInt(cmd, "port")
	dsn := resolveString(cmd, "db")
	smtpHost := resolveString(cmd, "smtp-host")
	smtpPort := resolveInt(cmd, "smtp-port")
	smtpFrom := resolveString(cmd, "smtp-from")

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return RunServer(ctx, ServerConfig{
		Bind:     bind,
		Port:     port,
		DSN:      dsn,
		SMTPHost: smtpHost,
		SMTPPort: smtpPort,
		SMTPFrom: smtpFrom,
	})
}

// ServerConfig holds configuration for RunServer.
type ServerConfig struct {
	Bind     string
	Port     int
	DSN      string
	SMTPHost string
	SMTPPort int
	SMTPFrom string
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	// Default DSN to XDG state directory.
	if cfg.DSN == "" {
		cfg.DSN = filepath.Join(config.StatePath(), "notifier.db")
	}

	// Open the store.
	store, err := sqlite.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Run QA seed data (no-op in non-QA builds).
	if err := seed.RunSeed(store.DB()); err != nil {
		return fmt.Errorf("run seed: %w", err)
	}

	// Set up goqite queue and job runner.
	nq, err := queue.NewNotificationQueue(store.DB())
	if err != nil {
		return fmt.Errorf("create notification queue: %w", err)
	}

	// Set up SMTP sender.
	sender, err := email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("create smtp sender: %w", err)
	}

	// Create and register the email worker.
	worker := queue.NewEmailWorker(store, sender)
	runner := queue.NewJobRunner(nq.Queue())
	runner.Register("send_notification", worker.Handle)

	// Start the job runner in a goroutine.
	go runner.Start(ctx)

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))

	// Apply middleware stack.
	handler := httpapi.WithMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// resolveString reads from koanf if the key exists, otherwise from
// the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	if config.K.Exists(key) {
		return config.K.String(key)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}

// resolveInt reads from koanf if the key exists, otherwise from
// the cobra flag.
func resolveInt(cmd *cobra.Command, key string) int {
	if config.K.Exists(key) {
		return config.K.Int(key)
	}
	v, _ := cmd.Flags().GetInt(key)
	return v
}
```

**Step 2: Update daemon_test.go for the new RunServer signature**

The existing `daemon_test.go` calls `RunServer(ctx, "127.0.0.1", port, dbPath)`
with 4 positional parameters. Update it to use the new `ServerConfig` struct.

Replace the contents of `cmd/daemon/daemon_test.go`:

```go
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/config"
)

func TestDaemonHealthEndpoint(t *testing.T) {
	// Set up temp XDG dirs.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := config.InitDirs(); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(); err != nil {
		t.Fatal(err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	dbPath := filepath.Join(tmp, "state", "notifier", "test.db")

	// Use a cancellable context to trigger graceful shutdown instead
	// of sending SIGINT to the process (which would interfere with
	// the test runner if other tests run concurrently).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, ServerConfig{
			Bind:     "127.0.0.1",
			Port:     port,
			DSN:      dbPath,
			SMTPHost: "127.0.0.1",
			SMTPPort: 1025,
			SMTPFrom: "test@localhost",
		})
	}()

	// Wait for server to be ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	ready := false
	for i := 0; i < 50; i++ {
		resp, err := http.Get(addr + "/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready within 5 seconds")
	}

	// Test health endpoint.
	resp, err := http.Get(addr + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify X-Request-ID header is set.
	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("X-Request-ID header missing")
	}

	// Verify response body.
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want %q", body["status"], "healthy")
	}

	// Cancel the context to trigger graceful shutdown.
	cancel()

	// Wait for server to stop.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down within 10 seconds")
	}
}
```

**Step 3: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 4: Run daemon tests to verify they pass**

Run: `go test -run TestDaemonHealthEndpoint ./cmd/daemon/...`
Expected: PASS

**Step 5: Commit**

`feat(daemon): wire notify endpoint, goqite queue, and email worker`

---

### Task 11: SMTP Configuration via Config/Env

Satisfies: daemon flag resolution for SMTP settings. Environment
variables `NOTIFIER_SMTP_HOST`, `NOTIFIER_SMTP_PORT`, and
`NOTIFIER_SMTP_FROM` override flags via koanf.

The koanf environment provider already transforms `NOTIFIER_SMTP_HOST`
to `smtp.host`, `NOTIFIER_SMTP_PORT` to `smtp.port`, and
`NOTIFIER_SMTP_FROM` to `smtp.from`. However, the cobra flag names use
hyphens (`smtp-host`), while koanf transforms underscores to dots.

Update the `resolveString`/`resolveInt` helpers in daemon.go to also
check the dotted koanf key when the flag name contains hyphens.

**Files:**
- Modify: `cmd/daemon/daemon.go`

**Step 1: Update resolve helpers to handle dotted koanf keys**

Replace the `resolveString` and `resolveInt` functions:

```go
// resolveString reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.String(dotKey)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}

// resolveInt reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveInt(cmd *cobra.Command, key string) int {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.Int(dotKey)
	}
	v, _ := cmd.Flags().GetInt(key)
	return v
}
```

Add `"strings"` to the import block in `cmd/daemon/daemon.go`.

**Step 2: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 3: Commit**

`feat(daemon): support dotted koanf keys for SMTP config via env vars`

---

### Task 12: QA Seed Data for Pending/Sending States

Satisfies: overview Build Types (QA seed for states introduced in this
step), REQ-004 (pending state).

Update the seed SQL to insert notifications in `pending` state and
enqueue corresponding delivery jobs programmatically so the QA build has
visible activity on startup. The `pending` notifications will be picked
up by the worker when the service starts (if an SMTP server is
available).

The goqite `jobs` package uses Go's `gob` binary encoding for its
message envelope, not JSON. Raw SQL cannot produce valid gob-encoded
payloads, so job enqueue must happen in Go code via `jobs.Create()`.
The seed SQL inserts notification rows only; job enqueue is handled by
a Go function that runs after the SQL seed.

**Files:**
- Modify: `internal/infra/seed/testdata/seed.sql`
- Modify: `internal/infra/seed/seed_qa.go`

**Step 1: Update seed.sql with Step 3 notification data (SQL only)**

Replace the contents of `internal/infra/seed/testdata/seed.sql`:

```sql
-- QA seed data for the notifier service.
-- Each implementation step adds INSERT statements for the states it
-- introduces. This file is embedded into the binary via //go:build qa
-- and executed on startup against a freshly migrated database.
--
-- Note: goqite job messages use gob encoding and MUST be enqueued
-- programmatically via jobs.Create() in Go code. Do NOT insert into
-- the goqite table directly from SQL.
--
-- Step 3: notifications in pending state.
-- Step 4: notifications in all states (delivered, failed, not_sent).

-- Pending notifications: the worker will attempt delivery on startup.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-001', 'alice@company.com', 0, 0, 3);

INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-002', 'bob@company.com', 0, 0, 3);

-- Pending notification to @example.com: will auto-fail on delivery.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-003', 'charlie@example.com', 0, 0, 3);
```

**Step 2: Update seed_qa.go to enqueue jobs programmatically**

Replace the contents of `internal/infra/seed/seed_qa.go`:

```go
//go:build qa

package seed

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

//go:embed testdata/seed.sql
var seedSQL []byte

// SeedSQL returns the raw embedded seed SQL. Exposed for testing.
func SeedSQL() []byte {
	return seedSQL
}

// seedJobs defines the goqite jobs to enqueue for seed notifications.
// Each entry corresponds to a notification row inserted by seed.sql.
var seedJobs = []struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}{
	{"ntf_seed-001", "alice@company.com", "req_seed-001"},
	{"ntf_seed-002", "bob@company.com", "req_seed-002"},
	{"ntf_seed-003", "charlie@example.com", "req_seed-003"},
}

// RunSeed executes the embedded seed SQL and enqueues delivery jobs
// programmatically via jobs.Create(). Called on startup when built
// with -tags qa.
func RunSeed(db *sql.DB) error {
	// Insert notification rows via SQL.
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed sql: %w", err)
	}

	// Enqueue delivery jobs via jobs.Create() so the gob-encoded
	// envelope format matches what the Runner expects.
	q := goqite.New(goqite.NewOpts{
		DB:         db,
		Name:       "notifications",
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	})

	ctx := context.Background()
	for _, sj := range seedJobs {
		payload, err := json.Marshal(sj)
		if err != nil {
			return fmt.Errorf("marshal seed job %s: %w", sj.NotificationID, err)
		}
		if _, err := jobs.Create(ctx, q, "send_notification", goqite.Message{Body: payload}); err != nil {
			return fmt.Errorf("enqueue seed job %s: %w", sj.NotificationID, err)
		}
	}

	return nil
}
```

**Step 3: Verify QA build compiles**

Run: `go build -tags qa ./...`
Expected: exits 0

**Step 4: Commit**

`feat(seed): add QA seed data for pending notifications and queue jobs`

---

### Task 13: E2E Test Harness and Mailpit Integration

Satisfies: end-to-end verification of the full notification delivery
flow including SMTP delivery to Mailpit.

This task creates the E2E test module, harness, and tests for the
scenarios defined in the notification-delivery spec. Tests verify the
full binary, real HTTP, real SQLite, and real SMTP (via Mailpit).

**Files:**
- Create: `tests/e2e/go.mod`
- Create: `tests/e2e/main_test.go`
- Create: `tests/e2e/harness_test.go`
- Create: `tests/e2e/notify_test.go`

**Step 1: Create tests/e2e/go.mod**

```
module github.com/workfort/notifier/tests/e2e

go 1.26.0
```

Run: `cd tests/e2e && go mod tidy`
Expected: exits 0

**Step 2: Create tests/e2e/main_test.go**

```go
package e2e_test

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var serviceBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "e2e-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	binPath := filepath.Join(tmp, "notifier")
	cmd := exec.Command("go", "build", "-race", "-o", binPath, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("build failed: %s\n%s", err, out)
	}
	serviceBin = binPath

	os.Exit(m.Run())
}
```

**Step 3: Create tests/e2e/harness_test.go**

```go
package e2e_test

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type Daemon struct {
	cmd    *exec.Cmd
	addr   string
	dir    string
	stderr *bytes.Buffer
}

type daemonConfig struct {
	smtpHost string
	smtpPort string
}

func defaultDaemonConfig() daemonConfig {
	return daemonConfig{
		smtpHost: "127.0.0.1",
		smtpPort: "1025",
	}
}

type DaemonOption func(*daemonConfig)

func WithSMTP(host, port string) DaemonOption {
	return func(c *daemonConfig) {
		c.smtpHost = host
		c.smtpPort = port
	}
}

func StartDaemon(t *testing.T, bin, addr string, opts ...DaemonOption) *Daemon {
	t.Helper()

	cfg := defaultDaemonConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	_, port, _ := net.SplitHostPort(addr)
	dir, err := os.MkdirTemp("", "e2e-daemon-*")
	if err != nil {
		t.Fatal(err)
	}

	d := &Daemon{
		addr:   addr,
		dir:    dir,
		stderr: &bytes.Buffer{},
	}

	d.cmd = exec.Command(bin, "daemon",
		"--bind", "127.0.0.1",
		"--port", port,
		"--smtp-host", cfg.smtpHost,
		"--smtp-port", cfg.smtpPort,
	)
	d.cmd.Stderr = d.stderr
	d.cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		"XDG_STATE_HOME="+filepath.Join(dir, "state"),
	)

	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Poll until the daemon accepts TCP connections.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}

	d.Stop()
	t.Fatalf("daemon did not become ready at %s: %s", addr, d.stderr.String())
	return nil
}

func (d *Daemon) Stop() {
	if d.cmd.Process != nil {
		_ = d.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- d.cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			_ = d.cmd.Process.Kill()
		case <-done:
		}
	}
	os.RemoveAll(d.dir)
}

func (d *Daemon) StopFatal(t *testing.T) {
	t.Helper()
	d.Stop()
	if strings.Contains(d.stderr.String(), "DATA RACE") {
		t.Fatalf("data race detected:\n%s", d.stderr.String())
	}
}

func FreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// MailpitAddr returns the Mailpit SMTP and API addresses from env
// vars, falling back to Mailpit defaults.
func MailpitAddr() (smtpHost string, smtpPort string, apiBase string) {
	smtpHost = os.Getenv("MAILPIT_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "127.0.0.1"
	}
	smtpPort = os.Getenv("MAILPIT_SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "1025"
	}
	apiPort := os.Getenv("MAILPIT_API_PORT")
	if apiPort == "" {
		apiPort = "8025"
	}
	apiBase = fmt.Sprintf("http://%s:%s", smtpHost, apiPort)
	return
}
```

**Step 4: Create tests/e2e/notify_test.go**

```go
package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Spec: notification-delivery, Scenario: Successful notification
func TestNotifySuccess(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Delete all Mailpit messages to start clean.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// WHEN a POST request is sent to /v1/notify
	body := `{"email": "e2e-test@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()

	// THEN the system SHALL return HTTP 202
	if resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, respBody)
	}

	// AND the response body SHALL contain an id matching ntf_<uuid>
	var notifyResp map[string]string
	json.NewDecoder(resp.Body).Decode(&notifyResp)
	if !strings.HasPrefix(notifyResp["id"], "ntf_") {
		t.Errorf("id = %q, want ntf_ prefix", notifyResp["id"])
	}

	// AND the X-Request-ID header SHALL be present
	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Error("X-Request-ID header missing from response")
	}

	// Wait for the background worker to process the job (6s delay + processing).
	t.Log("waiting for email delivery (6s delay + processing)...")
	var mailpitMessages struct {
		Messages []struct {
			ID      string `json:"ID"`
			Subject string `json:"Subject"`
		} `json:"messages"`
		Total int `json:"total"`
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		json.NewDecoder(resp.Body).Decode(&mailpitMessages)
		resp.Body.Close()
		if mailpitMessages.Total > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if mailpitMessages.Total == 0 {
		t.Fatal("no email received in Mailpit within timeout")
	}

	// Verify the email was delivered to the correct address.
	msgID := mailpitMessages.Messages[0].ID
	msgResp, err := http.Get(fmt.Sprintf("%s/api/v1/message/%s", mailpitAPI, msgID))
	if err != nil {
		t.Fatalf("get mailpit message: %v", err)
	}
	defer msgResp.Body.Close()

	var msgDetail struct {
		To []struct {
			Address string `json:"Address"`
		} `json:"To"`
		Subject string `json:"Subject"`
		HTML    string `json:"HTML"`
		Text    string `json:"Text"`
	}
	json.NewDecoder(msgResp.Body).Decode(&msgDetail)

	if len(msgDetail.To) == 0 || msgDetail.To[0].Address != "e2e-test@company.com" {
		t.Errorf("email To = %v, want e2e-test@company.com", msgDetail.To)
	}
	if !strings.Contains(msgDetail.HTML, "e2e-test@company.com") {
		t.Error("HTML body does not contain recipient email")
	}
	if !strings.Contains(msgDetail.Text, "e2e-test@company.com") {
		t.Error("plaintext body does not contain recipient email")
	}
}

// Spec: notification-delivery, Scenario: Duplicate notification rejected
func TestNotifyDuplicate(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// First request succeeds.
	body := `{"email": "duplicate-test@company.com"}`
	resp, _ := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("first POST: expected 202, got %d", resp.StatusCode)
	}

	// Second request for the same email returns 409.
	resp, _ = http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second POST: expected 409, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "already notified") {
		t.Errorf("error = %q, want 'already notified'", errResp["error"])
	}
}

// Spec: notification-delivery, Scenario: Invalid email rejected
func TestNotifyInvalidEmail(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	body := `{"email": "not-an-email"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

// Spec: notification-delivery, Scenario: Email to example.com auto-fails
func TestNotifyExampleComFails(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// POST succeeds (returns 202) because the failure happens async.
	body := `{"email": "test@example.com"}`
	resp, _ := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for the worker to process and fail the job.
	// The @example.com check happens before the 6s delay, so this
	// should be fast.
	time.Sleep(3 * time.Second)

	// The health endpoint should still work (service is not crashed).
	resp, err := http.Get(fmt.Sprintf("http://%s/v1/health", addr))
	if err != nil {
		t.Fatalf("health check after example.com failure: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: expected 200, got %d", resp.StatusCode)
	}
}
```

**Step 5: Run go mod tidy in the e2e directory**

Run: `cd tests/e2e && go mod tidy`
Expected: exits 0

**Step 6: Start Mailpit and run E2E tests**

Start Mailpit in a separate terminal (or it may already be running):
Run: `mailpit &`

Run E2E tests:
Run: `cd tests/e2e && go test -v -count=1 ./...`
Expected: PASS -- all 4 tests pass. TestNotifySuccess takes ~8-10
seconds due to the 6-second send delay.

**Step 7: Commit**

`test(e2e): add E2E tests for notification delivery with Mailpit`

---

### Task 14: Update Mise Tasks for Email Build Dependency

The `build:go` and `release:production` tasks should depend on
`build:email` so templates are compiled before Go embeds them.

**Files:**
- Modify: `.mise/tasks/build/go`
- Modify: `.mise/tasks/release/production`

**Step 1: Add build:email dependency to build:go**

Replace `.mise/tasks/build/go`:

```bash
#!/usr/bin/env bash
#MISE description="Build the Go binary (debug, no SPA embed)"
#MISE depends=["build:email"]
set -euo pipefail

go build -o build/notifier .
```

**Step 2: Add build:email dependency to release:production**

Replace `.mise/tasks/release/production`:

```bash
#!/usr/bin/env bash
#MISE description="Build release binary with embedded SPA"
#MISE depends=["build:web", "build:email"]
set -euo pipefail

VERSION="${VERSION:-dev}"
CGO_ENABLED=0 go build \
    -tags spa \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -trimpath \
    -o build/notifier .
```

**Step 3: Verify the build chain works**

Run: `mise run build:go`
Expected: exits 0 -- email templates are built first, then Go binary

**Step 4: Commit**

`chore(mise): add build:email dependency to Go build tasks`

## Verification Checklist

- [ ] `mise run build:email` compiles Maizzle templates into `email/dist/`
- [ ] `email/dist/notification.html` contains inlined CSS (no `<style>` block)
- [ ] `email/dist/notification.txt` contains Go template tokens
- [ ] `go build ./...` succeeds (dev build, no tags)
- [ ] `go build -tags qa ./...` succeeds (qa build)
- [ ] `go test ./...` passes all unit tests
- [ ] `go test -tags qa ./...` passes all tests including seed
- [ ] `go vet ./...` reports no issues
- [ ] `mise run build:go` produces `build/notifier`
- [ ] `mise run test:unit` exits 0
- [ ] `mise run lint:go` exits 0
- [ ] `POST /v1/notify` with valid email returns 202 and `ntf_` ID
- [ ] `POST /v1/notify` with invalid email returns 422
- [ ] `POST /v1/notify` with duplicate email returns 409 with "already notified"
- [ ] Background worker sends email via SMTP (visible in Mailpit)
- [ ] Email contains HTML body with inlined brand colors
- [ ] Email contains plaintext alternative body
- [ ] Email includes `X-Request-ID` header
- [ ] `@example.com` addresses auto-fail without SMTP attempt
- [ ] 6-second delay is observable between pending and delivered states
- [ ] QA seed data creates pending notifications on startup
- [ ] E2E tests pass against Mailpit: `cd tests/e2e && go test -v -count=1 ./...`
- [ ] No data races (race detector enabled in E2E binary build)
