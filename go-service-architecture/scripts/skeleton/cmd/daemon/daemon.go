package daemon

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"maragu.dev/goqite"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/db"
	"github.com/workfort/notifier/internal/infra/email"
	"github.com/workfort/notifier/internal/infra/httpapi"
	mcpinfra "github.com/workfort/notifier/internal/infra/mcp"
	"github.com/workfort/notifier/internal/infra/queue"
	"github.com/workfort/notifier/internal/infra/seed"
	"github.com/workfort/notifier/internal/infra/ws"
)

// NewCmd creates the daemon subcommand.
func NewCmd(frontendFS embed.FS) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithFS(cmd, args, frontendFS)
		},
	}
	cmd.Flags().String("bind", "127.0.0.1", "Bind address")
	cmd.Flags().Int("port", 8080, "Listen port")
	cmd.Flags().String("db", "", "Database DSN (empty = SQLite in XDG state dir)")
	cmd.Flags().String("smtp-host", "127.0.0.1", "SMTP server host")
	cmd.Flags().Int("smtp-port", 1025, "SMTP server port")
	cmd.Flags().String("smtp-from", "notifier@localhost", "Email sender address")
	cmd.Flags().Bool("dev", false, "Enable dev mode (proxy to Vite dev server)")
	cmd.Flags().String("dev-url", "http://localhost:5173", "Vite dev server URL")
	cmd.Flags().Int("shutdown-timeout", 60, "Shutdown timeout in seconds")
	return cmd
}

func runWithFS(cmd *cobra.Command, _ []string, frontendFS embed.FS) error {
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve flags: koanf (file/env) takes precedence.
	bind := resolveString(cmd, "bind")
	port := resolveInt(cmd, "port")
	dsn := resolveString(cmd, "db")
	smtpHost := resolveString(cmd, "smtp-host")
	smtpPort := resolveInt(cmd, "smtp-port")
	smtpFrom := resolveString(cmd, "smtp-from")
	dev, _ := cmd.Flags().GetBool("dev")
	devURL := resolveString(cmd, "dev-url")
	shutdownTimeout := resolveDuration(cmd, "shutdown-timeout")

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Get version from the root command.
	version := "dev"
	if root := cmd.Root(); root != nil {
		version = root.Version
	}

	return RunServer(ctx, ServerConfig{
		Bind:       bind,
		Port:       port,
		DSN:        dsn,
		SMTPHost:   smtpHost,
		SMTPPort:   smtpPort,
		SMTPFrom:   smtpFrom,
		Version:    version,
		Dev:             dev,
		DevURL:          devURL,
		ShutdownTimeout: shutdownTimeout,
		FrontendFS:      frontendFS,
	})
}

// ServerConfig holds configuration for RunServer.
type ServerConfig struct {
	Bind       string
	Port       int
	DSN        string
	SMTPHost   string
	SMTPPort   int
	SMTPFrom   string
	Version    string
	Dev             bool
	DevURL          string
	ShutdownTimeout time.Duration
	FrontendFS      embed.FS
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	// QA builds force in-memory SQLite; non-QA passes through unchanged.
	cfg.DSN = resolveDSN(cfg.DSN)

	// Default DSN to XDG state directory (only reached in non-QA builds).
	if cfg.DSN == "" {
		cfg.DSN = filepath.Join(config.StatePath(), "notifier.db")
	}

	// Open the store (SQLite or PostgreSQL based on DSN).
	store, err := db.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Determine goqite SQL flavor from DSN.
	var queueFlavor queue.Flavor
	if strings.HasPrefix(cfg.DSN, "postgres") {
		queueFlavor = queue.FlavorPostgres
	}

	// Run QA seed data (no-op in non-QA builds).
	if strings.HasPrefix(cfg.DSN, "postgres") {
		if err := seed.RunSeed(store.DB(), goqite.SQLFlavorPostgreSQL); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	} else {
		if err := seed.RunSeed(store.DB()); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	}

	// Set up goqite queue and job runner.
	nq, err := queue.NewNotificationQueue(store.DB(), queueFlavor)
	if err != nil {
		return fmt.Errorf("create notification queue: %w", err)
	}

	// Set up SMTP sender.
	sender, err := email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("create smtp sender: %w", err)
	}

	allowedOrigins := resolveAllowedOrigins()

	// Create a separate context for the hub. Shutdown cancels it
	// after HTTP connections drain so write pumps can still dispatch
	// messages during graceful shutdown (REQ-015).
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	// Create WebSocket hub and start it (REQ-006: before HTTP server).
	hub := ws.NewHub(1000)
	go hub.Run(hubCtx)

	// Create a separate context for the job runner. Shutdown cancels
	// it after the hub so the runner can finish its current job before
	// the store is closed.
	runnerCtx, runnerCancel := context.WithCancel(context.Background())
	defer runnerCancel()

	// Create and register the email worker with broadcaster.
	worker := queue.NewEmailWorker(store, sender, hub)
	runner := queue.NewJobRunner(nq.Queue())
	runner.Register("send_notification", worker.Handle)

	// Start the job runner in a goroutine.
	// runnerDone closes when Start returns, signalling that all
	// in-flight jobs have finished (REQ-017, REQ-018).
	runnerDone := make(chan struct{})
	go func() {
		runner.Start(runnerCtx)
		close(runnerDone)
	}()

	// Create the MCP handler.
	mcpHandler := mcpinfra.NewMCPHandler(store, nq, cfg.Version)

	// Select SPA handler: dev proxy or embedded filesystem.
	var spaHandler http.Handler
	if cfg.Dev {
		spaHandler = httpapi.NewSPADevProxy(cfg.DevURL)
		slog.Info("dev mode: proxying to Vite", "url", cfg.DevURL)
	} else {
		// Spec delta: REQ-006 says fs.Sub(webFS, "dist") but the embed
		// lives at the project root, so the path is "web/dist".
		distFS, err := fs.Sub(cfg.FrontendFS, "web/dist")
		if err != nil {
			return fmt.Errorf("embedded SPA: %w", err)
		}

		// Warn if the embedded FS is empty (built without -tags spa).
		if _, readErr := fs.Stat(distFS, "index.html"); readErr != nil {
			slog.Warn("no embedded frontend found; built without -tags spa?",
				"hint", "use --dev to proxy to Vite, or rebuild with -tags spa")
		}

		spaHandler = httpapi.NewSPAHandler(distFS)
	}

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
	mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
	mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))
	mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx, allowedOrigins))
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

	// SPA catch-all — must be last so API routes take priority.
	mux.Handle("/", spaHandler)

	// Apply middleware stack.
	handler := httpapi.WithMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
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

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 60 * time.Second
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// 1. Drain HTTP connections -- reject new requests, let
	//    in-flight handlers (including WebSocket upgrades) finish.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}

	// 2. Signal MCP SSE clients to reconnect (REQ-013).
	if err := mcpHandler.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp shutdown", "error", err)
	}

	// 3. Cancel hub context -- hub stops, write pumps close WS
	//    connections with StatusNormalClosure (REQ-015).
	hubCancel()

	// 4. Cancel runner -- job runner drains current job, then exits.
	runnerCancel()

	// 5. Wait for in-flight jobs to finish, bounded by shutdown
	//    timeout so a stuck job cannot block forever (REQ-018).
	select {
	case <-runnerDone:
		slog.Info("runner stopped")
	case <-shutdownCtx.Done():
		slog.Warn("shutdown timeout waiting for runner")
	}

	// 6. Close database (store.Close) happens in the existing defer
	//    block above.
	return nil
}

// resolveString reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.String(dotKey)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}

// resolveInt reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveInt(cmd *cobra.Command, key string) int {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.Int(dotKey)
	}
	v, _ := cmd.Flags().GetInt(key)
	return v
}

// resolveDuration reads a timeout value (in seconds) from koanf or
// cobra flag and returns it as a time.Duration.
func resolveDuration(cmd *cobra.Command, key string) time.Duration {
	seconds := resolveInt(cmd, key)
	return time.Duration(seconds) * time.Second
}

// resolveAllowedOrigins returns the configured WebSocket origin
// patterns. It checks the koanf YAML path first, then falls back
// to the NOTIFIER_WS_ALLOWED_ORIGINS environment variable (comma-
// separated). If neither is set, returns the default patterns that
// permit local development (REQ-022).
func resolveAllowedOrigins() []string {
	defaultOrigins := []string{"localhost:*", "127.0.0.1:*"}

	// Check koanf for the YAML config path.
	if config.K.Exists("ws.allowed_origins") {
		if origins := config.K.Strings("ws.allowed_origins"); len(origins) > 0 {
			return origins
		}
	}

	// Fall back to the environment variable directly to avoid the
	// koanf underscore-to-dot mapping mismatch (NOTIFIER_WS_ALLOWED_ORIGINS
	// maps to "ws.allowed.origins" in koanf, not "ws.allowed_origins").
	if envVal := os.Getenv("NOTIFIER_WS_ALLOWED_ORIGINS"); envVal != "" {
		var origins []string
		for _, o := range strings.Split(envVal, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) > 0 {
			return origins
		}
	}

	return defaultOrigins
}
