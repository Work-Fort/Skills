package mcp

import (
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/workfort/notifier/internal/domain"
)

// MCPStore combines the store interfaces needed by all MCP tools.
// The composition root's Store satisfies this.
type MCPStore interface {
	domain.ResetStore
	domain.HealthChecker
}

// NewMCPHandler creates a StreamableHTTPServer with all MCP tools
// registered. The store must implement all required port interfaces
// (NotificationStore, HealthChecker, TransitionLogger, and the state
// machine accessor/mutator methods). The enqueuer provides job
// queueing. version is the service version string.
//
// Returns the StreamableHTTPServer as an http.Handler. The caller
// must also retain the *server.StreamableHTTPServer to call Shutdown
// during graceful shutdown (REQ-013).
func NewMCPHandler(store MCPStore, enqueuer domain.Enqueuer, version string) *server.StreamableHTTPServer {
	s := server.NewMCPServer("notifier", version)

	s.AddTool(
		gomcp.NewTool("send_notification",
			gomcp.WithDescription("Send a notification email to the given address. Creates a notification record and enqueues an email delivery job."),
			gomcp.WithString("email",
				gomcp.Required(),
				gomcp.Description("Email address to notify"),
			),
		),
		HandleSendNotification(store, enqueuer),
	)

	s.AddTool(
		gomcp.NewTool("reset_notification",
			gomcp.WithDescription("Reset a notification record so the email address can be notified again. Transitions the notification back to pending and clears the retry count."),
			gomcp.WithString("email",
				gomcp.Required(),
				gomcp.Description("Email address of the notification to reset"),
			),
		),
		HandleResetNotification(store, enqueuer),
	)

	s.AddTool(
		gomcp.NewTool("list_notifications",
			gomcp.WithDescription("List all notification records with their current state, retry count, and timestamps."),
			gomcp.WithString("after",
				gomcp.Description("Cursor for pagination (notification ID to start after)"),
			),
			gomcp.WithNumber("limit",
				gomcp.Description("Maximum number of results to return (default 20, max 100)"),
			),
		),
		HandleListNotifications(store),
	)

	s.AddTool(
		gomcp.NewTool("check_health",
			gomcp.WithDescription("Check the health of the notification service by pinging the database."),
		),
		HandleCheckHealth(store),
	)

	return server.NewStreamableHTTPServer(s)
}
