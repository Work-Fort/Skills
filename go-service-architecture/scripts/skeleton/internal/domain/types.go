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
