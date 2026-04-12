---
type: plan
step: "1"
title: "Project Skeleton"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "1"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans: []
---

# Step 1: Project Skeleton

## Overview

Establish the project layout, Go module, domain types, error sentinels,
port interfaces, mise build tasks, Dockerfile, and QA build tag
infrastructure for the Notifier service. After this step the project
compiles, lints clean, passes unit tests for domain types, and the mise
task runner can build all three variants (dev, qa, production).

No HTTP server, database, CLI, or runtime behavior is delivered here --
that is Step 2. This step delivers the static foundation that every
subsequent step builds on.

## Prerequisites

- Go 1.26.0 installed (pinned in `mise.toml`)
- Node 22 installed (pinned in `mise.toml`)
- `golangci-lint` installed (pinned in `mise.toml` as
  `"aqua:golangci/golangci-lint"`, or installed separately and
  available on PATH). Required by the `lint:go` and `ci` mise tasks.
- `mise` CLI available on PATH
- The `mise.toml` already exists at the project root with tool versions

## Tasks

### Task 1: Initialize Go Module and main.go Entrypoint

**Files:**
- Create: `go.mod`
- Create: `main.go`

**Step 1: Create go.mod**

```
module github.com/workfort/notifier

go 1.26.0

require github.com/google/uuid v1.6.0
```

Run: `go mod tidy` in the project root after creating `main.go`.

Note: `go mod tidy` will resolve the full dependency graph and write
`go.sum`. The `google/uuid` module is the only direct dependency for
Step 1.

**Step 2: Create main.go**

```go
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	// Cobra CLI will replace this in Step 2. For now, confirm the
	// binary builds and prints its version.
	fmt.Println("notifier", Version)
	os.Exit(0)
}
```

**Step 3: Run go mod tidy**

Run: `cd /home/kazw/Work/WorkFort/skills/lead/go-service-architecture/scripts/skeleton && go mod tidy`
Expected: exits 0, `go.sum` is created

**Step 4: Verify the module compiles**

Run: `cd /home/kazw/Work/WorkFort/skills/lead/go-service-architecture/scripts/skeleton && go build -o /dev/null .`
Expected: exits 0, no output

**Step 5: Commit**

`chore(init): initialize go module and main entrypoint`

---

### Task 2: Domain Types

Satisfies: overview State Machine states, notification-delivery REQ-003
(ID format), notification-state-machine REQ-001 (states), REQ-002
(iota enums), notification-management REQ-010 (notification fields).

Note: Status and Trigger use `int` iota enums per
notification-state-machine REQ-002, matching the architecture
reference's Tier 2 state machine pattern. The simpler `type Status
string` pattern in the architecture reference applies to non-state-machine
entity statuses, not to state machine states and triggers.

**Files:**
- Create: `internal/domain/types.go`
- Test: `internal/domain/types_test.go`

**Step 1: Write the test for domain types**

```go
package domain

import (
	"testing"
)

func TestStatusStringValues(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPending, "pending"},
		{StatusSending, "sending"},
		{StatusDelivered, "delivered"},
		{StatusFailed, "failed"},
		{StatusNotSent, "not_sent"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTriggerStringValues(t *testing.T) {
	tests := []struct {
		trigger Trigger
		want    string
	}{
		{TriggerSend, "send"},
		{TriggerDelivered, "delivered"},
		{TriggerFailed, "failed"},
		{TriggerSoftFail, "soft_fail"},
		{TriggerRetry, "retry"},
		{TriggerReset, "reset"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.trigger.String(); got != tt.want {
				t.Errorf("Trigger.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNotificationDefaultRetryLimit(t *testing.T) {
	n := Notification{}
	if n.RetryLimit != 0 {
		t.Fatalf("zero-value RetryLimit = %d, want 0", n.RetryLimit)
	}
}

func TestStatusStringOutOfRange(t *testing.T) {
	if got := Status(99).String(); got != "unknown" {
		t.Errorf("Status(99).String() = %q, want %q", got, "unknown")
	}
}

func TestTriggerStringOutOfRange(t *testing.T) {
	if got := Trigger(99).String(); got != "unknown" {
		t.Errorf("Trigger(99).String() = %q, want %q", got, "unknown")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestStatusStringValues ./internal/domain/...`
Expected: FAIL with "undefined: Status" (types not yet defined)

**Step 3: Write the domain types implementation**

```go
package domain

import "time"

// Status represents the state of a notification in the state machine.
type Status int

const (
	StatusPending   Status = iota // pending
	StatusSending                 // sending
	StatusDelivered               // delivered
	StatusFailed                  // failed
	StatusNotSent                 // not_sent
)

var statusStrings = [...]string{
	"pending",
	"sending",
	"delivered",
	"failed",
	"not_sent",
}

func (s Status) String() string {
	if int(s) < len(statusStrings) {
		return statusStrings[s]
	}
	return "unknown"
}

// Trigger represents an event that causes a state transition.
type Trigger int

const (
	TriggerSend      Trigger = iota // send — worker picks up job
	TriggerDelivered                // delivered — SMTP accepted
	TriggerFailed                   // failed — permanent failure
	TriggerSoftFail                 // soft_fail — transient failure
	TriggerRetry                    // retry — automatic retry from queue
	TriggerReset                    // reset — manual reset via API
)

var triggerStrings = [...]string{
	"send",
	"delivered",
	"failed",
	"soft_fail",
	"retry",
	"reset",
}

func (t Trigger) String() string {
	if int(t) < len(triggerStrings) {
		return triggerStrings[t]
	}
	return "unknown"
}

// DefaultRetryLimit is the default number of retries before a
// notification transitions to failed permanently. The initial attempt
// is not counted as a retry, so retry limit 3 means 4 total attempts.
const DefaultRetryLimit = 3

// Notification is the core domain entity.
type Notification struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Status     Status    `json:"status"`
	RetryCount int       `json:"retry_count"`
	RetryLimit int       `json:"retry_limit"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestStatusString|TestTriggerString|TestNotificationDefaultRetryLimit" ./internal/domain/...`
Expected: PASS (5 tests)

**Step 5: Commit**

`feat(domain): add notification entity, status and trigger enums`

---

### Task 3: Error Sentinels

Satisfies: notification-delivery REQ-008/REQ-009 (ErrAlreadyNotified),
REQ-024/REQ-025 (error-to-HTTP mapping), notification-management
REQ-003 (ErrNotFound for reset).

**Files:**
- Create: `internal/domain/errors.go`
- Test: `internal/domain/errors_test.go`

**Step 1: Write the test for error sentinels**

```go
package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		message  string
	}{
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrAlreadyNotified", ErrAlreadyNotified, "already notified"},
		{"ErrInvalidEmail", ErrInvalidEmail, "invalid email address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sentinel.Error() != tt.message {
				t.Errorf("Error() = %q, want %q", tt.sentinel.Error(), tt.message)
			}
		})
	}
}

func TestErrorSentinelsUnwrap(t *testing.T) {
	wrapped := fmt.Errorf("get notification abc: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped error should match ErrNotFound via errors.Is")
	}

	wrapped2 := fmt.Errorf("create notification: %w", ErrAlreadyNotified)
	if !errors.Is(wrapped2, ErrAlreadyNotified) {
		t.Error("wrapped error should match ErrAlreadyNotified via errors.Is")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestErrorSentinels ./internal/domain/...`
Expected: FAIL with "undefined: ErrNotFound"

**Step 3: Write the error sentinels implementation**

```go
package domain

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyNotified is returned when a notification has already
	// been sent to the given email address.
	ErrAlreadyNotified = errors.New("already notified")

	// ErrInvalidEmail is returned when the email address fails format
	// validation.
	ErrInvalidEmail = errors.New("invalid email address")
)
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestErrorSentinels" ./internal/domain/...`
Expected: PASS (2 tests)

**Step 5: Commit**

`feat(domain): add error sentinels for not-found, duplicate, and invalid email`

---

### Task 4: Port Interfaces

Satisfies: service-database REQ-020 (Store composite interface),
notification-delivery REQ-027/REQ-028 (EmailSender port),
notification-management REQ-012 (HealthChecker).

**Files:**
- Create: `internal/domain/store.go`
- Create: `internal/domain/email.go`

**Step 1: Create the store port interfaces**

```go
package domain

import (
	"context"
	"io"
)

// NotificationStore persists and retrieves notification records.
type NotificationStore interface {
	CreateNotification(ctx context.Context, n *Notification) error
	GetNotificationByEmail(ctx context.Context, email string) (*Notification, error)
	UpdateNotification(ctx context.Context, n *Notification) error
	ListNotifications(ctx context.Context, after string, limit int) ([]*Notification, error)
}

// HealthChecker verifies the backing store is reachable.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Store combines all storage interfaces for use at the composition
// root. Consumers (handlers, services) accept individual interfaces,
// not Store.
type Store interface {
	NotificationStore
	HealthChecker
	io.Closer
}
```

**Step 2: Create the email sender port interface**

```go
package domain

import "context"

// EmailMessage holds the data needed to send one email.
type EmailMessage struct {
	To      []string
	Subject string
	HTML    string
	Text    string
}

// EmailSender sends email messages. Implementations live in infra/.
type EmailSender interface {
	Send(ctx context.Context, msg *EmailMessage) error
}
```

**Step 3: Verify compilation**

Run: `go build ./internal/domain/...`
Expected: exits 0, no output

**Step 4: Commit**

`feat(domain): add store and email sender port interfaces`

---

### Task 5: ID Generation Utility

Satisfies: notification-delivery REQ-003 (prefixed UUID `ntf_<uuid>`),
service-observability REQ-004 (UUID generation for request IDs).

**Files:**
- Create: `internal/domain/identity.go`
- Test: `internal/domain/identity_test.go`

**Step 1: Write the test for ID generation**

```go
package domain

import (
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	tests := []struct {
		prefix string
	}{
		{"ntf"},
		{"req"},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			id := NewID(tt.prefix)
			if !strings.HasPrefix(id, tt.prefix+"_") {
				t.Errorf("NewID(%q) = %q, want prefix %q_", tt.prefix, id, tt.prefix)
			}
			// UUID v4 is 36 chars: prefix + _ + 36 = len(prefix) + 37
			wantLen := len(tt.prefix) + 1 + 36
			if len(id) != wantLen {
				t.Errorf("NewID(%q) length = %d, want %d", tt.prefix, len(id), wantLen)
			}
		})
	}
}

func TestNewIDUniqueness(t *testing.T) {
	a := NewID("ntf")
	b := NewID("ntf")
	if a == b {
		t.Errorf("two calls to NewID returned the same value: %q", a)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestNewID ./internal/domain/...`
Expected: FAIL with "undefined: NewID"

**Step 3: Write the implementation**

```go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// NewID returns a prefixed UUID string: "<prefix>_<uuid>".
func NewID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.New().String())
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestNewID" ./internal/domain/...`
Expected: PASS (2 tests)

**Step 5: Commit**

`feat(domain): add prefixed UUID identity generation`

---

### Task 6: Directory Scaffolding and Placeholder Files

Create the directory tree for packages that later steps will populate.
Each directory gets a `doc.go` with a package comment so the package
compiles and `go vet` is satisfied.

**Files:**
- Create: `cmd/daemon/doc.go`
- Create: `cmd/mcp-bridge/doc.go`
- Create: `cmd/admin/doc.go`
- Create: `internal/config/doc.go`
- Create: `internal/infra/sqlite/doc.go`
- Create: `internal/infra/postgres/doc.go`
- Create: `internal/infra/httpapi/doc.go`
- Create: `internal/infra/mcp/doc.go`
- Create: `internal/infra/email/doc.go`
- Create: `tests/e2e/.gitkeep`
- Create: `web/.gitkeep`

**Step 1: Create package doc files**

`cmd/daemon/doc.go`:
```go
// Package daemon implements the HTTP server subcommand.
package daemon
```

`cmd/mcp-bridge/doc.go`:
```go
// Package mcpbridge implements the stdio-to-HTTP MCP bridge subcommand.
package mcpbridge
```

Note: the Go package name is `mcpbridge` (no hyphen) because Go
package names cannot contain hyphens. The directory is `mcp-bridge`
to match the CLI subcommand name.

`cmd/admin/doc.go`:
```go
// Package admin implements CLI administration commands.
package admin
```

`internal/config/doc.go`:
```go
// Package config loads service configuration from YAML files and
// environment variables via koanf and manages XDG-compliant paths.
package config
```

`internal/infra/sqlite/doc.go`:
```go
// Package sqlite implements the domain.Store interface backed by SQLite
// using modernc.org/sqlite (CGO-free).
package sqlite
```

`internal/infra/postgres/doc.go`:
```go
// Package postgres implements the domain.Store interface backed by
// PostgreSQL using pgx/v5.
package postgres
```

`internal/infra/httpapi/doc.go`:
```go
// Package httpapi implements the REST API handlers, middleware, and
// SPA serving.
package httpapi
```

`internal/infra/mcp/doc.go`:
```go
// Package mcp implements the MCP tool handlers for AI agent integration.
package mcp
```

`internal/infra/email/doc.go`:
```go
// Package email implements the domain.EmailSender interface via SMTP
// using go-mail.
package email
```

**Step 2: Create placeholder files for non-Go directories**

`tests/e2e/.gitkeep`: empty file
`web/.gitkeep`: empty file

**Step 3: Verify entire project compiles**

Run: `go build ./...`
Expected: exits 0, no output

**Step 4: Commit**

`chore(scaffold): add directory structure with package doc files`

---

### Task 7: QA Build Tag Infrastructure

Satisfies: service-build REQ-005 (seed infrastructure in step 1),
REQ-006 (build tag guard), REQ-007 (embed + execute on startup).

This task creates the seed infrastructure that later steps populate
with actual SQL. For now it contains a placeholder comment in the SQL
file and a no-op seed runner for the non-QA build.

**Files:**
- Create: `internal/infra/seed/seed_qa.go`
- Create: `internal/infra/seed/seed_default.go`
- Create: `internal/infra/seed/testdata/seed.sql`
- Test: `internal/infra/seed/seed_test.go`

**Step 1: Write the test for QA seed infrastructure**

```go
//go:build qa

package seed

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSeedSQLNotEmpty(t *testing.T) {
	data := SeedSQL()
	if len(data) == 0 {
		t.Fatal("SeedSQL() returned empty bytes in qa build")
	}
}

func TestRunSeedExecutes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// The placeholder seed SQL is just a comment, so it should
	// succeed on any database without requiring schema.
	if err := RunSeed(db); err != nil {
		t.Fatalf("RunSeed() error: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -tags qa -run TestSeedSQLNotEmpty ./internal/infra/seed/...`
Expected: FAIL with "undefined: SeedSQL" (package does not exist yet)

**Step 3: Write the QA seed implementation**

`internal/infra/seed/seed_qa.go`:
```go
//go:build qa

package seed

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed testdata/seed.sql
var seedSQL []byte

// SeedSQL returns the raw embedded seed SQL. Exposed for testing.
func SeedSQL() []byte {
	return seedSQL
}

// RunSeed executes the embedded seed SQL against the given database.
// Called on startup when built with -tags qa.
func RunSeed(db *sql.DB) error {
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed: %w", err)
	}
	return nil
}
```

`internal/infra/seed/seed_default.go`:
```go
//go:build !qa

package seed

import "database/sql"

// RunSeed is a no-op in non-QA builds.
func RunSeed(_ *sql.DB) error {
	return nil
}
```

`internal/infra/seed/testdata/seed.sql`:
```sql
-- QA seed data for the notifier service.
-- Each implementation step adds INSERT statements for the states it
-- introduces. This file is embedded into the binary via //go:build qa
-- and executed on startup against a freshly migrated database.
--
-- Step 1: placeholder (no tables exist yet).
-- Step 3: notifications in pending state + enqueued jobs.
-- Step 4: notifications in all states (delivered, failed, not_sent).
SELECT 1;
```

The `SELECT 1;` is a valid no-op statement that ensures the file is
parseable SQL. It will be replaced with real INSERT statements in
Step 3 when the notification table schema exists.

**Step 4: Run go mod tidy**

Run: `go mod tidy`
Expected: exits 0. `modernc.org/sqlite` and its transitive
dependencies are added to `go.mod` and `go.sum`. This dependency is
pulled in by the QA build tag test only (`seed_test.go` imports the
`modernc.org/sqlite` driver).

**Step 5: Run tests with qa build tag**

Run: `go test -tags qa -run "TestSeedSQL|TestRunSeed" ./internal/infra/seed/...`
Expected: PASS (2 tests)

**Step 6: Verify default build compiles (no build tag)**

Run: `go build ./internal/infra/seed/...`
Expected: exits 0, no output (the default stub compiles without
the `modernc.org/sqlite` dependency)

**Step 7: Commit**

`feat(seed): add QA build tag infrastructure with placeholder seed SQL`

---

### Task 8: Mise Build Tasks

Satisfies: service-build REQ-010 through REQ-019.

All tasks are executable bash scripts under `.mise/tasks/`.
Subdirectories create colon-separated namespaces.

**Files:**
- Create: `.mise/tasks/build/go`
- Create: `.mise/tasks/build/web`
- Create: `.mise/tasks/release/dev`
- Create: `.mise/tasks/release/production`
- Create: `.mise/tasks/test/unit`
- Create: `.mise/tasks/test/e2e`
- Create: `.mise/tasks/dev/web`
- Create: `.mise/tasks/dev/storybook`
- Create: `.mise/tasks/lint/go`
- Create: `.mise/tasks/clean/go`
- Create: `.mise/tasks/ci`

**Step 1: Create .mise/tasks/build/go**

```bash
#!/usr/bin/env bash
#MISE description="Build the Go binary (debug, no SPA embed)"
set -euo pipefail

go build -o build/notifier .
```

**Step 2: Create .mise/tasks/build/web**

```bash
#!/usr/bin/env bash
#MISE description="Build the frontend (npm run build in web/)"
set -euo pipefail

cd web
npm run build
```

**Step 3: Create .mise/tasks/release/dev**

```bash
#!/usr/bin/env bash
#MISE description="Build debug binary with race detector"
set -euo pipefail

go build -race -o build/notifier .
```

**Step 4: Create .mise/tasks/release/production**

```bash
#!/usr/bin/env bash
#MISE description="Build release binary with embedded SPA"
#MISE depends=["build:web"]
set -euo pipefail

VERSION="${VERSION:-dev}"
CGO_ENABLED=0 go build \
    -tags spa \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -trimpath \
    -o build/notifier .
```

**Step 5: Create .mise/tasks/test/unit**

```bash
#!/usr/bin/env bash
#MISE description="Run unit tests"
set -euo pipefail

go test ./...
```

**Step 6: Create .mise/tasks/test/e2e**

```bash
#!/usr/bin/env bash
#MISE description="Run end-to-end tests"
#MISE depends=["build:go"]
set -euo pipefail

cd tests/e2e
go test -v -count=1 ./...
```

**Step 7: Create .mise/tasks/dev/web**

```bash
#!/usr/bin/env bash
#MISE description="Start the Vite dev server"
set -euo pipefail

cd web
npm run dev
```

**Step 8: Create .mise/tasks/dev/storybook**

```bash
#!/usr/bin/env bash
#MISE description="Start the Storybook dev server on port 6006"
set -euo pipefail

cd web
npm run storybook -- --port 6006
```

**Step 9: Create .mise/tasks/lint/go**

```bash
#!/usr/bin/env bash
#MISE description="Run Go linter"
set -euo pipefail

golangci-lint run ./...
```

**Step 10: Create .mise/tasks/clean/go**

```bash
#!/usr/bin/env bash
#MISE description="Remove Go build artifacts"
set -euo pipefail

rm -rf build/
```

**Step 11: Create .mise/tasks/ci**

```bash
#!/usr/bin/env bash
#MISE description="Run full CI checks"
#MISE depends=["lint:go", "test:unit", "build:go"]
set -euo pipefail

echo "CI passed"
```

**Step 12: Make all task scripts executable**

Run: `chmod +x .mise/tasks/build/go .mise/tasks/build/web .mise/tasks/release/dev .mise/tasks/release/production .mise/tasks/test/unit .mise/tasks/test/e2e .mise/tasks/dev/web .mise/tasks/dev/storybook .mise/tasks/lint/go .mise/tasks/clean/go .mise/tasks/ci`
Expected: exits 0

**Step 13: Verify mise sees the tasks**

Run: `mise tasks`
Expected: output includes `build:go`, `build:web`, `test:unit`,
`test:e2e`, `release:dev`, `release:production`, `lint:go`,
`clean:go`, `ci`, `dev:web`, `dev:storybook`

**Step 14: Run the build task to verify it works**

Run: `mise run build:go`
Expected: exits 0, `build/notifier` binary is created

**Step 15: Run the unit test task**

Run: `mise run test:unit`
Expected: exits 0, all domain tests pass

**Step 16: Commit**

`chore(mise): add build, test, release, dev, lint, and ci tasks`

---

### Task 9: Dockerfile

Satisfies: service-build REQ-021 (multi-stage with distroless),
REQ-022 (build flags), REQ-023 (nonroot user), REQ-024 (entrypoint
and CMD).

**Files:**
- Create: `Dockerfile`

**Step 1: Create the Dockerfile**

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o build/notifier .

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/build/notifier /usr/local/bin/notifier
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/notifier"]
CMD ["daemon"]
```

Notes:
- `modernc.org/sqlite` is CGO-free, so `distroless/static` works
  without libc.
- The `COPY go.mod go.sum` + `RUN go mod download` layer is cached
  separately from the source copy for faster rebuilds.
- The SPA is not embedded here because the Dockerfile does not run
  `npm build`. A production Dockerfile that embeds the SPA would add
  a Node build stage and use `-tags spa`. That refinement comes in
  Step 7 when the frontend is built.

**Step 2: Verify Dockerfile syntax (dry run)**

Run: `head -1 Dockerfile`
Expected: `FROM golang:1.26-alpine AS build`

This is a syntax sanity check. A full Docker build requires Docker
and is outside the scope of this step's verification.

**Step 3: Commit**

`chore(docker): add multi-stage Dockerfile with distroless runtime`

---

### Task 10: .gitignore

**Files:**
- Create: `.gitignore`

**Step 1: Create .gitignore**

```
# Build output
build/

# Go
*.exe
*.test
*.out

# Node
web/node_modules/
web/dist/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Email build output
email/dist/
```

**Step 2: Commit**

`chore: add .gitignore for build artifacts and editor files`

---

### Task 11: Final Full-Project Verification

**Depends on:** Tasks 1-10

**Step 1: Verify the full project compiles (dev build, no tags)**

Run: `mise run build:go`
Expected: exits 0, `build/notifier` binary exists

**Step 2: Run all unit tests**

Run: `mise run test:unit`
Expected: PASS — all domain tests pass

**Step 3: Run unit tests with qa build tag**

Run: `go test -tags qa ./...`
Expected: PASS — seed infrastructure tests pass alongside domain tests

**Step 4: Verify the release:dev build (race detector)**

Run: `mise run release:dev`
Expected: exits 0, `build/notifier` binary is created with the race
detector enabled

**Step 5: Verify the binary runs**

Run: `./build/notifier`
Expected: prints `notifier dev` and exits 0

**Step 6: Clean up build artifacts**

Run: `mise run clean:go`
Expected: `build/` directory removed

**Step 7: Commit (if any final adjustments were needed)**

`chore(skeleton): finalize step 1 project skeleton`

Only create this commit if adjustments were required during
verification. If everything passed cleanly, no commit is needed.

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings (dev build, no tags)
- [ ] `go build -tags qa ./...` succeeds (qa build)
- [ ] `go test ./...` passes all domain type and error sentinel tests
- [ ] `go test -tags qa ./...` passes all tests including seed infrastructure
- [ ] `go vet ./...` reports no issues
- [ ] `mise run build:go` produces `build/notifier`
- [ ] `mise run release:dev` produces `build/notifier` with race detector
- [ ] `mise run test:unit` exits 0
- [ ] `mise run lint:go` exits 0 (requires `golangci-lint`)
- [ ] `mise run ci` exits 0 (depends on lint:go, test:unit, build:go)
- [ ] `./build/notifier` prints version and exits
- [ ] `mise tasks` lists all 11 tasks
- [ ] All `.mise/tasks/*` scripts are executable
- [ ] Dockerfile starts with `FROM golang:1.26-alpine`
- [ ] Directory structure matches the layout in the overview
- [ ] No orphan packages — every directory under `internal/` compiles
- [ ] `go.sum` exists and is tracked
