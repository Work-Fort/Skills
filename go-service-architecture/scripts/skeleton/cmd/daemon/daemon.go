package daemon

import (
	"context"
	"embed"
	"errors"
	"fmt"
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
		Dev:        dev,
		DevURL:     devURL,
		FrontendFS: frontendFS,
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
	Dev        bool
	DevURL     string
	FrontendFS embed.FS
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	// Default DSN to XDG state directory.
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

	// Create a separate context for the hub. Shutdown cancels it
	// after HTTP connections drain so write pumps can still dispatch
	// messages during graceful shutdown (REQ-015).
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	// Create WebSocket hub and start it (REQ-006: before HTTP server).
	hub := ws.NewHub()
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
	go runner.Start(runnerCtx)

	// Create the MCP handler.
	mcpHandler := mcpinfra.NewMCPHandler(store, nq, cfg.Version)

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
	mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
	mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))
	mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx))
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// 5. Close database (store.Close) happens in the existing defer
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
