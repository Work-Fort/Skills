package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// resetRequest is the JSON body for POST /v1/notify/reset.
type resetRequest struct {
	Email string `json:"email"`
}

// HandleReset returns an http.HandlerFunc for POST /v1/notify/reset.
// It looks up the notification by email, transitions it back to pending
// via the state machine (TriggerReset, satisfying REQ-002), clears the
// retry count and timestamps, logs the transition, and returns 204 No
// Content.
func HandleReset(store domain.ResetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit (REQ-018)

		var req resetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		// Look up the notification by email.
		n, err := store.GetNotificationByEmail(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "not found",
				})
				return
			}
			slog.Error("get notification for reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-002: Transition to pending via the state machine. This
		// validates that TriggerReset is permitted from the current state
		// and updates the status through the mutator.
		prevStatus := n.Status
		sm := domain.ConfigureStateMachine(
			store.NotificationStateAccessor(n.ID),
			store.NotificationStateMutator(n.ID),
			n.RetryLimit,
			n.RetryCount,
		)
		if err := sm.FireCtx(r.Context(), domain.TriggerReset); err != nil {
			slog.Error("state machine reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// Log the transition for the audit trail.
		if err := store.LogTransition(r.Context(), "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset); err != nil {
			slog.Error("log reset transition failed", "error", err)
			// Non-fatal: the reset itself succeeded.
		}

		// Clear retry count and timestamps (except created_at).
		// REQ-004: clear retry_count. REQ-005: clear delivery results
		// (status already reset by state machine; retry_count is the
		// remaining delivery result field). REQ-006: reset timestamps.
		n.RetryCount = 0
		n.UpdatedAt = time.Time{}

		if err := store.UpdateNotification(r.Context(), n); err != nil {
			slog.Error("update notification for reset failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-007a: 204 No Content with empty body.
		w.WriteHeader(http.StatusNoContent)
	}
}
