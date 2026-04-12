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

// Spec: notification-delivery, Scenario: Successful notification
func TestNotifySuccess(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Delete all Mailpit messages to start clean.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// WHEN a POST request is sent to /v1/notify
	body := `{"email": "e2e-test@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()

	// THEN the system SHALL return HTTP 202
	if resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, respBody)
	}

	// AND the response body SHALL contain an id matching ntf_<uuid>
	var notifyResp map[string]string
	json.NewDecoder(resp.Body).Decode(&notifyResp)
	if !strings.HasPrefix(notifyResp["id"], "ntf_") {
		t.Errorf("id = %q, want ntf_ prefix", notifyResp["id"])
	}

	// AND the X-Request-ID header SHALL be present
	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Error("X-Request-ID header missing from response")
	}

	// Wait for the background worker to process the job (6s delay + processing).
	t.Log("waiting for email delivery (6s delay + processing)...")
	var mailpitMessages struct {
		Messages []struct {
			ID      string `json:"ID"`
			Subject string `json:"Subject"`
		} `json:"messages"`
		Total int `json:"total"`
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		json.NewDecoder(resp.Body).Decode(&mailpitMessages)
		resp.Body.Close()
		if mailpitMessages.Total > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if mailpitMessages.Total == 0 {
		t.Fatal("no email received in Mailpit within timeout")
	}

	// Verify the email was delivered to the correct address.
	msgID := mailpitMessages.Messages[0].ID
	msgResp, err := http.Get(fmt.Sprintf("%s/api/v1/message/%s", mailpitAPI, msgID))
	if err != nil {
		t.Fatalf("get mailpit message: %v", err)
	}
	defer msgResp.Body.Close()

	var msgDetail struct {
		To []struct {
			Address string `json:"Address"`
		} `json:"To"`
		Subject string `json:"Subject"`
		HTML    string `json:"HTML"`
		Text    string `json:"Text"`
	}
	json.NewDecoder(msgResp.Body).Decode(&msgDetail)

	if len(msgDetail.To) == 0 || msgDetail.To[0].Address != "e2e-test@company.com" {
		t.Errorf("email To = %v, want e2e-test@company.com", msgDetail.To)
	}
	if !strings.Contains(msgDetail.HTML, "e2e-test@company.com") {
		t.Error("HTML body does not contain recipient email")
	}
	if !strings.Contains(msgDetail.Text, "e2e-test@company.com") {
		t.Error("plaintext body does not contain recipient email")
	}
}

// Spec: notification-delivery, Scenario: Duplicate notification rejected
func TestNotifyDuplicate(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// First request succeeds.
	body := `{"email": "duplicate-test@company.com"}`
	resp, _ := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("first POST: expected 202, got %d", resp.StatusCode)
	}

	// Second request for the same email returns 409.
	resp, _ = http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second POST: expected 409, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "already notified") {
		t.Errorf("error = %q, want 'already notified'", errResp["error"])
	}
}

// Spec: notification-delivery, Scenario: Invalid email rejected
func TestNotifyInvalidEmail(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	body := `{"email": "not-an-email"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

// Spec: notification-delivery, Scenario: Email to example.com auto-fails
func TestNotifyExampleComFails(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// POST succeeds (returns 202) because the failure happens async.
	body := `{"email": "test@example.com"}`
	resp, _ := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for the worker to process and fail the job.
	// The @example.com check happens before the 6s delay, so this
	// should be fast.
	time.Sleep(3 * time.Second)

	// The health endpoint should still work (service is not crashed).
	resp, err := http.Get(fmt.Sprintf("http://%s/v1/health", addr))
	if err != nil {
		t.Fatalf("health check after example.com failure: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: expected 200, got %d", resp.StatusCode)
	}
}
