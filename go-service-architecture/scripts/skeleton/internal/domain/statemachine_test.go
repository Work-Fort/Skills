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
