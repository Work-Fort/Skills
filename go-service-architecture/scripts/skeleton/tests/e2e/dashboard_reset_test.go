package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDashboardResetDelivered simulates the dashboard flow:
// 1. POST /v1/notify to create a notification
// 2. Poll until delivered
// 3. POST /v1/notify/reset (the same call the dashboard Reset button makes)
// 4. Poll until re-delivered
// 5. Verify the notification went through the full cycle
func TestDashboardResetDelivered(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)
	email := "dashboard-reset-e2e@company.com"

	// Step 1: Send notification.
	resp, err := http.Post(
		base+"/v1/notify",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"email": %q}`, email)),
	)
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Step 2: Wait for delivery.
	state := pollNotificationState(t, base, email, "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	// Step 3: Verify initial email arrived.
	mailCount := pollMailpitCount(t, mailpitAPI, 1, 5*time.Second)
	if mailCount < 1 {
		t.Fatal("no email received in Mailpit before reset")
	}

	// Step 4: Reset via the same endpoint the dashboard calls.
	resp, err = http.Post(
		base+"/v1/notify/reset",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"email": %q}`, email)),
	)
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d", resp.StatusCode)
	}

	// Step 5: Verify the notification returns to delivered after re-processing.
	state = pollNotificationState(t, base, email, "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("after reset: expected re-delivery to delivered, got %q", state)
	}

	// Step 6: Verify a second email arrived.
	mailCount = pollMailpitCount(t, mailpitAPI, 2, 10*time.Second)
	if mailCount < 2 {
		t.Errorf("expected at least 2 emails after reset, got %d", mailCount)
	}

	// Step 7: Verify the notification is visible in the list with delivered state
	// and retry_count reset to 0.
	resp, err = http.Get(base + "/v1/notifications?limit=100")
	if err != nil {
		t.Fatalf("GET /v1/notifications: %v", err)
	}
	var listResp struct {
		Notifications []struct {
			Email      string `json:"email"`
			State      string `json:"state"`
			RetryCount int    `json:"retry_count"`
		} `json:"notifications"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	found := false
	for _, n := range listResp.Notifications {
		if n.Email == email {
			found = true
			if n.State != "delivered" {
				t.Errorf("state = %q, want delivered", n.State)
			}
		}
	}
	if !found {
		t.Fatal("notification not found in list response after reset")
	}
}
