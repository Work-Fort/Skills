package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/config"
)

func TestDaemonHealthEndpoint(t *testing.T) {
	// Set up temp XDG dirs.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := config.InitDirs(); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(); err != nil {
		t.Fatal(err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	dbPath := filepath.Join(tmp, "state", "notifier", "test.db")

	// Use a cancellable context to trigger graceful shutdown instead
	// of sending SIGINT to the process (which would interfere with
	// the test runner if other tests run concurrently).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, ServerConfig{
			Bind:     "127.0.0.1",
			Port:     port,
			DSN:      dbPath,
			SMTPHost: "127.0.0.1",
			SMTPPort: 1025,
			SMTPFrom: "test@localhost",
		})
	}()

	// Wait for server to be ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	ready := false
	for i := 0; i < 50; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("RunServer returned early with error: %v", err)
		default:
		}
		resp, err := http.Get(addr + "/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready within 5 seconds")
	}

	// Test health endpoint.
	resp, err := http.Get(addr + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify X-Request-ID header is set.
	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("X-Request-ID header missing")
	}

	// Verify response body.
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want %q", body["status"], "healthy")
	}

	// Cancel the context to trigger graceful shutdown.
	cancel()

	// Wait for server to stop.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down within 10 seconds")
	}
}
