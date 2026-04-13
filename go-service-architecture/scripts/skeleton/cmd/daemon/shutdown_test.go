package daemon

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/workfort/notifier/internal/config"
)

func TestGracefulShutdownWaitsForRunner(t *testing.T) {
	// Set up temp XDG dirs — same pattern as TestDaemonHealthEndpoint.
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := ServerConfig{
		Bind:            "127.0.0.1",
		Port:            port,
		DSN:             dbPath,
		SMTPHost:        "127.0.0.1",
		SMTPPort:        1025,
		SMTPFrom:        "test@localhost",
		Version:         "test",
		ShutdownTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, cfg)
	}()

	// Give the server a moment to start.
	time.Sleep(200 * time.Millisecond)

	// Cancel the context to trigger shutdown.
	cancel()

	// Wait for RunServer to return.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunServer returned error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("RunServer did not return within 20 seconds after cancel")
	}
}
