---
type: plan
step: "4"
title: "State Machine"
status: pending
assessment_status: in_progress
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "4"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-2-cli-and-database
  - step-3-notification-delivery
---

# Step 4: State Machine

## Overview

Replace the ad-hoc state transitions in the email worker with a formal
state machine powered by `qmuntal/stateless`. After this step every
notification lifecycle transition is governed by the state machine
configuration in the domain layer, invalid transitions are rejected,
every state change is recorded in an audit log table, retry
count/limit logic is enforced by guards, and the QA seed includes
notifications in all terminal states (delivered, failed, not_sent).

The worker in Step 3 already handles the basic flow (pending -> sending
-> delivered/failed/not_sent) with inline status assignments and retry
checks. This step extracts that logic into a `stateless` state machine
so that:

- Transition rules live in the domain, not scattered across infra code.
- Invalid transitions are impossible (the library rejects them).
- Guards enforce the retry-limit-exceeded -> failed path.
- An audit log table records every state change for observability.
- The worker becomes a thin orchestrator that fires triggers instead of
  setting statuses directly.

## Prerequisites

- Step 3 completed: notify endpoint, goqite queue, email worker, SMTP
  sender, QA seed for pending state, Mailpit E2E tests all working
- Go 1.26.0 (pinned in `mise.toml`)
- `mise` CLI available on PATH

## New Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/qmuntal/stateless` | v1.8.0 | Finite state machine with external storage |

Existing: `modernc.org/sqlite`, `github.com/google/uuid`,
`github.com/pressly/goose/v3`, `maragu.dev/goqite`,
`github.com/wneessen/go-mail` (all from Steps 1-3).

Note: `go mod tidy` resolves exact versions. The version above is the
target -- `go mod tidy` may select a compatible patch version.

## Spec Traceability

All tasks trace to `openspec/specs/notification-state-machine/spec.md`.

**REQ-021 note:** REQ-021 requires `retry_count` and `retry_limit` to
be visible in the API response and dashboard. The fields already exist
on the `Notification` struct with JSON tags (`json:"retry_count"` and
`json:"retry_limit"`), so the existing `/v1/notify` POST response and
the future list endpoint will include them. Dashboard visibility is
deferred to Step 5 (list endpoint and frontend). The verification
checklist below includes a check confirming the POST response returns
these fields.

## Tasks

### Task 1: State Machine Configuration in Domain

Satisfies: REQ-002 (iota enums), REQ-003 (pending -> sending),
REQ-004 (sending -> delivered), REQ-005 (sending -> failed), REQ-006
(sending -> not_sent), REQ-007 (not_sent -> sending), REQ-008
(terminal -> pending reset), REQ-009 (pending -> failed NOT permitted),
REQ-010 (pending -> delivered NOT permitted), REQ-011 (delivered ->
sending NOT permitted), REQ-012 (failed -> sending NOT permitted),
REQ-013 (stateless with external storage), REQ-014 (domain layer,
accessor/mutator parameters), REQ-016 (FiringQueued), REQ-020 (guard
for retry limit exhausted), REQ-024 (delivered/failed terminal),
REQ-025 (not_sent is not terminal).

**Files:**
- Create: `internal/domain/statemachine.go`
- Test: `internal/domain/statemachine_test.go`

**Step 1: Write the failing test**

```go
package domain

import (
	"context"
	"testing"

	"github.com/qmuntal/stateless"
)

// memState is a test helper that provides in-memory accessor/mutator.
type memState struct {
	state Status
}

func (m *memState) accessor(_ context.Context) (stateless.State, error) {
	return m.state, nil
}

func (m *memState) mutator(_ context.Context, s stateless.State) error {
	m.state = s.(Status)
	return nil
}

func TestStateMachinePermittedTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    Status
		trigger Trigger
		wantTo  Status
	}{
		{"pending to sending", StatusPending, TriggerSend, StatusSending},
		{"sending to delivered", StatusSending, TriggerDelivered, StatusDelivered},
		{"sending to failed", StatusSending, TriggerFailed, StatusFailed},
		{"sending to not_sent", StatusSending, TriggerSoftFail, StatusNotSent},
		{"not_sent to sending", StatusNotSent, TriggerRetry, StatusSending},
		{"delivered to pending (reset)", StatusDelivered, TriggerReset, StatusPending},
		{"failed to pending (reset)", StatusFailed, TriggerReset, StatusPending},
		{"not_sent to pending (reset)", StatusNotSent, TriggerReset, StatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &memState{state: tt.from}
			// Use a high retry limit so the guard does not block.
			sm := ConfigureStateMachine(ms.accessor, ms.mutator, 10, 0)

			if err := sm.FireCtx(context.Background(), tt.trigger); err != nil {
				t.Fatalf("Fire(%v) from %v: %v", tt.trigger, tt.from, err)
			}
			if ms.state != tt.wantTo {
				t.Errorf("state = %v, want %v", ms.state, tt.wantTo)
			}
		})
	}
}

func TestStateMachineRejectedTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    Status
		trigger Trigger
	}{
		{"pending to failed", StatusPending, TriggerFailed},
		{"pending to delivered", StatusPending, TriggerDelivered},
		{"delivered to sending", StatusDelivered, TriggerSend},
		{"failed to sending", StatusFailed, TriggerSend},
		{"failed to sending via retry", StatusFailed, TriggerRetry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &memState{state: tt.from}
			sm := ConfigureStateMachine(ms.accessor, ms.mutator, 10, 0)

			err := sm.FireCtx(context.Background(), tt.trigger)
			if err == nil {
				t.Fatalf("Fire(%v) from %v: expected error, got nil", tt.trigger, tt.from)
			}
			// State should not change.
			if ms.state != tt.from {
				t.Errorf("state changed to %v, want %v (unchanged)", ms.state, tt.from)
			}
		})
	}
}

func TestStateMachineRetryLimitGuard(t *testing.T) {
	// When retry_count >= retry_limit, TriggerSoftFail from sending
	// should be rejected by the guard, and TriggerFailed should be
	// used instead.
	ms := &memState{state: StatusSending}
	sm := ConfigureStateMachine(ms.accessor, ms.mutator, 3, 3)

	// soft_fail should be blocked by the "retries remaining" guard.
	err := sm.FireCtx(context.Background(), TriggerSoftFail)
	if err == nil {
		t.Fatal("Fire(TriggerSoftFail) with exhausted retries: expected error, got nil")
	}
	if ms.state != StatusSending {
		t.Errorf("state = %v, want StatusSending (unchanged)", ms.state)
	}

	// TriggerFailed should still work from sending.
	if err := sm.FireCtx(context.Background(), TriggerFailed); err != nil {
		t.Fatalf("Fire(TriggerFailed) from sending: %v", err)
	}
	if ms.state != StatusFailed {
		t.Errorf("state = %v, want StatusFailed", ms.state)
	}
}

func TestStateMachineRetryAllowedWhenUnderLimit(t *testing.T) {
	ms := &memState{state: StatusSending}
	sm := ConfigureStateMachine(ms.accessor, ms.mutator, 3, 1)

	if err := sm.FireCtx(context.Background(), TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail) with retries remaining: %v", err)
	}
	if ms.state != StatusNotSent {
		t.Errorf("state = %v, want StatusNotSent", ms.state)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestStateMachine" ./internal/domain/...`
Expected: FAIL with "undefined: ConfigureStateMachine"

**Step 3: Add the stateless dependency**

Run: `cd /path/to/skeleton && go get github.com/qmuntal/stateless@v1.8.0 && go mod tidy`
Expected: `go.mod` and `go.sum` updated with the stateless dependency

**Step 4: Write the implementation**

```go
package domain

import (
	"context"

	"github.com/qmuntal/stateless"
)

// ConfigureStateMachine creates a stateless state machine with the
// notification lifecycle transitions. The accessor and mutator
// functions connect the machine to external storage (provided by the
// infra layer). retryLimit and retryCount are the current values for
// the notification being processed -- the guard on TriggerSoftFail
// uses them to decide whether retries are still allowed.
//
// This function lives in the domain layer and does not import any
// infrastructure packages (REQ-014).
func ConfigureStateMachine(
	accessor func(ctx context.Context) (stateless.State, error),
	mutator func(ctx context.Context, state stateless.State) error,
	retryLimit int,
	retryCount int,
) *stateless.StateMachine {
	sm := stateless.NewStateMachineWithExternalStorage(
		accessor, mutator, stateless.FiringQueued,
	)

	// REQ-003: pending -> sending (worker picks up job).
	sm.Configure(StatusPending).
		Permit(TriggerSend, StatusSending)

	// REQ-004: sending -> delivered (SMTP accepted).
	// REQ-005: sending -> failed (permanent failure).
	// REQ-006: sending -> not_sent (transient failure, retries remaining).
	// REQ-020: guard blocks TriggerSoftFail when retry limit exhausted.
	sm.Configure(StatusSending).
		Permit(TriggerDelivered, StatusDelivered).
		Permit(TriggerFailed, StatusFailed).
		Permit(TriggerSoftFail, StatusNotSent, func(_ context.Context, _ ...any) bool {
			return retryCount < retryLimit
		})

	// REQ-004: delivered is terminal (REQ-024).
	// REQ-008: delivered -> pending (reset).
	sm.Configure(StatusDelivered).
		Permit(TriggerReset, StatusPending)

	// REQ-005: failed is terminal (REQ-024).
	// REQ-008: failed -> pending (reset).
	sm.Configure(StatusFailed).
		Permit(TriggerReset, StatusPending)

	// REQ-007: not_sent -> sending (automatic retry).
	// REQ-008: not_sent -> pending (reset).
	// REQ-025: not_sent is NOT terminal.
	sm.Configure(StatusNotSent).
		Permit(TriggerRetry, StatusSending).
		Permit(TriggerReset, StatusPending)

	return sm
}
```

**Step 5: Run tests to verify they pass**

Run: `go test -run "TestStateMachine" ./internal/domain/...`
Expected: PASS (4 tests)

**Step 6: Commit**

`feat(domain): add stateless state machine configuration with guards`

---

### Task 2: Audit Log Migration

Satisfies: REQ-022 (state_transitions audit log table), REQ-023
(entity_type, entity_id, from_state, to_state, trigger, created_at).

**Files:**
- Create: `internal/infra/sqlite/migrations/003_state_transitions.sql`

**Step 1: Create the audit log migration**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS state_transitions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_state_transitions_entity
    ON state_transitions(entity_type, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_state_transitions_entity;
DROP TABLE IF EXISTS state_transitions;
```

The `from_state`, `to_state`, and `trigger` columns store the human-
readable string values (e.g., "pending", "sending", "send") rather
than integer iota values. This makes the audit log self-documenting
when queried directly via SQL.

**Step 2: Verify the migration compiles with the store**

Run: `go build ./internal/infra/sqlite/...`
Expected: exits 0 (the embedded `migrations/*.sql` glob picks up the
new file automatically)

**Step 3: Commit**

`feat(db): add state_transitions audit log migration`

---

### Task 3: Store Accessor, Mutator, and Audit Log Methods

Satisfies: REQ-013 (external storage via accessor/mutator), REQ-015
(accessor reads from database, mutator writes to database), REQ-022
(audit log insertion).

**Depends on:** Task 2 (audit log migration)

**Files:**
- Modify: `internal/infra/sqlite/store.go`
- Test: `internal/infra/sqlite/store_test.go`

**Step 1: Write the failing tests**

Add the following tests to `internal/infra/sqlite/store_test.go`:

```go
func TestNotificationStateAccessor(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_acc-1",
		Email:      "accessor@test.com",
		Status:     domain.StatusSending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	accessor := store.NotificationStateAccessor("ntf_acc-1")
	state, err := accessor(ctx)
	if err != nil {
		t.Fatalf("accessor() error: %v", err)
	}
	if state.(domain.Status) != domain.StatusSending {
		t.Errorf("state = %v, want StatusSending", state)
	}
}

func TestNotificationStateMutator(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_mut-1",
		Email:      "mutator@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	mutator := store.NotificationStateMutator("ntf_mut-1")
	if err := mutator(ctx, domain.StatusSending); err != nil {
		t.Fatalf("mutator() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "mutator@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusSending {
		t.Errorf("status = %v, want StatusSending", got.Status)
	}
}

func TestLogTransition(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	if err := store.LogTransition(ctx, "notification", "ntf_log-1",
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition() error: %v", err)
	}

	// Verify the row was inserted.
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?",
		"ntf_log-1",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("transition count = %d, want 1", count)
	}
}

func TestLogTransitionMultipleEntries(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	transitions := []struct {
		from    domain.Status
		to      domain.Status
		trigger domain.Trigger
	}{
		{domain.StatusPending, domain.StatusSending, domain.TriggerSend},
		{domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail},
		{domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry},
		{domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered},
	}

	for _, tr := range transitions {
		if err := store.LogTransition(ctx, "notification", "ntf_log-2",
			tr.from, tr.to, tr.trigger); err != nil {
			t.Fatalf("LogTransition(%v -> %v): %v", tr.from, tr.to, err)
		}
	}

	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?",
		"ntf_log-2",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("transition count = %d, want 4", count)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestNotificationStateAccessor|TestNotificationStateMutator|TestLogTransition" ./internal/infra/sqlite/...`
Expected: FAIL with "store.NotificationStateAccessor undefined"

**Step 3: Write the implementation**

Add the following methods to `internal/infra/sqlite/store.go`:

```go
// NotificationStateAccessor returns an accessor function for the
// stateless state machine. It reads the current status of a
// notification by ID from the database (REQ-015).
func (s *Store) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(ctx context.Context) (stateless.State, error) {
		var state int
		err := s.db.QueryRowContext(ctx,
			"SELECT status FROM notifications WHERE id = ?", notificationID,
		).Scan(&state)
		if err != nil {
			return nil, fmt.Errorf("read notification state: %w", err)
		}
		return domain.Status(state), nil
	}
}

// NotificationStateMutator returns a mutator function for the
// stateless state machine. It writes the new status to the database
// and updates the updated_at timestamp (REQ-015).
func (s *Store) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(ctx context.Context, state stateless.State) error {
		now := time.Now().UTC()
		_, err := s.db.ExecContext(ctx,
			"UPDATE notifications SET status = ?, updated_at = ? WHERE id = ?",
			int(state.(domain.Status)), now.Format(time.RFC3339), notificationID,
		)
		if err != nil {
			return fmt.Errorf("write notification state: %w", err)
		}
		return nil
	}
}

// LogTransition records a state transition in the audit log table
// (REQ-022, REQ-023). Stores human-readable string values.
func (s *Store) LogTransition(ctx context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entityType, entityID, from.String(), to.String(), trigger.String(),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("log transition: %w", err)
	}
	return nil
}
```

Add the `stateless` import to the import block in `store.go`:

```go
import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/qmuntal/stateless"
	_ "modernc.org/sqlite"

	"github.com/workfort/notifier/internal/domain"
)
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestNotificationStateAccessor|TestNotificationStateMutator|TestLogTransition" ./internal/infra/sqlite/...`
Expected: PASS (4 tests)

**Step 5: Commit**

`feat(sqlite): add state machine accessor, mutator, and audit log methods`

---

### Task 4: Add TransitionLogger to Domain Store Interface

Satisfies: REQ-022 (audit log recording), REQ-013 (domain port for
transition logging).

The worker needs to log transitions through a domain interface so it
does not depend on the SQLite store directly.

**Depends on:** Task 3 (store methods)

**Files:**
- Modify: `internal/domain/store.go`

**Step 1: Add the TransitionLogger interface**

Add the following to `internal/domain/store.go`:

```go
// TransitionLogger records state transitions for audit purposes.
type TransitionLogger interface {
	LogTransition(ctx context.Context, entityType, entityID string, from, to Status, trigger Trigger) error
}
```

Update the `Store` aggregate interface to include it:

```go
// Store combines all storage interfaces for use at the composition
// root. Consumers (handlers, services) accept individual interfaces,
// not Store.
type Store interface {
	NotificationStore
	TransitionLogger
	HealthChecker
	io.Closer
}
```

**Step 2: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 3: Commit**

`feat(domain): add TransitionLogger port for audit log`

---

### Task 5: Refactor Worker to Use State Machine

Satisfies: REQ-003 (pending -> sending), REQ-004 (sending -> delivered),
REQ-005 (sending -> failed), REQ-006 (sending -> not_sent), REQ-013
(stateless integration in worker), REQ-015 (accessor/mutator backed by
database), REQ-017 (retry_count tracking), REQ-019 (increment
retry_count on soft fail), REQ-020 (retry limit guard), REQ-021
(retry fields present in API response -- partial, dashboard deferred
to Step 5), REQ-022 (audit log on every transition).

**Depends on:** Task 1 (state machine config), Task 3 (accessor/mutator),
Task 4 (TransitionLogger interface)

**Files:**
- Modify: `internal/infra/queue/worker.go`
- Modify: `internal/infra/queue/worker_test.go`

**Step 1: Write the updated tests**

Replace the contents of `internal/infra/queue/worker_test.go`:

```go
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/qmuntal/stateless"

	infraemail "github.com/workfort/notifier/internal/infra/email"

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

// spyStore satisfies WorkerStore using in-memory maps. The accessor
// and mutator read/write the notification's Status field in the map,
// mimicking the database-backed versions in the real store.
type spyStore struct {
	notifications map[string]*domain.Notification
	transitions   []transitionRecord
}

type transitionRecord struct {
	entityType string
	entityID   string
	from       domain.Status
	to         domain.Status
	trigger    domain.Trigger
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

func (s *spyStore) LogTransition(_ context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	s.transitions = append(s.transitions, transitionRecord{
		entityType: entityType,
		entityID:   entityID,
		from:       from,
		to:         to,
		trigger:    trigger,
	})
	return nil
}

func (s *spyStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		n, ok := s.notifications[notificationID]
		if !ok {
			return nil, domain.ErrNotFound
		}
		return n.Status, nil
	}
}

func (s *spyStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, state stateless.State) error {
		n, ok := s.notifications[notificationID]
		if !ok {
			return domain.ErrNotFound
		}
		n.Status = state.(domain.Status)
		return nil
	}
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

	// Verify audit log: pending->sending, sending->delivered.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[0].from != domain.StatusPending || store.transitions[0].to != domain.StatusSending {
		t.Errorf("transition[0] = %v->%v, want pending->sending",
			store.transitions[0].from, store.transitions[0].to)
	}
	if store.transitions[1].from != domain.StatusSending || store.transitions[1].to != domain.StatusDelivered {
		t.Errorf("transition[1] = %v->%v, want sending->delivered",
			store.transitions[1].from, store.transitions[1].to)
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

	// Verify audit log: pending->sending, sending->not_sent.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
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

	// When retry_count >= retry_limit, the worker fires TriggerRetry
	// (not_sent -> sending), attempts send, send fails, then the
	// soft_fail guard rejects TriggerSoftFail, so it fires
	// TriggerFailed (sending -> failed) and acks.
	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected nil (ack) when retry limit exceeded, got: %v", err)
	}

	n := store.notifications["ntf_789"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}

	// Verify audit log: not_sent->sending, sending->failed.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[1].to != domain.StatusFailed {
		t.Errorf("transition[1].to = %v, want failed", store.transitions[1].to)
	}
}

func TestEmailWorkerExampleComAutoFail(t *testing.T) {
	store := newSpyStore()
	store.notifications["ntf_ex1"] = &domain.Notification{
		ID:         "ntf_ex1",
		Email:      "test@example.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	// Sender returns ErrExampleDomain for @example.com addresses.
	sender := &spyEmailSender{err: fmt.Errorf("send to test@example.com: %w", infraemail.ErrExampleDomain)}
	worker := NewEmailWorker(store, sender)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_ex1",
		Email:          "test@example.com",
		RequestID:      "req_ex1",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// @example.com path: pending -> sending -> failed (not not_sent).
	n := store.notifications["ntf_ex1"]
	if n.Status != domain.StatusFailed {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusFailed)
	}

	// Verify audit log: pending->sending, sending->failed.
	if len(store.transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(store.transitions))
	}
	if store.transitions[0].from != domain.StatusPending || store.transitions[0].to != domain.StatusSending {
		t.Errorf("transition[0] = %v->%v, want pending->sending",
			store.transitions[0].from, store.transitions[0].to)
	}
	if store.transitions[1].from != domain.StatusSending || store.transitions[1].to != domain.StatusFailed {
		t.Errorf("transition[1] = %v->%v, want sending->failed",
			store.transitions[1].from, store.transitions[1].to)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run TestEmailWorker ./internal/infra/queue/...`
Expected: FAIL (NewEmailWorker signature changed -- now takes 2 args)

**Step 3: Write the updated worker implementation**

Replace the contents of `internal/infra/queue/worker.go`:

```go
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/qmuntal/stateless"

	infraemail "github.com/workfort/notifier/internal/infra/email"

	"github.com/workfort/notifier/internal/domain"
)

// WorkerStore combines the notification storage and state machine
// accessor/mutator interfaces needed by the worker. The SQLite Store
// satisfies this interface.
type WorkerStore interface {
	domain.NotificationStore
	domain.TransitionLogger
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// EmailWorker processes email delivery jobs from the goqite queue.
// It uses the domain state machine for all state transitions and
// logs each transition to the audit log.
type EmailWorker struct {
	store  WorkerStore
	sender domain.EmailSender
}

// NewEmailWorker creates a new worker with the given store and email
// sender. The store provides notification CRUD, state machine
// accessor/mutator (database-backed), and transition logging.
func NewEmailWorker(store WorkerStore, sender domain.EmailSender) *EmailWorker {
	return &EmailWorker{store: store, sender: sender}
}

// fireAndLog fires a trigger on the state machine and logs the
// transition to the audit log. The accessor/mutator write to the
// database, so no separate UpdateNotification call is needed for
// the status change.
func (w *EmailWorker) fireAndLog(ctx context.Context, sm *stateless.StateMachine, n *domain.Notification, from domain.Status, trigger domain.Trigger, to domain.Status) error {
	if err := sm.FireCtx(ctx, trigger); err != nil {
		return fmt.Errorf("fire %v: %w", trigger, err)
	}
	if err := w.store.LogTransition(ctx, "notification", n.ID, from, to, trigger); err != nil {
		slog.Error("log transition failed", "error", err,
			"notification_id", n.ID, "from", from, "to", to)
		// Log failure is non-fatal; the state change already succeeded.
	}
	return nil
}

// Handle processes a single email delivery job. It is registered with
// the jobs.Runner via runner.Register("send_notification", worker.Handle).
//
// Flow:
//  1. Deserialise payload
//  2. Load notification from store
//  3. Create state machine with database-backed accessor/mutator
//  4. Fire send trigger (pending/not_sent -> sending)
//  5. Render email template
//  6. Send via SMTP (includes 6s delay)
//  7. On success: fire delivered trigger, return nil
//  8. On @example.com: fire failed trigger, return nil (permanent)
//  9. On transient failure: increment retry_count, fire soft_fail
//     trigger, return error (goqite retries via visibility timeout)
//  10. On retry limit exhausted: fire failed trigger, return nil
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

	// Create state machine with database-backed accessor/mutator
	// (REQ-015). The accessor reads the current status from the DB;
	// the mutator writes the new status. This keeps the state machine
	// and database synchronized on every transition.
	sm := domain.ConfigureStateMachine(
		w.store.NotificationStateAccessor(n.ID),
		w.store.NotificationStateMutator(n.ID),
		n.RetryLimit,
		n.RetryCount,
	)

	// Fire send trigger: pending -> sending or not_sent -> sending.
	// The retry-limit check happens implicitly: if the notification
	// is in not_sent with exhausted retries, the guard on
	// TriggerSoftFail would have already moved it to failed on the
	// previous attempt. If it arrives here in not_sent, retries are
	// still available.
	prevStatus := n.Status
	var sendTrigger domain.Trigger
	if n.Status == domain.StatusNotSent {
		sendTrigger = domain.TriggerRetry
	} else {
		sendTrigger = domain.TriggerSend
	}
	if err := w.fireAndLog(ctx, sm, n, prevStatus, sendTrigger, domain.StatusSending); err != nil {
		return err
	}
	// Re-read status from store after state machine transition.
	n.Status = domain.StatusSending

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
		slog.Warn("email send failed",
			"error", err,
			"notification_id", n.ID,
			"retry_count", n.RetryCount,
		)

		// Check if this is a permanent failure (@example.com).
		if errors.Is(err, infraemail.ErrExampleDomain) {
			if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
				slog.Error("fire failed trigger", "error", fErr)
			}
			return nil // ack -- permanent failure, no retry
		}

		// Transient failure: fire soft_fail, increment retry.
		if smErr := sm.FireCtx(ctx, domain.TriggerSoftFail); smErr != nil {
			// Guard rejected: retry limit exhausted during this
			// attempt. Fire TriggerFailed from sending instead.
			slog.Info("soft_fail rejected by guard, firing failed",
				"notification_id", n.ID,
				"retry_count", n.RetryCount,
				"retry_limit", n.RetryLimit,
			)
			if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
				slog.Error("fire failed trigger after guard rejection", "error", fErr)
			}
			return nil // ack -- retries exhausted
		}
		// soft_fail succeeded (sending -> not_sent).
		if logErr := w.store.LogTransition(ctx, "notification", n.ID,
			domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail); logErr != nil {
			slog.Error("log transition failed", "error", logErr)
		}

		n.RetryCount++
		if updateErr := w.store.UpdateNotification(ctx, n); updateErr != nil {
			slog.Error("update notification retry_count", "error", updateErr)
		}
		return fmt.Errorf("send email: %w", err) // return error for goqite retry
	}

	// Success: fire delivered trigger.
	if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerDelivered, domain.StatusDelivered); fErr != nil {
		slog.Error("fire delivered trigger", "error", fErr)
		return fmt.Errorf("fire delivered: %w", fErr)
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
Expected: PASS (4 tests)

**Step 5: Commit**

`refactor(queue): replace ad-hoc state transitions with stateless state machine`

---

### Task 6: Update Daemon Wiring

Satisfies: wiring the `WorkerStore` (accessor, mutator, transition
logger) into the worker at the composition root.

**Depends on:** Task 5 (worker refactor)

**Files:**
- Modify: `cmd/daemon/daemon.go:111-112`

**Step 1: Verify the worker construction in daemon.go**

The existing call should already be:

```go
	worker := queue.NewEmailWorker(store, sender)
```

The SQLite `Store` already satisfies `queue.WorkerStore` (it
implements `domain.NotificationStore`, `domain.TransitionLogger`,
`NotificationStateAccessor`, and `NotificationStateMutator` -- all
added in Task 3/4). The call site does not change; the `store`
parameter now satisfies the wider `WorkerStore` interface.

**Step 2: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 3: Run all tests to verify nothing is broken**

Run: `go test ./...`
Expected: PASS (all tests)

**Step 4: Commit**

`feat(daemon): wire WorkerStore into email worker`

---

### Task 7: QA Seed Data for All States

Satisfies: overview Build Types (QA seed for delivered, failed,
not_sent states), REQ-022 (seed audit log entries).

Update the seed data to include notifications in all terminal states
so the QA build shows a complete lifecycle from first boot. Also seed
audit log entries for the pre-existing terminal-state notifications.

**Depends on:** Task 2 (audit log migration), Task 6 (daemon wiring)

**Files:**
- Modify: `internal/infra/seed/testdata/seed.sql`
- Modify: `internal/infra/seed/seed_qa.go`

**Step 1: Update seed.sql with Step 4 notification data**

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

-- Step 4: notifications in terminal/retry states.

-- Delivered notification (status=2).
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-004', 'delivered@company.com', 2, 0, 3);

-- Failed notification (status=3) -- exhausted retries.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-005', 'failed@company.com', 3, 3, 3);

-- Not_sent notification (status=4) with 1 retry used -- will auto-retry.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-006', 'retry@company.com', 4, 1, 3);

-- Audit log entries for pre-seeded terminal-state notifications.
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-004', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-004', 'sending', 'delivered', 'delivered');

INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'failed', 'failed');

INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-006', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-006', 'sending', 'not_sent', 'soft_fail');
```

**Step 2: Update seed_qa.go to enqueue a retry job for not_sent**

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
// Only pending and not_sent notifications need delivery jobs -- terminal
// states (delivered, failed) do not get jobs.
var seedJobs = []struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}{
	// Step 3: pending notifications.
	{"ntf_seed-001", "alice@company.com", "req_seed-001"},
	{"ntf_seed-002", "bob@company.com", "req_seed-002"},
	{"ntf_seed-003", "charlie@example.com", "req_seed-003"},
	// Step 4: not_sent notification (will auto-retry).
	{"ntf_seed-006", "retry@company.com", "req_seed-006"},
}

// RunSeed executes the embedded seed SQL and enqueues delivery jobs
// programmatically via jobs.Create(). Called on startup when built
// with -tags qa.
func RunSeed(db *sql.DB) error {
	// Insert notification rows and audit log entries via SQL.
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

**Step 3: Update seed_test.go schema fixture**

The existing `seed_test.go` creates an in-memory SQLite database with
only the `notifications` and `goqite` tables. The updated `seed.sql`
now inserts into `state_transitions`, so the test schema must include
that table.

Add the `state_transitions` table to the schema fixture in
`internal/infra/seed/seed_test.go`:

```sql
CREATE TABLE IF NOT EXISTS state_transitions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
```

Also update the expected assertion counts:
- Expected notification count: 6 (was 3)
- Expected goqite message count: 4 (was 3)

**Step 4: Verify QA build compiles**

Run: `go build -tags qa ./...`
Expected: exits 0

**Step 5: Run seed tests**

Run: `go test -tags qa ./internal/infra/seed/...`
Expected: PASS

**Step 6: Commit**

`feat(seed): add QA seed data for delivered, failed, and not_sent states`

---

### Task 8: Integration Test -- Full State Machine Flow via Store

Satisfies: REQ-003 through REQ-012 (all permitted and rejected
transitions verified against real SQLite), REQ-022/REQ-023 (audit
log verified with real rows).

This test verifies the state machine, accessor, mutator, and audit log
work together end-to-end against a real SQLite database (not mocks).

**Depends on:** Task 3 (store methods), Task 1 (state machine config)

**Files:**
- Create: `internal/infra/sqlite/statemachine_integration_test.go`

**Step 1: Write the integration test**

```go
package sqlite

import (
	"context"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestStateMachineIntegrationDeliveryPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-1",
		Email:      "sm-int@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Build the state machine with real accessor/mutator.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit,
		n.RetryCount,
	)

	// pending -> sending
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// sending -> delivered
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Verify final state in database.
	got, err := store.GetNotificationByEmail(ctx, "sm-int@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log has 2 entries.
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}

func TestStateMachineIntegrationRetryPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-2",
		Email:      "sm-retry@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// pending -> sending
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend)

	// sending -> not_sent (soft fail)
	if err := sm.FireCtx(ctx, domain.TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail)

	// not_sent -> sending (retry) -- need new SM with updated state.
	n.RetryCount = 1
	sm = domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerRetry); err != nil {
		t.Fatalf("Fire(TriggerRetry): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry)

	// sending -> delivered
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered)

	// Verify 4 audit log entries (pending->sending, sending->not_sent,
	// not_sent->sending, sending->delivered).
	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("audit log entries = %d, want 4", count)
	}
}

func TestStateMachineIntegrationRejectedTransition(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-int-3",
		Email:      "sm-reject@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)

	// pending -> failed should be rejected (REQ-009).
	err = sm.FireCtx(ctx, domain.TriggerFailed)
	if err == nil {
		t.Fatal("expected error for pending -> failed, got nil")
	}

	// Verify state unchanged in database.
	got, err := store.GetNotificationByEmail(ctx, "sm-reject@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending (unchanged)", got.Status)
	}
}
```

**Step 2: Run the integration tests**

Run: `go test -run "TestStateMachineIntegration" ./internal/infra/sqlite/...`
Expected: PASS (3 tests)

**Step 3: Commit**

`test(sqlite): add state machine integration tests with real database`

---

### Task 9: Full Build Verification

Verifies all changes work together: dev build, QA build, all tests.

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS (all packages)

**Step 2: Run QA build test suite**

Run: `go test -tags qa ./...`
Expected: PASS (all packages, including seed tests)

**Step 3: Build all variants**

Run: `go build ./...`
Expected: exits 0

Run: `go build -tags qa ./...`
Expected: exits 0

**Step 4: Run linter**

Run: `mise run lint:go`
Expected: no warnings

**Step 5: Commit (if any fixups needed)**

`chore: fix lint/build issues from step 4`

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings
- [ ] `go build -tags qa ./...` succeeds with no warnings
- [ ] `go test ./...` passes (all packages)
- [ ] `go test -tags qa ./...` passes (all packages)
- [ ] `mise run lint:go` produces no warnings
- [ ] State machine rejects `pending -> failed` (REQ-009)
- [ ] State machine rejects `pending -> delivered` (REQ-010)
- [ ] State machine rejects `delivered -> sending` (REQ-011)
- [ ] State machine rejects `failed -> sending` (REQ-012)
- [ ] `sending -> not_sent` blocked when retry_count >= retry_limit (REQ-020)
- [ ] Every state transition logs to `state_transitions` table (REQ-022)
- [ ] Audit log entries contain entity_type, entity_id, from_state, to_state, trigger, created_at (REQ-023)
- [ ] QA seed includes delivered, failed, and not_sent notifications
- [ ] QA seed includes audit log entries for terminal-state notifications
- [ ] Worker no longer directly assigns `n.Status =` -- all transitions go through state machine
- [ ] Worker uses store's database-backed accessor/mutator, not in-memory closures (REQ-015)
- [ ] Retry-limit-exceeded path fires through the state machine (not_sent -> sending -> failed), not a direct status bypass
- [ ] `@example.com` path: pending -> sending -> failed (REQ-005, never not_sent)
- [ ] `/v1/notify` POST response includes `retry_count` and `retry_limit` fields (REQ-021 partial; dashboard deferred to Step 5)
- [ ] `seed_test.go` schema fixture includes `state_transitions` table
