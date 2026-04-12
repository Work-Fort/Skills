package mcpbridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridgeForwardsRequest(t *testing.T) {
	// Create a fake MCP server that echoes back a JSON-RPC response.
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request is a POST with JSON content.
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}

		// Verify auth token is passed.
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("authorization = %q, want 'Bearer test-token'", auth)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		// Return a JSON-RPC response.
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]string{"status": "ok"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeMCP.Close()

	// Simulate stdin with a JSON-RPC request.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_health"}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	err := Bridge(stdin, &stdout, fakeMCP.URL, "test-token")
	if err != nil {
		t.Fatalf("Bridge error: %v", err)
	}

	// Verify stdout contains the response.
	output := strings.TrimSpace(stdout.String())
	if !strings.Contains(output, "jsonrpc") {
		t.Errorf("stdout = %q, want JSON-RPC response", output)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["id"] != float64(1) {
		t.Errorf("id = %v, want 1", resp["id"])
	}
}

func TestBridgeHandlesMultipleMessages(t *testing.T) {
	var receivedCount int
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]string{"status": "ok"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeMCP.Close()

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"check_health"}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	err := Bridge(stdin, &stdout, fakeMCP.URL, "")
	if err != nil {
		t.Fatalf("Bridge error: %v", err)
	}

	if receivedCount != 2 {
		t.Errorf("received = %d, want 2", receivedCount)
	}

	// Verify two responses were written.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("response lines = %d, want 2", len(lines))
	}
}
