package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// pollNotificationState polls GET /v1/notifications until the
// notification with the given email reaches the target state, or the
// timeout expires. Returns the final observed state.
func pollNotificationState(t *testing.T, base, email, targetState string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastState string
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v1/notifications?limit=100")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var listResp struct {
			Notifications []struct {
				Email string `json:"email"`
				State string `json:"state"`
			} `json:"notifications"`
		}
		json.NewDecoder(resp.Body).Decode(&listResp)
		resp.Body.Close()

		for _, n := range listResp.Notifications {
			if n.Email == email {
				lastState = n.State
				if n.State == targetState {
					return n.State
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastState
}

// pollMailpitCount polls Mailpit until at least targetCount messages
// exist, or the timeout expires. Returns the final count.
func pollMailpitCount(t *testing.T, mailpitAPI string, targetCount int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var count int
	for time.Now().Before(deadline) {
		resp, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var msgs struct {
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&msgs)
		resp.Body.Close()
		count = msgs.Total
		if count >= targetCount {
			return count
		}
		time.Sleep(500 * time.Millisecond)
	}
	return count
}

// TestResetAndRedeliver verifies the full cycle: notify -> deliver ->
// reset -> re-deliver. This is the primary E2E test for the reset bug
// fix.
func TestResetAndRedeliver(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send notification.
	body := `{"email": "reset-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Step 2: Wait for delivery.
	state := pollNotificationState(t, base, "reset-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	mailCount := pollMailpitCount(t, mailpitAPI, 1, 5*time.Second)
	if mailCount < 1 {
		t.Fatal("no email received in Mailpit before reset")
	}

	// Step 3: Reset.
	resetBody := `{"email": "reset-e2e@company.com"}`
	resp, err = http.Post(base+"/v1/notify/reset", "application/json", strings.NewReader(resetBody))
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d: %s", resp.StatusCode, respBody)
	}

	// Step 4: Verify state went back to pending (briefly).
	// Poll for delivered again -- the re-delivery should happen.
	state = pollNotificationState(t, base, "reset-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("after reset: expected re-delivery to delivered, got %q", state)
	}

	// Step 5: Verify a second email arrived in Mailpit.
	mailCount = pollMailpitCount(t, mailpitAPI, 2, 10*time.Second)
	if mailCount < 2 {
		t.Errorf("expected at least 2 emails in Mailpit after re-delivery, got %d", mailCount)
	}
}

// TestResetRetryCount verifies that retry_count is reset to 0 after a
// reset, observable through the list endpoint.
func TestResetRetryCount(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send and wait for delivery.
	body := `{"email": "retry-count-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()

	state := pollNotificationState(t, base, "retry-count-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	// Step 2: Reset.
	resetBody := `{"email": "retry-count-e2e@company.com"}`
	resp, err = http.Post(base+"/v1/notify/reset", "application/json", strings.NewReader(resetBody))
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d", resp.StatusCode)
	}

	// Step 3: Immediately check that retry_count is 0.
	// The reset handler clears retry_count synchronously before
	// returning 204, so it should be observable right away.
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
		if n.Email == "retry-count-e2e@company.com" {
			found = true
			if n.RetryCount != 0 {
				t.Errorf("retry_count = %d, want 0 after reset", n.RetryCount)
			}
			// State should be pending (or sending if worker is fast).
			if n.State != "pending" && n.State != "sending" && n.State != "delivered" {
				t.Errorf("state = %q, want pending/sending/delivered", n.State)
			}
		}
	}
	if !found {
		t.Fatal("notification not found in list response")
	}
}
