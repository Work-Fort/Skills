package httpapi

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/workfort/notifier/internal/domain"
)

const defaultPageLimit = 20
const maxPageLimit = 100

// listNotificationItem is the JSON representation of a notification
// in the list response.
type listNotificationItem struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	State      string `json:"state"`
	RetryCount int    `json:"retry_count"`
	RetryLimit int    `json:"retry_limit"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// listMeta is the pagination metadata in the list response.
type listMeta struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// listResponse is the JSON response for GET /v1/notifications.
type listResponse struct {
	Notifications []listNotificationItem `json:"notifications"`
	Meta          listMeta               `json:"meta"`
}

// HandleList returns an http.HandlerFunc for GET /v1/notifications.
// It supports cursor-based pagination via `after` (base64-encoded ID)
// and `limit` query parameters.
func HandleList(store domain.NotificationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse pagination parameters.
		limit := defaultPageLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > maxPageLimit {
			limit = maxPageLimit
		}

		// Decode cursor: the `after` param is a base64-encoded
		// notification ID.
		var afterID string
		if v := r.URL.Query().Get("after"); v != "" {
			decoded, err := base64.StdEncoding.DecodeString(v)
			if err == nil {
				afterID = string(decoded)
			}
		}

		// Fetch limit+1 to determine if there are more results.
		notifications, err := store.ListNotifications(r.Context(), afterID, limit+1)
		if err != nil {
			slog.Error("list notifications failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		hasMore := len(notifications) > limit
		if hasMore {
			notifications = notifications[:limit]
		}

		// Build response items.
		items := make([]listNotificationItem, 0, len(notifications))
		for _, n := range notifications {
			items = append(items, listNotificationItem{
				ID:         n.ID,
				Email:      n.Email,
				State:      n.Status.String(),
				RetryCount: n.RetryCount,
				RetryLimit: n.RetryLimit,
				CreatedAt:  n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt:  n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		// Build pagination metadata.
		meta := listMeta{HasMore: hasMore}
		if hasMore && len(notifications) > 0 {
			lastID := notifications[len(notifications)-1].ID
			meta.NextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
		}

		writeJSON(w, http.StatusOK, listResponse{
			Notifications: items,
			Meta:          meta,
		})
	}
}
