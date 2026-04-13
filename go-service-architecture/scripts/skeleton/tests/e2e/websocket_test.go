package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestWebSocketBroadcast verifies that connecting a WebSocket client
// to /v1/ws and then sending POST /v1/notify produces broadcast
// messages for each state transition (at minimum "sending" and
// "delivered"). Satisfies notification-realtime REQ-024, REQ-025.
func TestWebSocketBroadcast(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)
	wsURL := fmt.Sprintf("ws://%s/v1/ws", addr)

	// Step 1: Connect WebSocket client.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.CloseNow()

	// Step 2: Send POST /v1/notify.
	email := "ws-e2e@company.com"
	body := fmt.Sprintf(`{"email": %q}`, email)
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var notifyResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&notifyResp)
	notificationID := notifyResp.ID
	if notificationID == "" {
		t.Fatal("notification ID is empty")
	}

	// Step 3: Read WebSocket messages until we see both "sending"
	// and "delivered", or timeout.
	type wsMsg struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}

	seenStates := make(map[string]bool)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			// Timeout reading -- continue polling until outer deadline.
			continue
		}

		var msg wsMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Errorf("invalid JSON from WebSocket: %s", data)
			continue
		}

		// REQ-025: verify message contains id and state.
		if msg.ID == "" {
			t.Errorf("WebSocket message missing 'id': %s", data)
		}
		if msg.State == "" {
			t.Errorf("WebSocket message missing 'state': %s", data)
		}

		// Only track messages for our notification.
		if msg.ID == notificationID {
			t.Logf("received WS: id=%s state=%s", msg.ID, msg.State)
			seenStates[msg.State] = true
		}

		if seenStates["sending"] && seenStates["delivered"] {
			break
		}
	}

	// REQ-024: verify we saw at minimum "sending" and "delivered".
	if !seenStates["sending"] {
		t.Error("did not receive WebSocket broadcast for 'sending' state")
	}
	if !seenStates["delivered"] {
		t.Error("did not receive WebSocket broadcast for 'delivered' state")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}
