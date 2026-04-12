package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSPADevProxy_ForwardsRequest(t *testing.T) {
	// Start a fake Vite dev server.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vite-dev-server"))
	}))
	defer backend.Close()

	proxy := NewSPADevProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "vite-dev-server" {
		t.Fatalf("expected proxied content, got %q", body)
	}
}

func TestNewSPADevProxy_PreservesPath(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewSPADevProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/src/main.tsx", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if gotPath != "/src/main.tsx" {
		t.Fatalf("expected path /src/main.tsx, got %q", gotPath)
	}
}

func TestNewSPADevProxy_PanicsOnInvalidURL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid URL, got none")
		}
	}()
	NewSPADevProxy("://not-a-url")
}
