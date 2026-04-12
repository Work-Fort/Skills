package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

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
func HandleNotify(store domain.NotificationStore, enqueuer domain.Enqueuer) http.HandlerFunc {
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
