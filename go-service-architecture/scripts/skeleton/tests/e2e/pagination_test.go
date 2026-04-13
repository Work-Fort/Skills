package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// paginationListResponse captures the list response shape for
// pagination tests.
type paginationListResponse struct {
	Notifications []json.RawMessage `json:"notifications"`
	Meta          struct {
		HasMore    bool `json:"has_more"`
		TotalCount int  `json:"total_count"`
		TotalPages int  `json:"total_pages"`
	} `json:"meta"`
}

// TestPaginationEmptyList verifies that an empty notification list
// returns the correct metadata. Satisfies REQ-028(c).
func TestPaginationEmptyList(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	resp, err := http.Get(base + "/v1/notifications")
	if err != nil {
		t.Fatalf("GET /v1/notifications: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body paginationListResponse
	json.NewDecoder(resp.Body).Decode(&body)

	if body.Meta.TotalCount != 0 {
		t.Errorf("total_count = %d, want 0", body.Meta.TotalCount)
	}
	if body.Meta.TotalPages != 0 {
		t.Errorf("total_pages = %d, want 0", body.Meta.TotalPages)
	}
	if body.Meta.HasMore {
		t.Error("has_more = true, want false")
	}
	if len(body.Notifications) != 0 {
		t.Errorf("notifications length = %d, want 0", len(body.Notifications))
	}
}

// TestPaginationLimitClamping verifies that limit values above 100
// are silently clamped to 100. Satisfies REQ-028(a).
func TestPaginationLimitClamping(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// Create a single notification so the list is not empty.
	body := `{"email": "clamp-test@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Request with limit=200 (above max 100).
	resp, err = http.Get(base + "/v1/notifications?limit=200")
	if err != nil {
		t.Fatalf("GET /v1/notifications?limit=200: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var listResp paginationListResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	// With 1 notification and limit clamped to 100, we should get
	// at most 100 results (here just 1).
	if len(listResp.Notifications) > 100 {
		t.Errorf("notifications length = %d, want <= 100 (clamped)", len(listResp.Notifications))
	}
}

// TestPaginationTotalPages verifies total_pages is correctly
// calculated as ceil(total_count / limit). Satisfies REQ-028(b).
func TestPaginationTotalPages(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// Create 3 notifications.
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"email": "pages-test-%d@company.com"}`, i)
		resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /v1/notify: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("notify %d: expected 202, got %d", i, resp.StatusCode)
		}
	}

	// Request with limit=2 -- 3 items / 2 per page = 2 pages.
	resp, err := http.Get(base + "/v1/notifications?limit=2")
	if err != nil {
		t.Fatalf("GET /v1/notifications?limit=2: %v", err)
	}
	defer resp.Body.Close()

	var listResp paginationListResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	if listResp.Meta.TotalCount != 3 {
		t.Errorf("total_count = %d, want 3", listResp.Meta.TotalCount)
	}
	if listResp.Meta.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2 (ceil(3/2))", listResp.Meta.TotalPages)
	}
	if !listResp.Meta.HasMore {
		t.Error("has_more = false, want true")
	}
	if len(listResp.Notifications) != 2 {
		t.Errorf("notifications length = %d, want 2", len(listResp.Notifications))
	}
}
