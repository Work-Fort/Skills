---
type: plan
step: "2"
title: "CLI and Database"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "2"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
---

# Step 2: CLI and Database

## Overview

Wire up the runtime foundation: Cobra CLI with subcommands, koanf
configuration loading, XDG-compliant paths, SQLite store with Goose
migrations, a health endpoint, structured JSON logging, request ID
middleware, and graceful shutdown. After this step the service starts
via `notifier daemon`, creates its SQLite database, runs migrations,
serves `GET /v1/health`, logs every request as structured JSON with a
request ID, and shuts down cleanly on SIGINT/SIGTERM.

No notification-specific endpoints or business logic is delivered here
-- that is Step 3. This step delivers the runtime plumbing that every
subsequent step depends on.

## Prerequisites

- Step 1 completed: project compiles, domain types exist, mise tasks
  work, directory scaffolding is in place
- Go 1.26.0 (pinned in `mise.toml`)
- `mise` CLI available on PATH

## New Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/spf13/cobra` | v1.9.1 | CLI framework |
| `github.com/knadh/koanf/v2` | v2.1.2 | Configuration loading |
| `github.com/knadh/koanf/providers/file` | v1.1.3 | YAML file provider |
| `github.com/knadh/koanf/providers/env` | v1.0.0 | Environment variable provider |
| `github.com/knadh/koanf/parsers/yaml` | v1.0.0 | YAML parser |
| `github.com/pressly/goose/v3` | v3.24.1 | Schema migrations |

Existing: `modernc.org/sqlite` (from Step 1), `github.com/google/uuid`
(from Step 1).

Note: `go mod tidy` resolves exact versions. The versions above are the
targets -- `go mod tidy` may select compatible patch versions. The key
constraint is the major version path in the import.

## Tasks

### Task 1: Configuration Package

Satisfies: service-cli REQ-005 (koanf), REQ-006 (load order), REQ-007
(env prefix), REQ-008 (missing config file), REQ-009 (load once),
REQ-010 (XDG config path), REQ-011 (XDG state path), REQ-012
(InitDirs).

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// No config file, no env vars — should succeed with defaults.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.String("bind"); got != "" {
		t.Errorf("default bind = %q, want empty string", got)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config", "notifier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("port: 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.Int("port"); got != 9090 {
		t.Errorf("port = %d, want 9090", got)
	}
}

func TestLoadConfigEnvOverridesYAML(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config", "notifier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("port: 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("NOTIFIER_PORT", "3000")

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.Int("port"); got != 3000 {
		t.Errorf("port = %d, want 3000 (env should override YAML)", got)
	}
}

func TestInitDirsCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := InitDirs(); err != nil {
		t.Fatalf("InitDirs() error: %v", err)
	}

	configDir := filepath.Join(tmp, "config", "notifier")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Errorf("config dir not created: %s", configDir)
	}
	stateDir := filepath.Join(tmp, "state", "notifier")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Errorf("state dir not created: %s", stateDir)
	}
}

func TestConfigPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	got := ConfigPath()
	want := filepath.Join(tmp, "config", "notifier", "config.yaml")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestStatePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	got := StatePath()
	want := filepath.Join(tmp, "state", "notifier")
	if got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestLoadConfigDefaults ./internal/config/...`
Expected: FAIL with "undefined: Load"

**Step 3: Write the implementation**

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const serviceName = "notifier"

// K is the global koanf instance. It is loaded once during startup
// (in PersistentPreRunE) and read during request handling. It is not
// safe for concurrent Load calls, but concurrent Get calls after a
// single Load are safe.
var K = koanf.New(".")

// Load reads configuration from the YAML config file and environment
// variables. A missing config file is not an error. Environment
// variables override file values. The prefix NOTIFIER_ is stripped,
// the remainder is lowercased, and underscores become dots.
func Load() error {
	// Reset for testability — allows calling Load() multiple times
	// in tests without accumulating state from prior calls.
	K = koanf.New(".")

	// Load from YAML file (missing file is ok).
	_ = K.Load(file.Provider(ConfigPath()), yaml.Parser())

	// Load from environment — strip prefix, lowercase, replace _ with .
	if err := K.Load(env.Provider("NOTIFIER_", ".", func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, "NOTIFIER_")),
			"_", ".", -1,
		)
	}), nil); err != nil {
		return fmt.Errorf("load env config: %w", err)
	}
	return nil
}

// ConfigPath returns the path to the YAML config file:
// $XDG_CONFIG_HOME/notifier/config.yaml
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, serviceName, "config.yaml")
}

// StatePath returns the XDG state directory for runtime data:
// $XDG_STATE_HOME/notifier/
func StatePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, serviceName)
}

// InitDirs creates the XDG config and state directories if they do
// not exist.
func InitDirs() error {
	dirs := []string{
		filepath.Dir(ConfigPath()),
		StatePath(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}
	return nil
}
```

**Step 4: Run go mod tidy to fetch dependencies**

Run: `go mod tidy`
Expected: exits 0. koanf and its providers/parsers are added to
`go.mod` and `go.sum`.

**Step 5: Run tests to verify they pass**

Run: `go test -run "TestLoadConfig|TestInitDirs|TestConfigPath|TestStatePath" ./internal/config/...`
Expected: PASS (6 tests)

**Step 6: Commit**

`feat(config): add koanf config loading and XDG path management`

---

### Task 2: Cobra Root Command and CLI Entrypoint

Satisfies: service-cli REQ-001 (daemon subcommand registration point),
REQ-004 (version from ldflags), REQ-009 (koanf load in
PersistentPreRunE), REQ-012 (InitDirs in PersistentPreRunE), REQ-016
(os.Exit on error).

**Depends on:** Task 1 (config package)

**Files:**
- Create: `internal/cli/root.go`
- Modify: `main.go`

**Step 1: Create the root command**

```go
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
)

// Version is set from main.go, which receives it via -ldflags.
var Version = "dev"

// NewRootCmd creates the root cobra command with PersistentPreRunE
// that initialises XDG directories and loads configuration.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notifier",
		Short:   "Notification service",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := config.InitDirs(); err != nil {
				return err
			}
			return config.Load()
		},
		// Silence Cobra's built-in error/usage printing so we
		// control output via os.Exit in Execute().
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return cmd
}

// Execute runs the root command. Exits with code 1 on error.
func Execute() {
	root := NewRootCmd()
	root.Version = Version
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 2: Update main.go to use the CLI**

Replace the contents of `main.go`:

```go
package main

import "github.com/workfort/notifier/internal/cli"

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	cli.Version = Version
	cli.Execute()
}
```

**Step 3: Run go mod tidy**

Run: `go mod tidy`
Expected: exits 0. `github.com/spf13/cobra` added to `go.mod`.

**Step 4: Verify the binary builds and runs**

Run: `go build -o /dev/null . && go run . --version`
Expected: exits 0, output contains "notifier version dev"

**Step 5: Commit**

`feat(cli): add cobra root command with config and XDG init`

---

### Task 3: Daemon Subcommand (Minimal — HTTP Server + Graceful Shutdown)

Satisfies: service-cli REQ-001 (daemon subcommand), REQ-013 (--bind
flag), REQ-014 (--port flag), REQ-015 (--db flag),
service-observability REQ-012 (ReadTimeout), REQ-013 (WriteTimeout),
REQ-014 (IdleTimeout), REQ-015 (ReadHeaderTimeout).

The daemon starts the HTTP server and handles graceful shutdown. The
mux is initially empty -- the health endpoint is wired in Task 7 after
the store exists. Flags bind to koanf keys via defaults so the config
file and env vars can also set them.

**Depends on:** Task 2 (root command)

**Files:**
- Create: `cmd/daemon/daemon.go` (replaces `cmd/daemon/doc.go`)
- Modify: `internal/cli/root.go`

**Step 1: Create the daemon subcommand**

Replace `cmd/daemon/doc.go` with `cmd/daemon/daemon.go`:

```go
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
)

// NewCmd creates the daemon subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE:  run,
	}
	cmd.Flags().String("bind", "127.0.0.1", "Bind address")
	cmd.Flags().Int("port", 8080, "Listen port")
	cmd.Flags().String("db", "", "Database DSN (empty = SQLite in XDG state dir)")
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Resolve flags: koanf (file/env) takes precedence if the key
	// exists; otherwise fall back to the CLI flag default. Using
	// K.Exists() instead of checking for zero values ensures that
	// intentional zero/empty values (e.g., port 0 for random port,
	// empty bind for all interfaces) are honoured.
	var bind string
	if config.K.Exists("bind") {
		bind = config.K.String("bind")
	} else {
		bind, _ = cmd.Flags().GetString("bind")
	}
	var port int
	if config.K.Exists("port") {
		port = config.K.Int("port")
	} else {
		port, _ = cmd.Flags().GetInt("port")
	}

	mux := http.NewServeMux()

	addr := fmt.Sprintf("%s:%d", bind, port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
```

**Step 2: Register daemon subcommand in the root command**

Modify `internal/cli/root.go` -- add the import and registration
inside `NewRootCmd`:

```go
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/cmd/daemon"
	"github.com/workfort/notifier/internal/config"
)

// Version is set from main.go, which receives it via -ldflags.
var Version = "dev"

// NewRootCmd creates the root cobra command with PersistentPreRunE
// that initialises XDG directories and loads configuration.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notifier",
		Short:   "Notification service",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := config.InitDirs(); err != nil {
				return err
			}
			return config.Load()
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(daemon.NewCmd())
	return cmd
}

// Execute runs the root command. Exits with code 1 on error.
func Execute() {
	root := NewRootCmd()
	root.Version = Version
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 3: Delete the old doc.go placeholder**

Delete `cmd/daemon/doc.go` since `daemon.go` now defines package
`daemon`.

**Step 4: Verify the binary builds**

Run: `go build -o /dev/null .`
Expected: exits 0

**Step 5: Commit**

`feat(daemon): add daemon subcommand with HTTP server and graceful shutdown`

---

### Task 4: Structured Logging

Satisfies: service-observability REQ-001 (log/slog), REQ-002 (JSON
format).

Initialise the global slog logger with JSON output during daemon
startup so all log statements throughout the service emit structured
JSON.

**Files:**
- Modify: `cmd/daemon/daemon.go`

**Step 1: Add slog JSON handler initialization at the top of the `run` function**

Add the following at the start of the `run` function in
`cmd/daemon/daemon.go`, before the flag resolution:

```go
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
```

This line already uses `log/slog` and `os` which are imported. No new
imports are needed.

**Step 2: Verify the binary builds**

Run: `go build -o /dev/null .`
Expected: exits 0

**Step 3: Commit**

`feat(logging): initialise slog JSON handler in daemon startup`

---

### Task 5: Request ID and Logging Middleware

Satisfies: service-observability REQ-004 (UUID per request), REQ-005
(request ID in context), REQ-006 (X-Request-ID response header),
REQ-008 (middleware order), REQ-009 (request logging), REQ-010
(statusRecorder with Unwrap), REQ-011 (panic recovery).

**Spec delta:** The observability spec REQ-008 defines two middleware
layers (request logging, then panic recovery). This plan adds a third
outermost layer -- request ID -- because the logging middleware needs
the request ID in the context to include it in log entries. The
ordering is: request ID (outermost) -> request logging -> panic recovery
(innermost). The spec should be updated to acknowledge the request ID
middleware as the outermost layer.

**Files:**
- Create: `internal/infra/httpapi/middleware.go`
- Test: `internal/infra/httpapi/middleware_test.go`

**Step 1: Write the failing tests**

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in the context.
		reqID := RequestIDFromContext(r.Context())
		if reqID == "" {
			t.Error("request ID not found in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := WithRequestID(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify X-Request-ID header is set and has a reasonable format
	// (prefix + separator + UUID). The exact prefix is an implementation
	// detail tested in domain/identity_test.go.
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatal("X-Request-ID response header is empty")
	}
	if !strings.Contains(rid, "_") {
		t.Errorf("X-Request-ID = %q, want format prefix_uuid", rid)
	}
}

func TestRequestLoggingMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := WithRequestLogging(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

func TestPanicRecoveryMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := WithPanicRecovery(inner)
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// Should not panic.
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	if sr.Unwrap() != rec {
		t.Error("Unwrap() did not return the underlying ResponseWriter")
	}
}

func TestMiddlewareStackOrder(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is available (set by outer middleware).
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("request ID not set by middleware stack")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := WithMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header missing from middleware stack")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestRequestID|TestRequestLogging|TestPanicRecovery|TestStatusRecorder|TestMiddlewareStack" ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: RequestIDFromContext"

**Step 3: Write the middleware implementation**

```go
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID generates a unique request ID, stores it in the
// request context, and sets the X-Request-ID response header.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := domain.NewID("req")
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, required by
// http.ResponseController and middleware that need to access the
// original writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// WithRequestLogging logs every HTTP request with method, path,
// status code, and duration.
func WithRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		reqID := RequestIDFromContext(r.Context())
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
			"request_id", reqID,
		)
	})
}

// WithPanicRecovery catches panics in downstream handlers, logs the
// error, and returns HTTP 500 if no response has been written.
// This middleware is always wrapped by WithRequestLogging which
// installs a statusRecorder, so w is guaranteed to be a
// *statusRecorder here.
func WithPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
				)
				// Only write 500 if nothing has been sent yet.
				if rec, ok := w.(*statusRecorder); ok && !rec.written {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// WithMiddleware applies the full middleware stack in the correct
// order (outermost first): request ID, then request logging, then
// panic recovery. Request ID is outermost so the logging middleware
// can read it from context. This extends observability spec REQ-008
// (which specifies logging then panic recovery) with the request ID
// layer; the spec should be updated to reflect this.
func WithMiddleware(next http.Handler) http.Handler {
	return WithRequestID(WithRequestLogging(WithPanicRecovery(next)))
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestRequestID|TestRequestLogging|TestPanicRecovery|TestStatusRecorder|TestMiddlewareStack" ./internal/infra/httpapi/...`
Expected: PASS (5 tests)

**Step 5: Commit**

`feat(httpapi): add request ID, logging, and panic recovery middleware`

---

### Task 6: SQLite Store with Goose Migrations

Satisfies: service-database REQ-001 (backend selection), REQ-002
(default to XDG state dir), REQ-003 (modernc.org/sqlite), REQ-004
(WAL), REQ-005 (foreign keys), REQ-006 (busy timeout), REQ-007
(MaxOpenConns=1), REQ-008 (in-memory for tests), REQ-013 (goose),
REQ-014 (embedded migrations), REQ-015 (numbered SQL), REQ-016
(up/down markers), REQ-017 (auto-migrate on open), REQ-018
(parameterized placeholders), REQ-020 (implements domain.Store).

**Files:**
- Create: `internal/infra/sqlite/migrations/001_init.sql`
- Create: `internal/infra/sqlite/store.go` (replaces `doc.go`)
- Test: `internal/infra/sqlite/store_test.go`

**Step 1: Create the initial migration**

`internal/infra/sqlite/migrations/001_init.sql`:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    status      INTEGER NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    retry_limit INTEGER NOT NULL DEFAULT 3,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_notifications_email ON notifications(email);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_status;
DROP INDEX IF EXISTS idx_notifications_email;
DROP TABLE IF EXISTS notifications;
```

**Step 2: Write the failing tests**

```go
package sqlite

import (
	"context"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestOpenInMemory(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error: %v", err)
	}
	defer store.Close()
}

func TestPing(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestCreateAndGetNotification(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_test-123",
		Email:      "test@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatalf("CreateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "test@test.com")
	if err != nil {
		t.Fatalf("GetNotificationByEmail() error: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID = %q, want %q", got.ID, n.ID)
	}
	if got.Email != n.Email {
		t.Errorf("Email = %q, want %q", got.Email, n.Email)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("Status = %v, want %v", got.Status, domain.StatusPending)
	}
	if got.RetryLimit != domain.DefaultRetryLimit {
		t.Errorf("RetryLimit = %d, want %d", got.RetryLimit, domain.DefaultRetryLimit)
	}
}

func TestCreateDuplicateReturnsAlreadyNotified(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_dup-1",
		Email:      "dup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n2 := &domain.Notification{
		ID:         "ntf_dup-2",
		Email:      "dup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	err = store.CreateNotification(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !isDomainErr(err, domain.ErrAlreadyNotified) {
		t.Errorf("error = %v, want ErrAlreadyNotified", err)
	}
}

func TestGetNotificationNotFound(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetNotificationByEmail(context.Background(), "nobody@test.com")
	if err == nil {
		t.Fatal("expected error for missing notification, got nil")
	}
	if !isDomainErr(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateNotification(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_upd-1",
		Email:      "update@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n.Status = domain.StatusSending
	if err := store.UpdateNotification(ctx, n); err != nil {
		t.Fatalf("UpdateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "update@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusSending {
		t.Errorf("Status = %v, want %v", got.Status, domain.StatusSending)
	}
}

func TestListNotifications(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	for i, email := range []string{"a@test.com", "b@test.com", "c@test.com"} {
		n := &domain.Notification{
			ID:         domain.NewID("ntf"),
			Email:      email,
			Status:     domain.StatusPending,
			RetryLimit: domain.DefaultRetryLimit,
		}
		_ = i
		if err := store.CreateNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.ListNotifications(ctx, "", 2)
	if err != nil {
		t.Fatalf("ListNotifications() error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}

	// Second page using the last ID as cursor.
	list2, err := store.ListNotifications(ctx, list[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(list2))
	}
}

// isDomainErr is a test helper using errors.Is.
func isDomainErr(err, target error) bool {
	return err != nil && errors.Is(err, target)
}
```

Note: add `"errors"` to the import block.

**Step 3: Run tests to verify they fail**

Run: `go test -run TestOpenInMemory ./internal/infra/sqlite/...`
Expected: FAIL with "undefined: Open"

**Step 4: Write the SQLite store implementation**

Replace `internal/infra/sqlite/doc.go` with
`internal/infra/sqlite/store.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/workfort/notifier/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store implements domain.Store backed by SQLite.
type Store struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB for sharing with goqite.
// Satisfies service-database REQ-019.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Open creates a new SQLite store. An empty DSN creates an in-memory
// database (for tests). A non-empty DSN is used as a file path.
// Migrations are run automatically.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// REQ-007: single-writer serialization.
	db.SetMaxOpenConns(1)

	// REQ-004: WAL mode.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	// REQ-005: foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// REQ-006: busy timeout.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Run embedded goose migrations.
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetBaseFS(migrations); err != nil {
		return nil, fmt.Errorf("set migration fs: %w", err)
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// CreateNotification inserts a new notification record. Returns
// domain.ErrAlreadyNotified if the email already exists.
func (s *Store) CreateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, email, status, retry_count, retry_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Email, int(n.Status), n.RetryCount, n.RetryLimit,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create notification %s: %w", n.Email, domain.ErrAlreadyNotified)
		}
		return fmt.Errorf("create notification: %w", err)
	}
	n.CreatedAt = now
	n.UpdatedAt = now
	return nil
}

// GetNotificationByEmail retrieves a notification by email address.
// Returns domain.ErrNotFound if no record exists.
func (s *Store) GetNotificationByEmail(ctx context.Context, email string) (*domain.Notification, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
		 FROM notifications WHERE email = ?`, email,
	)

	n := &domain.Notification{}
	var status int
	var createdAt, updatedAt string
	err := row.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get notification %s: %w", email, domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}
	n.Status = domain.Status(status)
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return n, nil
}

// UpdateNotification updates an existing notification record.
func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET status = ?, retry_count = ?, retry_limit = ?, updated_at = ?
		 WHERE id = ?`,
		int(n.Status), n.RetryCount, n.RetryLimit, now.Format(time.RFC3339), n.ID,
	)
	if err != nil {
		return fmt.Errorf("update notification: %w", err)
	}
	n.UpdatedAt = now
	return nil
}

// ListNotifications returns notifications with cursor-based pagination.
// If after is empty, returns from the beginning. Limit controls page size.
func (s *Store) ListNotifications(ctx context.Context, after string, limit int) ([]*domain.Notification, error) {
	var rows *sql.Rows
	var err error
	if after == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications ORDER BY created_at ASC, id ASC LIMIT ?`, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications
			 WHERE (created_at, id) > (
			     SELECT created_at, id FROM notifications WHERE id = ?
			 )
			 ORDER BY created_at ASC, id ASC LIMIT ?`, after, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var result []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		var status int
		var createdAt, updatedAt string
		if err := rows.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Status = domain.Status(status)
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, n)
	}
	return result, rows.Err()
}

// isUniqueViolation checks if a SQLite error is a UNIQUE constraint
// violation. modernc.org/sqlite returns error strings containing
// "UNIQUE constraint failed".
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

**Step 5: Delete the old doc.go placeholder**

Delete `internal/infra/sqlite/doc.go` since `store.go` now defines
package `sqlite`.

**Step 6: Run go mod tidy**

Run: `go mod tidy`
Expected: exits 0. `pressly/goose/v3` added to `go.mod`.

**Step 7: Run tests to verify they pass**

Run: `go test -run "TestOpen|TestPing|TestCreate|TestGet|TestUpdate|TestList" ./internal/infra/sqlite/...`
Expected: PASS (7 tests)

**Step 8: Commit**

`feat(sqlite): add SQLite store with goose migrations and CRUD operations`

---

### Task 7: Health Endpoint

Satisfies: service-observability health check (architecture reference
health endpoint pattern), service-database REQ-020 (HealthChecker via
Ping).

**Depends on:** Task 5 (middleware), Task 6 (store)

**Files:**
- Create: `internal/infra/httpapi/health.go`
- Test: `internal/infra/httpapi/health_test.go`

**Step 1: Write the failing test**

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubHealthChecker implements domain.HealthChecker for tests.
type stubHealthChecker struct {
	err error
}

func (s *stubHealthChecker) Ping(_ context.Context) error {
	return s.err
}

func TestHealthEndpointHealthy(t *testing.T) {
	handler := HandleHealth(&stubHealthChecker{err: nil})
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want %q", body["status"], "healthy")
	}
}

func TestHealthEndpointUnhealthy(t *testing.T) {
	handler := HandleHealth(&stubHealthChecker{err: errors.New("db down")})
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "unhealthy" {
		t.Errorf("status = %q, want %q", body["status"], "unhealthy")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestHealthEndpoint" ./internal/infra/httpapi/...`
Expected: FAIL with "undefined: HandleHealth"

**Step 3: Write the health endpoint implementation**

```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/workfort/notifier/internal/domain"
)

// HandleHealth returns an http.HandlerFunc that checks database
// connectivity via domain.HealthChecker.
func HandleHealth(checker domain.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := checker.Ping(r.Context())
		status := "healthy"
		httpCode := http.StatusOK
		if err != nil {
			status = "unhealthy"
			httpCode = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpCode)
		//nolint:errcheck // response write errors are unactionable after WriteHeader
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestHealthEndpoint" ./internal/infra/httpapi/...`
Expected: PASS (2 tests)

**Step 5: Commit**

`feat(httpapi): add health endpoint with database ping check`

---

### Task 8: Wire Store and Health into Daemon

Connect the SQLite store and health endpoint into the daemon
subcommand so the server is fully operational.

**Depends on:** Task 3 (daemon), Task 6 (store), Task 7 (health
endpoint)

**Files:**
- Modify: `cmd/daemon/daemon.go`

**Step 1: Update the daemon run function to open the store, register the health endpoint, and apply middleware**

Replace the `run` function body in `cmd/daemon/daemon.go`:

```go
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/httpapi"
	"github.com/workfort/notifier/internal/infra/seed"
	"github.com/workfort/notifier/internal/infra/sqlite"
)

// NewCmd creates the daemon subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE:  run,
	}
	cmd.Flags().String("bind", "127.0.0.1", "Bind address")
	cmd.Flags().Int("port", 8080, "Listen port")
	cmd.Flags().String("db", "", "Database DSN (empty = SQLite in XDG state dir)")
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve flags: koanf (file/env) takes precedence if the key
	// exists; otherwise fall back to the CLI flag default. Using
	// K.Exists() instead of checking for zero values ensures that
	// intentional zero/empty values (e.g., port 0 for random port,
	// empty bind for all interfaces) are honoured.
	var bind string
	if config.K.Exists("bind") {
		bind = config.K.String("bind")
	} else {
		bind, _ = cmd.Flags().GetString("bind")
	}
	var port int
	if config.K.Exists("port") {
		port = config.K.Int("port")
	} else {
		port, _ = cmd.Flags().GetInt("port")
	}
	var dsn string
	if config.K.Exists("db") {
		dsn = config.K.String("db")
	} else {
		dsn, _ = cmd.Flags().GetString("db")
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return RunServer(ctx, bind, port, dsn)
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context instead of
// relying on process-wide signal delivery.
func RunServer(ctx context.Context, bind string, port int, dsn string) error {
	// If DSN is still empty (no config/env/flag override), use SQLite
	// in the XDG state directory.
	if dsn == "" {
		dsn = filepath.Join(config.StatePath(), "notifier.db")
	}

	// Open the store.
	store, err := sqlite.Open(dsn)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Run QA seed data (no-op in non-QA builds).
	if err := seed.RunSeed(store.DB()); err != nil {
		return fmt.Errorf("run seed: %w", err)
	}

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))

	// Apply middleware stack.
	handler := httpapi.WithMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", bind, port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
```

**Step 2: Verify the binary builds**

Run: `go build -o /dev/null .`
Expected: exits 0

**Step 3: Commit**

`feat(daemon): wire SQLite store, health endpoint, and middleware into daemon`

---

### Task 9: Smoke Test — Full Server Lifecycle

Verify the entire stack works end-to-end: server starts, health
endpoint responds, server shuts down cleanly.

**Depends on:** Task 8

**Files:**
- Create: `cmd/daemon/daemon_test.go`

**Step 1: Write the integration test**

```go
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
	ln.Close()

	dbPath := filepath.Join(tmp, "state", "notifier", "test.db")

	// Use a cancellable context to trigger graceful shutdown instead
	// of sending SIGINT to the process (which would interfere with
	// the test runner if other tests run concurrently).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, "127.0.0.1", port, dbPath)
	}()

	// Wait for server to be ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	ready := false
	for i := 0; i < 50; i++ {
		resp, err := http.Get(addr + "/v1/health")
		if err == nil {
			resp.Body.Close()
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
	defer resp.Body.Close()

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
```

**Step 2: Run the smoke test**

Run: `go test -run TestDaemonHealthEndpoint -timeout 30s ./cmd/daemon/...`
Expected: PASS (1 test). Server starts, health responds 200 with
`{"status":"healthy"}` and X-Request-ID header, server shuts down
cleanly.

**Step 3: Commit**

`test(daemon): add smoke test for server lifecycle and health endpoint`

---

### Task 10: Full Build Verification and Lint

**Depends on:** Tasks 1-9

**Step 1: Run go mod tidy to ensure clean dependencies**

Run: `go mod tidy`
Expected: exits 0

**Step 2: Verify the full project compiles (dev build)**

Run: `mise run build:go`
Expected: exits 0, `build/notifier` binary exists

**Step 3: Run all unit tests**

Run: `mise run test:unit`
Expected: PASS -- all config, middleware, health, store, and domain
tests pass

**Step 4: Run all unit tests with QA build tag**

Run: `go test -tags qa ./...`
Expected: PASS -- seed tests pass alongside all other tests

**Step 5: Run the linter**

Run: `mise run lint:go`
Expected: exits 0, no warnings

**Step 6: Run the CI task**

Run: `mise run ci`
Expected: exits 0 (depends on lint:go, test:unit, build:go)

**Step 7: Verify the binary starts and responds to health check**

Rely on the smoke test from Task 9 which validates the full server
lifecycle programmatically (start, health check, graceful shutdown)
using a random port and a cancellable context.

**Step 8: Clean up**

Run: `mise run clean:go`
Expected: `build/` directory removed

**Step 9: Commit (if any adjustments were needed)**

`chore(step2): finalize step 2 CLI and database`

Only create this commit if adjustments were required during
verification. If everything passed cleanly, no commit is needed.

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings
- [ ] `go build -tags qa ./...` succeeds
- [ ] `go test ./...` passes all tests (config, middleware, health, store, domain)
- [ ] `go test -tags qa ./...` passes all tests including seed
- [ ] `go vet ./...` reports no issues
- [ ] `mise run build:go` produces `build/notifier`
- [ ] `mise run test:unit` exits 0
- [ ] `mise run lint:go` exits 0
- [ ] `mise run ci` exits 0
- [ ] `./build/notifier --version` prints version string
- [ ] `./build/notifier daemon` starts and creates SQLite DB in XDG state dir
- [ ] `curl http://127.0.0.1:8080/v1/health` returns `{"status":"healthy"}`
- [ ] Health response includes `X-Request-ID` header (non-empty, prefix_uuid format)
- [ ] Ctrl-C triggers graceful shutdown with "shutdown signal received" log
- [ ] Log output is structured JSON with method, path, status, duration fields
- [ ] Missing config file does not cause startup error
- [ ] `NOTIFIER_PORT=3000 ./build/notifier daemon` binds to port 3000
- [ ] SQLite database file is created in `$XDG_STATE_HOME/notifier/`
- [ ] In-memory SQLite tests pass (`sqlite.Open("")`)
- [ ] No orphan packages -- every directory under `internal/` compiles
