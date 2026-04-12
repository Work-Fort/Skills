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
