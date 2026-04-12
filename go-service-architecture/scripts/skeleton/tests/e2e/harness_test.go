package e2e_test

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type Daemon struct {
	cmd    *exec.Cmd
	addr   string
	dir    string
	stderr *bytes.Buffer
}

type daemonConfig struct {
	smtpHost string
	smtpPort string
}

func defaultDaemonConfig() daemonConfig {
	return daemonConfig{
		smtpHost: "127.0.0.1",
		smtpPort: "1025",
	}
}

type DaemonOption func(*daemonConfig)

func WithSMTP(host, port string) DaemonOption {
	return func(c *daemonConfig) {
		c.smtpHost = host
		c.smtpPort = port
	}
}

func StartDaemon(t *testing.T, bin, addr string, opts ...DaemonOption) *Daemon {
	t.Helper()

	cfg := defaultDaemonConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	_, port, _ := net.SplitHostPort(addr)
	dir, err := os.MkdirTemp("", "e2e-daemon-*")
	if err != nil {
		t.Fatal(err)
	}

	d := &Daemon{
		addr:   addr,
		dir:    dir,
		stderr: &bytes.Buffer{},
	}

	d.cmd = exec.Command(bin, "daemon",
		"--bind", "127.0.0.1",
		"--port", port,
		"--smtp-host", cfg.smtpHost,
		"--smtp-port", cfg.smtpPort,
	)
	d.cmd.Stderr = d.stderr
	d.cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		"XDG_STATE_HOME="+filepath.Join(dir, "state"),
	)

	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Poll until the daemon accepts TCP connections.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}

	d.Stop()
	t.Fatalf("daemon did not become ready at %s: %s", addr, d.stderr.String())
	return nil
}

func (d *Daemon) Stop() {
	if d.cmd.Process != nil {
		_ = d.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- d.cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			_ = d.cmd.Process.Kill()
		case <-done:
		}
	}
	os.RemoveAll(d.dir)
}

func (d *Daemon) StopFatal(t *testing.T) {
	t.Helper()
	d.Stop()
	if strings.Contains(d.stderr.String(), "DATA RACE") {
		t.Fatalf("data race detected:\n%s", d.stderr.String())
	}
}

func FreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// MailpitAddr returns the Mailpit SMTP and API addresses from env
// vars, falling back to Mailpit defaults.
func MailpitAddr() (smtpHost string, smtpPort string, apiBase string) {
	smtpHost = os.Getenv("MAILPIT_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "127.0.0.1"
	}
	smtpPort = os.Getenv("MAILPIT_SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "1025"
	}
	apiPort := os.Getenv("MAILPIT_API_PORT")
	if apiPort == "" {
		apiPort = "8025"
	}
	apiBase = fmt.Sprintf("http://%s:%s", smtpHost, apiPort)
	return
}
