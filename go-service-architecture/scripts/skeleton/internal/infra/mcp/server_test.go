package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMCPHandlerRespondsToPost(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}

	handler := NewMCPHandler(store, enqueuer, "test")

	// Send an MCP initialize request to verify the handler is wired.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// The MCP server should respond with 200 and a JSON-RPC response.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, string(respBody))
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "jsonrpc") {
		t.Errorf("response does not contain jsonrpc: %s", string(respBody))
	}
}

func TestNewMCPHandlerWithStripPrefix(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}

	mcpHandler := NewMCPHandler(store, enqueuer, "test")

	// Mount with StripPrefix as it will be in production.
	mux := http.NewServeMux()
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		respBody, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, string(respBody))
	}
}
