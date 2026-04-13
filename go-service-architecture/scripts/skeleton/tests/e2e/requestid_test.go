package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRequestIDInEmail verifies that the X-Request-ID from the HTTP
// response is propagated into the email headers in Mailpit. Satisfies
// notification-delivery REQ-031.
func TestRequestIDInEmail(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send POST /v1/notify and capture X-Request-ID.
	body := `{"email": "reqid-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	httpRequestID := resp.Header.Get("X-Request-ID")
	if httpRequestID == "" {
		t.Fatal("X-Request-ID header missing from HTTP response")
	}
	t.Logf("HTTP X-Request-ID: %s", httpRequestID)

	// Step 2: Wait for email delivery to Mailpit.
	var mailpitMessages struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
		Total int `json:"total"`
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		json.NewDecoder(r.Body).Decode(&mailpitMessages)
		r.Body.Close()
		if mailpitMessages.Total > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if mailpitMessages.Total == 0 {
		t.Fatal("no email received in Mailpit within timeout")
	}

	// Step 3: Retrieve the email from Mailpit and check headers.
	msgID := mailpitMessages.Messages[0].ID
	msgResp, err := http.Get(fmt.Sprintf("%s/api/v1/message/%s/headers", mailpitAPI, msgID))
	if err != nil {
		t.Fatalf("get mailpit headers: %v", err)
	}
	defer msgResp.Body.Close()

	// Mailpit returns headers as a map of string -> []string.
	var headers map[string][]string
	if err := json.NewDecoder(msgResp.Body).Decode(&headers); err != nil {
		t.Fatalf("decode headers: %v", err)
	}

	// Step 4: Verify X-Request-ID in email matches HTTP response.
	emailReqIDs, ok := headers["X-Request-Id"]
	if !ok {
		// Try case variations -- Mailpit may normalise header names.
		emailReqIDs, ok = headers["X-Request-ID"]
	}
	if !ok {
		t.Fatalf("X-Request-ID header not found in email headers: %v", headers)
	}

	found := false
	for _, v := range emailReqIDs {
		if v == httpRequestID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("email X-Request-ID = %v, want %q", emailReqIDs, httpRequestID)
	}
}
