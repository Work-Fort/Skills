package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// HandleSendNotification returns an MCP tool handler for
// send_notification. It calls the same domain logic as POST /v1/notify
// (REQ-004).
func HandleSendNotification(store domain.NotificationStore, enqueuer domain.Enqueuer) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		email := req.GetString("email", "")
		if email == "" {
			return gomcp.NewToolResultError("email is required"), nil
		}

		if err := domain.ValidateEmail(email); err != nil {
			return gomcp.NewToolResultError(err.Error()), nil
		}

		id := domain.NewID("ntf")
		n := &domain.Notification{
			ID:         id,
			Email:      email,
			Status:     domain.StatusPending,
			RetryCount: 0,
			RetryLimit: domain.DefaultRetryLimit,
		}

		if err := store.CreateNotification(ctx, n); err != nil {
			if errors.Is(err, domain.ErrAlreadyNotified) {
				return gomcp.NewToolResultError("already notified"), nil
			}
			return gomcp.NewToolResultError("internal error: " + err.Error()), nil
		}

		jobPayload := queue.EmailJobPayload{
			NotificationID: id,
			Email:          email,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(ctx, payload); err != nil {
			return gomcp.NewToolResultError("failed to enqueue: " + err.Error()), nil
		}

		result, _ := json.Marshal(map[string]string{"id": id})
		return gomcp.NewToolResultText(string(result)), nil
	}
}

// HandleResetNotification returns an MCP tool handler for
// reset_notification. It calls the same domain logic as
// POST /v1/notify/reset (REQ-005).
func HandleResetNotification(store domain.ResetStore) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		email := req.GetString("email", "")
		if email == "" {
			return gomcp.NewToolResultError("email is required"), nil
		}

		n, err := store.GetNotificationByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return gomcp.NewToolResultError("not found"), nil
			}
			return gomcp.NewToolResultError("internal error: " + err.Error()), nil
		}

		prevStatus := n.Status
		sm := domain.ConfigureStateMachine(
			store.NotificationStateAccessor(n.ID),
			store.NotificationStateMutator(n.ID),
			n.RetryLimit,
			n.RetryCount,
		)
		if err := sm.FireCtx(ctx, domain.TriggerReset); err != nil {
			return gomcp.NewToolResultError("reset failed: " + err.Error()), nil
		}

		//nolint:errcheck // Log failure is non-fatal; reset already succeeded.
		_ = store.LogTransition(ctx, "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset)

		n.RetryCount = 0
		n.UpdatedAt = time.Time{}
		if err := store.UpdateNotification(ctx, n); err != nil {
			return gomcp.NewToolResultError("update failed: " + err.Error()), nil
		}

		return gomcp.NewToolResultText("notification reset"), nil
	}
}

// HandleListNotifications returns an MCP tool handler for
// list_notifications. It calls the same domain logic as
// GET /v1/notifications (REQ-006).
func HandleListNotifications(store domain.NotificationStore) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		after := req.GetString("after", "")
		limit := req.GetInt("limit", 20)
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		notifications, err := store.ListNotifications(ctx, after, limit)
		if err != nil {
			return gomcp.NewToolResultError("list failed: " + err.Error()), nil
		}

		type item struct {
			ID         string `json:"id"`
			Email      string `json:"email"`
			State      string `json:"state"`
			RetryCount int    `json:"retry_count"`
			RetryLimit int    `json:"retry_limit"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
		}
		items := make([]item, 0, len(notifications))
		for _, n := range notifications {
			items = append(items, item{
				ID:         n.ID,
				Email:      n.Email,
				State:      n.Status.String(),
				RetryCount: n.RetryCount,
				RetryLimit: n.RetryLimit,
				CreatedAt:  n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt:  n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		result, _ := json.Marshal(map[string]any{
			"notifications": items,
		})
		return gomcp.NewToolResultText(string(result)), nil
	}
}

// HandleCheckHealth returns an MCP tool handler for check_health.
// It calls the same domain logic as GET /v1/health (REQ-007).
func HandleCheckHealth(checker domain.HealthChecker) server.ToolHandlerFunc {
	return func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		err := checker.Ping(ctx)
		status := "healthy"
		if err != nil {
			status = "unhealthy"
		}
		result, _ := json.Marshal(map[string]string{"status": status})
		return gomcp.NewToolResultText(string(result)), nil
	}
}
