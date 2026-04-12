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
